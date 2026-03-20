// Feature: xp-music-player — PlayerStore
// Requirements: 1.6, 1.7, 1.8, 3.6, 3.7, 3.8, 8.1, 8.2, 8.3, 8.4

import { defineStore } from 'pinia';
import { markRaw, watch } from 'vue';
import { audioEngine, _audioElement } from 'src/services/audioEngine';
import { db } from 'src/services/db';
import { s3Client } from 'src/services/s3Client';

// Resolve a local track path to a blob URL using stored FileSystemDirectoryHandle
async function resolveLocalUrl(localPath: string): Promise<string | null> {
  const sources = await db.getSources();
  for (const source of sources) {
    if (source.type !== 'local') continue;
    const handle = (source.config as { handle: FileSystemDirectoryHandle }).handle;
    try {
      // localPath is relative like "subfolder/song.mp3" or "song.mp3"
      const parts = localPath.split('/').filter(Boolean);
      let dir: FileSystemDirectoryHandle = handle;
      for (let i = 0; i < parts.length - 1; i++) {
        dir = await dir.getDirectoryHandle(parts[i]!);
      }
      const fileName = parts[parts.length - 1]!;
      const fileHandle = await dir.getFileHandle(fileName);
      const file = await fileHandle.getFile();
      return URL.createObjectURL(file);
    } catch {
      // try next source
    }
  }
  return null;
}
// Req 3.8: Wrap non-serializable audio objects with markRaw();
// stored as a module-level variable, outside the Pinia state tree
const _engine = markRaw(audioEngine);

// Interval handle for currentTime/duration polling (module-level, not in state)
let _intervalId: ReturnType<typeof setInterval> | null = null;

function _startInterval(store: { currentTime: number; duration: number }): void {
  if (_intervalId !== null) return;
  // Req 1.8: update currentTime and duration at most every 250ms
  _intervalId = setInterval(() => {
    store.currentTime = _audioElement.currentTime;
    store.duration = isFinite(_audioElement.duration) ? _audioElement.duration : 0;
  }, 250);
}

function _stopInterval(): void {
  if (_intervalId !== null) {
    clearInterval(_intervalId);
    _intervalId = null;
  }
}

export const usePlayerStore = defineStore('player', {
  state: () => ({
    // Req 3.6: queue contains only string IDs, no full Track objects
    currentTrackId: null as string | null,
    isPlaying: false,
    isLooping: false,
    queue: [] as string[],
    currentTime: 0,
    duration: 0,
    volume: 1,
  }),

  actions: {
    async play(trackId: string): Promise<void> {
      // Req 3.7: fetch Track metadata from DB by ID when needed
      const track = await db.getTrack(trackId);
      if (!track) return;

      let url: string;
      if (track.sourceTag === 's3' && track.s3Key) {
        url = await s3Client.getPresignedUrl(track.s3Key);
      } else if (track.sourceTag === 'local' && track.localPath) {
        const resolved = await resolveLocalUrl(track.localPath);
        if (!resolved) return; // handle not available (permission revoked)
        url = resolved;
      } else {
        return;
      }

      await _engine.loadAndPlay(url);

      this.currentTrackId = trackId;
      this.isPlaying = true;

      // Req 1.8: start 250ms polling interval for currentTime/duration
      _startInterval(this);
    },

    pause(): void {
      _engine.pause();
      this.isPlaying = false;
      _stopInterval();
    },

    resume(): void {
      _engine.resume();
      this.isPlaying = true;
      _startInterval(this);
    },

    seek(seconds: number): void {
      _engine.seek(seconds);
      this.currentTime = seconds;
    },

    toggleLoop(): void {
      this.isLooping = !this.isLooping;
    },

    async next(): Promise<void> {
      // If looping single track, restart current
      if (this.isLooping && this.currentTrackId) {
        this.seek(0);
        _engine.resume();
        this.isPlaying = true;
        _startInterval(this);
        return;
      }
      const idx = this.currentTrackId ? this.queue.indexOf(this.currentTrackId) : -1;
      const nextIdx = idx + 1;
      if (nextIdx < this.queue.length) {
        const nextId = this.queue[nextIdx];
        if (nextId) await this.play(nextId);
      } else {
        // Req 1.7: queue exhausted — stop playback
        this.isPlaying = false;
        _stopInterval();
      }
    },

    async previous(): Promise<void> {
      if (this.currentTime > 3) {
        // Restart current track if more than 3 seconds in
        this.seek(0);
      } else {
        const idx = this.currentTrackId ? this.queue.indexOf(this.currentTrackId) : -1;
        const prevIdx = idx - 1;
        if (prevIdx >= 0) {
          const prevId = this.queue[prevIdx];
          if (prevId) await this.play(prevId);
        } else {
          // Already at first track — restart it
          this.seek(0);
        }
      }
    },

    setVolume(v: number): void {
      _engine.setVolume(v);
      this.volume = Math.min(1, Math.max(0, v));
    },

    // Req 3.6: queue stores only string IDs
    setQueue(ids: string[]): void {
      this.queue = ids;
    },

    enqueue(id: string): void {
      this.queue.push(id);
    },
  },
});

