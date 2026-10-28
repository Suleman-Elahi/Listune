import { parseBuffer } from 'music-metadata';
import { openDB, type IDBPDatabase } from 'idb';
import type { S3Config, LocalSource, Track, MusicPlayerDB } from 'src/types/models';
import {
  buildPresignedUrl,
  proxyUrlFor,
  s3SourceId,
  stableTrackId,
} from '../services/sigv4';

// ── DB helpers (inline, no import from main thread service) ──────────────────

let dbPromise: Promise<IDBPDatabase<MusicPlayerDB>> | null = null;

function getDB(): Promise<IDBPDatabase<MusicPlayerDB>> {
  if (!dbPromise) {
    dbPromise = openDB<MusicPlayerDB>('music-player-db', 1, {
      upgrade(db) {
        if (!db.objectStoreNames.contains('tracks')) {
          const trackStore = db.createObjectStore('tracks', { keyPath: 'id' });
          trackStore.createIndex('by-artist', 'artist');
          trackStore.createIndex('by-album', 'album');
        }
        if (!db.objectStoreNames.contains('artwork')) {
          db.createObjectStore('artwork');
        }
        if (!db.objectStoreNames.contains('sources')) {
          db.createObjectStore('sources', { keyPath: 'name' });
        }
      },
    });
  }
  return dbPromise;
}

async function putTracks(tracks: Track[]): Promise<void> {
  const database = await getDB();
  const tx = database.transaction('tracks', 'readwrite');
  await Promise.all([...tracks.map((t) => tx.store.put(t)), tx.done]);
}

async function putArtwork(trackId: string, blob: Blob): Promise<void> {
  const database = await getDB();
  await database.put('artwork', blob, trackId);
}

// ── S3 ListObjectsV2 ─────────────────────────────────────────────────────────

const BASE_URL = (import.meta as { env?: { VITE_BACKEND_URL?: string } }).env?.VITE_BACKEND_URL || "http://localhost:8090";

const AUDIO_EXTENSIONS = new Set([
  '.mp3',
  '.flac',
  '.ogg',
  '.oga',
  '.opus',
  '.m4a',
  '.m4b',
  '.wav',
  '.aac',
  '.wma',
  '.aiff',
  '.aif',
  '.ape',
  '.webm',
]);

function decodeXmlEntities(value: string): string {
  return value
    .replace(/&lt;/g, '<')
    .replace(/&gt;/g, '>')
    .replace(/&quot;/g, '"')
    .replace(/&apos;/g, "'")
    .replace(/&amp;/g, '&');
}

function decodeListValue(value: string): string {
  const entityDecoded = decodeXmlEntities(value);
  try {
    return decodeURIComponent(entityDecoded);
  } catch {
    return entityDecoded;
  }
}

async function listS3Objects(cfg: S3Config): Promise<string[]> {
  const keys: string[] = [];
  let continuationToken: string | null = null;

  do {
    const query: Record<string, string> = {
      'list-type': '2',
      // Ask S3 to percent-encode keys/tokens so XML entities can't corrupt them.
      'encoding-type': 'url',
    };
    if (continuationToken) {
      query['continuation-token'] = continuationToken;
    }

    const url = await buildPresignedUrl(cfg, '', { expiresIn: 60, query });
    const response = await fetch(proxyUrlFor(url));
    if (!response.ok) {
      throw new Error(`ListObjectsV2 failed: ${response.status} ${response.statusText}`);
    }

    const xml = await response.text();

    for (const match of xml.matchAll(/<Key>([\s\S]*?)<\/Key>/g)) {
      const key = decodeListValue(match[1]!);
      const ext = key.slice(key.lastIndexOf('.')).toLowerCase();
      if (AUDIO_EXTENSIONS.has(ext)) {
        keys.push(key);
      } else {
        console.debug('[scanner] skipping non-audio S3 object', key);
      }
    }

    const isTruncated = /<IsTruncated>\s*true\s*<\/IsTruncated>/i.test(xml);
    const tokenMatch = xml.match(/<NextContinuationToken>([\s\S]*?)<\/NextContinuationToken>/);
    continuationToken = isTruncated && tokenMatch ? decodeListValue(tokenMatch[1]!) : null;
  } while (continuationToken);

  return keys;
}

