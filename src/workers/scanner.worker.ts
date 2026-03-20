import { parseBuffer } from 'music-metadata-browser';
import { openDB, type IDBPDatabase } from 'idb';
import type { S3Config, LocalSource, Track, MusicPlayerDB } from 'src/types/models';

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

// ── SigV4 helpers (duplicated from s3Client for worker context) ──────────────

async function hmacSha256(key: ArrayBuffer, data: string): Promise<ArrayBuffer> {
  const cryptoKey = await crypto.subtle.importKey(
    'raw',
    key,
    { name: 'HMAC', hash: 'SHA-256' },
    false,
    ['sign'],
  );
  return crypto.subtle.sign('HMAC', cryptoKey, new TextEncoder().encode(data));
}

async function sha256Hex(data: string): Promise<string> {
  const buf = await crypto.subtle.digest('SHA-256', new TextEncoder().encode(data));
  return Array.from(new Uint8Array(buf))
    .map((b) => b.toString(16).padStart(2, '0'))
    .join('');
}

function toHex(buf: ArrayBuffer): string {
  return Array.from(new Uint8Array(buf))
    .map((b) => b.toString(16).padStart(2, '0'))
    .join('');
}

function formatDate(d: Date): string {
  return d.toISOString().slice(0, 10).replace(/-/g, '');
}

function formatDateTime(d: Date): string {
  return d.toISOString().replace(/[-:]/g, '').replace(/\.\d{3}/, '');
}

async function getPresignedUrl(cfg: S3Config, key: string, expiresIn = 3600): Promise<string> {
  const now = new Date();
  const dateStamp = formatDate(now);
  const amzDate = formatDateTime(now);
  const credentialScope = `${dateStamp}/${cfg.region}/s3/aws4_request`;
  const credential = `${cfg.accessKey}/${credentialScope}`;

  const queryParams: Record<string, string> = {
    'X-Amz-Algorithm': 'AWS4-HMAC-SHA256',
    'X-Amz-Credential': credential,
    'X-Amz-Date': amzDate,
    'X-Amz-Expires': String(expiresIn),
    'X-Amz-SignedHeaders': 'host',
  };

  const sortedKeys = Object.keys(queryParams).sort();
  const canonicalQueryString = sortedKeys
    .map((k) => `${encodeURIComponent(k)}=${encodeURIComponent(queryParams[k]!)}`)
    .join('&');

  const endpointUrl = new URL(cfg.endpoint);
  const host = endpointUrl.host;
  const canonicalUri = `/${cfg.bucket}/${key}`;
  const canonicalHeaders = `host:${host}\n`;
  const signedHeaders = 'host';
  const payloadHash = 'UNSIGNED-PAYLOAD';

  const canonicalRequest = [
    'GET',
    canonicalUri,
    canonicalQueryString,
    canonicalHeaders,
    signedHeaders,
    payloadHash,
  ].join('\n');

  const canonicalRequestHash = await sha256Hex(canonicalRequest);
  const stringToSign = [
    'AWS4-HMAC-SHA256',
    amzDate,
    credentialScope,
    canonicalRequestHash,
  ].join('\n');

  const kSecret = new TextEncoder().encode(`AWS4${cfg.secretKey}`).buffer as ArrayBuffer;
  const kDate = await hmacSha256(kSecret, dateStamp);
  const kRegion = await hmacSha256(kDate, cfg.region);
  const kService = await hmacSha256(kRegion, 's3');
  const kSigning = await hmacSha256(kService, 'aws4_request');

  const signatureBuf = await hmacSha256(kSigning, stringToSign);
  const signature = toHex(signatureBuf);

  return `${cfg.endpoint}/${cfg.bucket}/${key}?${canonicalQueryString}&X-Amz-Signature=${signature}`;
}

// ── S3 ListObjectsV2 ─────────────────────────────────────────────────────────

const AUDIO_EXTENSIONS = new Set(['.mp3', '.flac', '.ogg', '.m4a', '.wav', '.aac']);