// Req 1.6: Subscribe to audioEngine 'ended' event to auto-advance queue
// Req 1.7: set isPlaying=false when queue is exhausted
// Use lazy pinia access so this works regardless of store initialization order
_engine.on('ended', () => {
  import('pinia').then(({ getActivePinia }) => {
    const pinia = getActivePinia();
    if (!pinia) return;
    const store = usePlayerStore(pinia);
    store.next().catch(() => undefined);
  }).catch(() => undefined);
});

// Req 8.1–8.4: Media Session integration
// Call once after the store is created (e.g. from App.vue or a boot file).
// All calls are guarded by `'mediaSession' in navigator` (Req 8.4).
export function setupMediaSession(store: ReturnType<typeof usePlayerStore>): void {
  // Req 8.4: skip entirely if Media Session API is unavailable
  if (!('mediaSession' in navigator)) return;

  // Req 8.2: Register action handlers delegating to AudioEngine / store
  navigator.mediaSession.setActionHandler('play', () => {
    store.resume();
  });
  navigator.mediaSession.setActionHandler('pause', () => {
    store.pause();
  });
  navigator.mediaSession.setActionHandler('previoustrack', () => {
    store.previous().catch(() => undefined);
  });
  navigator.mediaSession.setActionHandler('nexttrack', () => {
    store.next().catch(() => undefined);
  });

  // Req 8.1: When currentTrackId changes, update mediaSession.metadata
  watch(
    () => store.currentTrackId,
    async (trackId) => {
      if (!('mediaSession' in navigator)) return;
      if (!trackId) {
        navigator.mediaSession.metadata = null;
        return;
      }

      const track = await db.getTrack(trackId);
      if (!track) return;

      const artworkList: MediaImage[] = [];
      if (track.artworkId) {
        try {
          const blob = await db.getArtwork(track.artworkId);
          if (blob) {
            const url = URL.createObjectURL(blob);
            artworkList.push({ src: url });
          }
        } catch {
          // artwork fetch failure is non-fatal
        }
      }

      navigator.mediaSession.metadata = new MediaMetadata({
        title: track.title,
        artist: track.artist,
        album: track.album,
        artwork: artworkList,
      });
    },
  );

  // Req 8.3: When currentTime or duration changes, call setPositionState
  // Ensure position <= duration to satisfy the API constraint
  watch(
    () => [store.currentTime, store.duration] as [number, number],
    ([currentTime, duration]) => {
      if (!('mediaSession' in navigator)) return;
      if (duration <= 0) return;
      try {
        navigator.mediaSession.setPositionState({
          duration,
          position: Math.min(currentTime, duration),
          playbackRate: 1,
        });
      } catch {
        // setPositionState may throw if values are invalid; ignore silently
      }
    },
  );
}