// ── Local source helpers ─────────────────────────────────────────────────────

async function collectLocalFiles(
  dir: FileSystemDirectoryHandle,
  path = '',
): Promise<{ handle: FileSystemFileHandle; path: string }[]> {
  const results: { handle: FileSystemFileHandle; path: string }[] = [];
  // @ts-expect-error FileSystemDirectoryHandle is async iterable in modern browsers
  // eslint-disable-next-line @typescript-eslint/await-thenable
  for await (const [name, entry] of dir) {
    const entryPath = path ? `${path}/${name}` : name;
    if (entry.kind === 'file') {
      const ext = name.slice(name.lastIndexOf('.')).toLowerCase();
      if (AUDIO_EXTENSIONS.has(ext)) {
        results.push({ handle: entry as FileSystemFileHandle, path: entryPath });
      }
    } else if (entry.kind === 'directory') {
      const sub = await collectLocalFiles(entry as FileSystemDirectoryHandle, entryPath);
      results.push(...sub);
    }
  }
  return results;
}

// ── Metadata parsing helpers ─────────────────────────────────────────────────

function filenameWithoutExtension(filename: string): string {
  const base = filename.slice(filename.lastIndexOf('/') + 1);
  const dotIdx = base.lastIndexOf('.');
  return dotIdx > 0 ? base.slice(0, dotIdx) : base;
}

function totalSizeFromResponse(response: Response): number | undefined {
  const contentRange = response.headers.get('Content-Range');
  const rangeMatch = contentRange?.match(/\/(\d+)$/);
  if (rangeMatch) return Number(rangeMatch[1]);
  const contentLength = response.headers.get('Content-Length');
  return contentLength ? Number(contentLength) : undefined;
}

// ── Main scan logic ──────────────────────────────────────────────────────────

const BATCH_SIZE = 20;
const RANGE_BYTES = 131072; // 128 KB

let cancelled = false;

async function scanS3(cfg: S3Config, sourceName: string): Promise<void> {
  let keys: string[];
  try {
    keys = await listS3Objects(cfg);
  } catch (err) {
    self.postMessage({ type: 'error', message: String(err) });
    return;
  }

  const total = keys.length;
  let scanned = 0;
  let errors = 0;

  for (let i = 0; i < keys.length; i += BATCH_SIZE) {
    if (cancelled) break;

    const batch = keys.slice(i, i + BATCH_SIZE);
    const tracks: Track[] = [];

    for (const key of batch) {
      if (cancelled) break;

      const id = await stableTrackId(sourceName, 's3', key);
      let buffer: ArrayBuffer;
      let totalSize: number | undefined;

      try {
        const url = await buildPresignedUrl(cfg, key, { expiresIn: 300 });
        const response = await fetch(proxyUrlFor(url), {
          headers: { Range: `bytes=0-${RANGE_BYTES - 1}` },
        });
        if (!response.ok && response.status !== 206) {
          console.warn('[scanner] S3 fetch failed', key, response.status, response.statusText);
          errors++;
          continue;
        }
        totalSize = totalSizeFromResponse(response);
        buffer = await response.arrayBuffer();
      } catch (err) {
        console.warn('[scanner] S3 fetch error', key, err);
        errors++;
        continue;
      }

      let track: Track;
      try {
        const metadata = await parseBuffer(
          new Uint8Array(buffer),
          totalSize ? { size: totalSize } : undefined,
          { duration: true, skipCovers: false },
        );
        const { title, artist, album } = metadata.common;
        const duration = metadata.format.duration ?? 0;
        const cover = metadata.common.picture?.[0];

        let artworkId: string | null = null;
        if (cover) {
          const blob = new Blob(
            [
              cover.data.buffer.slice(
                cover.data.byteOffset,
                cover.data.byteOffset + cover.data.byteLength,
              ) as ArrayBuffer,
            ],
            { type: cover.format },
          );
          await putArtwork(id, blob);
          artworkId = id;
        }

        track = {
          id,
          title: title || filenameWithoutExtension(key),
          artist: artist || '',
          album: album || '',
          duration,
          sourceTag: 's3',
          sourceName,
          artworkId,
          s3Key: key,
        };
      } catch {
        track = {
          id,
          title: filenameWithoutExtension(key),
          artist: '',
          album: '',
          duration: 0,
          sourceTag: 's3',
          sourceName,
          artworkId: null,
          s3Key: key,
        };
      }

      tracks.push(track);
      scanned++;
    }

    if (tracks.length > 0) {
      await putTracks(tracks);
    }

    self.postMessage({ type: 'progress', scanned, total, errors });
  }

  if (!cancelled) {
    self.postMessage({ type: 'complete', scanned, total });
    self.close();
  }
}

