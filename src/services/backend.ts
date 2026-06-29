// Thin client for the Listune backend server.
// All heavy logic (OAuth token exchange, AcoustID, MusicBrainz) lives server-side.

const BASE_URL = import.meta.env.VITE_BACKEND_URL || 'http://localhost:8090';

export interface IdentifyResult {
  title?: string;
  artist?: string;
  album?: string;
  year?: string;
  genre?: string;
  coverArtUrl?: string;
  mbid?: string;
  releaseId?: string;
  score?: number;
  fingerprint?: string;
  duration?: number;
  allArtists?: string[];
  error?: string;
}

export interface ServerTrackMeta {
  id: string;
  title: string;
  artist: string;
  album: string;
  duration: number;
  year?: number;
  genre?: string;
  hasArtwork: boolean;
  serverPath: string;
}

export interface ScanResult {
  tracks: ServerTrackMeta[];
  total: number;
  errors: number;
  error?: string;
}

export const backend = {
  /** The backend base URL (for constructing proxy URLs etc.) */
  baseUrl: BASE_URL,

  /** Redirect user to Google SSO. */
  loginGoogle(): void {
    window.location.href = `${BASE_URL}/auth/google`;
  },

  /** Scan a server-side folder for audio files. */
  async scanServerFolder(folderPath: string): Promise<ScanResult> {
    const res = await fetch(`${BASE_URL}/api/library/scan`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ folderPath }),
    });
    return res.json() as Promise<ScanResult>;
  },

  /** Get the streaming URL for a server-hosted track. */
  streamUrl(serverPath: string): string {
    return `${BASE_URL}/api/stream?path=${encodeURIComponent(serverPath)}`;
  },

  /** Get the artwork URL for a server-hosted track. */
  artworkUrl(serverPath: string): string {
    return `${BASE_URL}/api/artwork?path=${encodeURIComponent(serverPath)}`;
  },

  /** Write ID3 tags to a file (local server path or S3). */
  async writeTags(params: {
    filePath?: string;
    s3Key?: string;
    s3Endpoint?: string;
    s3Bucket?: string;
    s3Region?: string;
    s3Access?: string;
    s3Secret?: string;
    title?: string;
    artist?: string;
    album?: string;
    year?: string;
    genre?: string;
    coverArtUrl?: string;
  }): Promise<{ success: boolean; error?: string }> {
    const res = await fetch(`${BASE_URL}/api/write-tags`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(params),
    });
    return res.json();
  },

  /** Identify a track by uploading an audio file to the backend.
   *  Backend runs fpcalc → AcoustID → MusicBrainz and returns metadata. */
  async identifyTrack(audioBlob: Blob, filename: string): Promise<IdentifyResult> {
    const form = new FormData();
    form.append('audio', audioBlob, filename);
    const res = await fetch(`${BASE_URL}/api/identify`, {
      method: 'POST',
      body: form,
    });
    return res.json() as Promise<IdentifyResult>;
  },

  /** Identify a server-side file by path (no upload needed). */
  async identifyServerFile(serverPath: string): Promise<IdentifyResult> {
    // Fetch the file from the stream endpoint and re-upload for identification
    const res = await fetch(`${BASE_URL}/api/stream?path=${encodeURIComponent(serverPath)}`);
    if (!res.ok) return { error: 'failed to fetch file' };
    const blob = await res.blob();
    const filename = serverPath.split('/').pop() || 'track.mp3';
    return this.identifyTrack(blob, filename);
  },

  /** Check if backend is reachable. */
  async healthCheck(): Promise<boolean> {
    try {
      const res = await fetch(`${BASE_URL}/health`, { signal: AbortSignal.timeout(3000) });
      return res.ok;
    } catch {
      return false;
    }
  },

  /** Load persisted user state by key. Returns null if not found. */
  async getState<T>(key: string): Promise<T | null> {
    try {
      const res = await fetch(`${BASE_URL}/api/state?key=${encodeURIComponent(key)}`);
      if (!res.ok) return null;
      return await res.json() as T;
    } catch {
      return null;
    }
  },

  /** Save user state by key. */
  async putState(key: string, value: unknown): Promise<boolean> {
    try {
      const res = await fetch(`${BASE_URL}/api/state?key=${encodeURIComponent(key)}`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(value),
      });
      return res.ok;
    } catch {
      return false;
    }
  },
};