async function listS3Objects(cfg: S3Config): Promise<string[]> {
  const keys: string[] = [];
  let continuationToken: string | null = null;

  do {
    const now = new Date();
    const dateStamp = formatDate(now);
    const amzDate = formatDateTime(now);
    const credentialScope = `${dateStamp}/${cfg.region}/s3/aws4_request`;
    const credential = `${cfg.accessKey}/${credentialScope}`;

    const queryParams: Record<string, string> = {
      'list-type': '2',
      'X-Amz-Algorithm': 'AWS4-HMAC-SHA256',
      'X-Amz-Credential': credential,
      'X-Amz-Date': amzDate,
      'X-Amz-Expires': '60',
      'X-Amz-SignedHeaders': 'host',
    };
    if (continuationToken) {
      queryParams['continuation-token'] = continuationToken;
    }

    const sortedKeys = Object.keys(queryParams).sort();
    const canonicalQueryString = sortedKeys
      .map((k) => `${encodeURIComponent(k)}=${encodeURIComponent(queryParams[k]!)}`)
      .join('&');

    const endpointUrl = new URL(cfg.endpoint);
    const host = endpointUrl.host;
    const canonicalUri = `/${cfg.bucket}`;
    const canonicalHeaders = `host:${host}\n`;
    const signedHeaders = 'host';
    const payloadHash = await sha256Hex('');

    const canonicalRequest = [
      'GET',
      canonicalUri,
      canonicalQueryString,
      canonicalHeaders,
      signedHeaders,
      payloadHash,
    ].join('\n');

    const canonicalRequestHash = await sha256Hex(canonicalRequest);
    const stringToSign = [
      'AWS4-HMAC-SHA256',
      amzDate,
      credentialScope,
      canonicalRequestHash,
    ].join('\n');

    const kSecret = new TextEncoder().encode(`AWS4${cfg.secretKey}`).buffer as ArrayBuffer;
    const kDate = await hmacSha256(kSecret, dateStamp);
    const kRegion = await hmacSha256(kDate, cfg.region);
    const kService = await hmacSha256(kRegion, 's3');
    const kSigning = await hmacSha256(kService, 'aws4_request');
    const signatureBuf = await hmacSha256(kSigning, stringToSign);
    const signature = toHex(signatureBuf);

    const url = `${cfg.endpoint}/${cfg.bucket}?${canonicalQueryString}&X-Amz-Signature=${signature}`;
    const response = await fetch(url);
    if (!response.ok) {
      throw new Error(`ListObjectsV2 failed: ${response.status} ${response.statusText}`);
    }

    const xml = await response.text();

    // Parse <Key> elements from XML
    const keyMatches = xml.matchAll(/<Key>([^<]+)<\/Key>/g);
    for (const match of keyMatches) {
      const key = match[1]!;
      const ext = key.slice(key.lastIndexOf('.')).toLowerCase();
      if (AUDIO_EXTENSIONS.has(ext)) {
        keys.push(key);
      }
    }

    // Check for continuation token
    const tokenMatch = xml.match(/<NextContinuationToken>([^<]+)<\/NextContinuationToken>/);
    continuationToken = tokenMatch ? tokenMatch[1]! : null;
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

// ── Main scan logic ──────────────────────────────────────────────────────────

const BATCH_SIZE = 20;
const RANGE_BYTES = 131072; // 128 KB

let cancelled = false;

async function scanS3(cfg: S3Config): Promise<void> {
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

      const id = crypto.randomUUID();
      let buffer: ArrayBuffer;

      try {
        const url = await getPresignedUrl(cfg, key);
        const response = await fetch(url, {
          headers: { Range: `bytes=0-${RANGE_BYTES - 1}` },
        });
        if (!response.ok) {
          errors++;
          continue;
        }
        buffer = await response.arrayBuffer();
      } catch {
        errors++;
        continue;
      }

      let track: Track;
      try {
        const metadata = await parseBuffer(new Uint8Array(buffer), undefined, {
          duration: true,
          skipCovers: false,
        });
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
          title: title || filenameWithoutExtension(key),
          artist: artist || '',
          album: album || '',
          duration,
          sourceTag: 's3',
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
    self.postMessage({ type: 'complete' });
    self.close();
  }
}

async function scanLocal(source: LocalSource): Promise<void> {
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

      const id = crypto.randomUUID();
      let buffer: ArrayBuffer;

      try {
        const file = await handle.getFile();
        const slice = file.slice(0, RANGE_BYTES);
        buffer = await slice.arrayBuffer();
      } catch {
        errors++;
        continue;
      }

      let track: Track;
      try {
        const metadata = await parseBuffer(new Uint8Array(buffer), undefined, {
          duration: true,
          skipCovers: false,
        });
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
          title: title || filenameWithoutExtension(path),
          artist: artist || '',
          album: album || '',
          duration,
          sourceTag: 'local',
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
    self.postMessage({ type: 'complete' });
    self.close();
  }
}

// ── Message handler ──────────────────────────────────────────────────────────

self.addEventListener('message', (event: MessageEvent) => {
  const msg = event.data as { type: string; source?: S3Config | LocalSource };

  if (msg.type === 'cancel') {
    cancelled = true;
    return;
  }

  if (msg.type === 'scan') {
    const source = msg.source!;
    // Distinguish S3Config from LocalSource by presence of 'handle'
    if ('handle' in source) {
      scanLocal(source as LocalSource).catch((err) => {
        self.postMessage({ type: 'error', message: String(err) });
      });
    } else {
      scanS3(source as S3Config).catch((err) => {
        self.postMessage({ type: 'error', message: String(err) });
      });
    }
  }
});