async function scanLocal(source: LocalSource, sourceName: string): Promise<void> {
  let files: { handle: FileSystemFileHandle; path: string }[];
  try {
    files = await collectLocalFiles(source.handle);
  } catch (err) {
    self.postMessage({ type: 'error', message: String(err) });
    return;
  }

  const total = files.length;
  let scanned = 0;
  let errors = 0;

  for (let i = 0; i < files.length; i += BATCH_SIZE) {
    if (cancelled) break;

    const batch = files.slice(i, i + BATCH_SIZE);
    const tracks: Track[] = [];

    for (const { handle, path } of batch) {
      if (cancelled) break;

      const id = await stableTrackId(sourceName, 'local', path);
      let buffer: ArrayBuffer;
      let fileSize: number;

      try {
        const file = await handle.getFile();
        fileSize = file.size;
        const slice = file.slice(0, RANGE_BYTES);
        buffer = await slice.arrayBuffer();
      } catch {
        errors++;
        continue;
      }

      let track: Track;
      try {
        const metadata = await parseBuffer(
          new Uint8Array(buffer),
          fileSize ? { size: fileSize } : undefined,
          { duration: true, skipCovers: false },
        );
        const { title, artist, album } = metadata.common;
        const duration = metadata.format.duration ?? 0;
        const cover = metadata.common.picture?.[0];

        let artworkId: string | null = null;
        if (cover) {
          const blob = new Blob(
            [
              cover.data.buffer.slice(
                cover.data.byteOffset,
                cover.data.byteOffset + cover.data.byteLength,
              ) as ArrayBuffer,
            ],
            { type: cover.format },
          );
          await putArtwork(id, blob);
          artworkId = id;
        }

        track = {
          id,
          title: title || filenameWithoutExtension(path),
          artist: artist || '',
          album: album || '',
          duration,
          sourceTag: 'local',
          sourceName,
          artworkId,
          localPath: path,
        };
      } catch {
        track = {
          id,
          title: filenameWithoutExtension(path),
          artist: '',
          album: '',
          duration: 0,
          sourceTag: 'local',
          sourceName,
          artworkId: null,
          localPath: path,
        };
      }

      tracks.push(track);
      scanned++;
    }

    if (tracks.length > 0) {
      await putTracks(tracks);
    }

    self.postMessage({ type: 'progress', scanned, total, errors });
  }

  if (!cancelled) {
    self.postMessage({ type: 'complete', scanned, total });
    self.close();
  }
}


async function scanGDrive(source: { folderId: string; folderName?: string; token: string }, sourceName: string): Promise<void> {
  let files: { id: string; name: string; mimeType: string; size?: string }[];
  try {
    const res = await fetch(`${BASE_URL}/api/gdrive/files?folderId=${encodeURIComponent(source.folderId)}&token=${encodeURIComponent(source.token)}`, {
      signal: AbortSignal.timeout(180000),
    });
    if (!res.ok) {
      if (res.status === 401) throw new Error('Google session expired. Sign in again.');
      throw new Error(`list failed: ${res.status}`);
    }
    const data: unknown = await res.json();
    // Backend sends `null` for an empty listing; treat anything non-array as empty.
    files = Array.isArray(data)
      ? (data as { id: string; name: string; mimeType: string; size?: string }[])
      : [];
  } catch (err) {
    self.postMessage({ type: 'error', message: String(err) });
    return;
  }

  const total = files.length;
  let scanned = 0;
  let errors = 0;

  for (let i = 0; i < files.length; i += BATCH_SIZE) {
    if (cancelled) break;
    const batch = files.slice(i, i + BATCH_SIZE);
    const tracks: Track[] = [];
    for (const f of batch) {
      if (cancelled) break;
      const id = await stableTrackId(sourceName, 'gdrive', f.id);
      let buffer: ArrayBuffer;
      let totalSize: number | undefined;
      try {
        const res = await fetch(`${BASE_URL}/api/gdrive/stream?fileId=${encodeURIComponent(f.id)}&token=${encodeURIComponent(source.token)}`, {
          headers: { Range: `bytes=0-${RANGE_BYTES - 1}` },
        });
        if (res.status === 401) {
          // Session died mid-scan: stop instead of counting every file as an error.
          self.postMessage({ type: 'error', message: 'Google session expired. Sign in again.' });
          return;
        }
        if (!res.ok && res.status !== 206) {
          errors++;
          continue;
        }
        totalSize = totalSizeFromResponse(res);
        buffer = await res.arrayBuffer();
      } catch {
        errors++;
        continue;
      }

      let track: Track;
      try {
        const metadata = await parseBuffer(
          new Uint8Array(buffer),
          totalSize ? { size: totalSize } : (f.size ? { size: Number(f.size) } : undefined),
          { duration: true, skipCovers: false },
        );
        const { title, artist, album } = metadata.common;
        const duration = metadata.format.duration ?? 0;
        const cover = metadata.common.picture?.[0];
        let artworkId: string | null = null;
        if (cover) {
          const blob = new Blob([cover.data.buffer.slice(cover.data.byteOffset, cover.data.byteOffset + cover.data.byteLength) as ArrayBuffer], { type: cover.format });
          await putArtwork(id, blob);
          artworkId = id;
        }
        track = {
          id,
          title: title || filenameWithoutExtension(f.name),
          artist: artist || '',
          album: album || '',
          duration,
          sourceTag: 'gdrive',
          sourceName,
          artworkId,
          gdriveFileId: f.id,
        };
      } catch {
        track = {
          id,
          title: filenameWithoutExtension(f.name),
          artist: '',
          album: '',
          duration: 0,
          sourceTag: 'gdrive',
          sourceName,
          artworkId: null,
          gdriveFileId: f.id,
        };
      }
      tracks.push(track);
      scanned++;
    }
    if (tracks.length > 0) await putTracks(tracks);
    self.postMessage({ type: 'progress', scanned, total, errors });
  }
  if (!cancelled) {
    self.postMessage({ type: 'complete', scanned, total });
    self.close();
  }
}

// ── Message handler ──────────────────────────────────────────────────────────

self.addEventListener('message', (event: MessageEvent) => {
  const msg = event.data as {
    type: string;
    source?: S3Config | LocalSource | { folderId: string; folderName?: string; token: string };
    sourceName?: string;
  };

  if (msg.type === 'cancel') {
    cancelled = true;
    return;
  }

  if (msg.type === 'scan') {
    const source = msg.source!;
    // Distinguish S3Config from LocalSource by presence of 'handle'
    if ('handle' in source) {
      const sourceName = msg.sourceName ?? source.handle.name;
      scanLocal(source, sourceName).catch((err) => {
        self.postMessage({ type: 'error', message: String(err) });
      });
    } else if ('folderId' in source) {
      const cfg = source;
      const sourceName = msg.sourceName ?? `gdrive-${cfg.folderId}`;
      scanGDrive(cfg, sourceName).catch((err) => {
        self.postMessage({ type: 'error', message: String(err) });
      });
    } else {
      const cfg = source;
      const sourceName = msg.sourceName ?? s3SourceId(cfg);
      scanS3(cfg, sourceName).catch((err) => {
        self.postMessage({ type: 'error', message: String(err) });
      });
    }
  }
});
