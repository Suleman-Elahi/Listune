// Thin client for the YouTube endpoints of the Listune backend.
// Search uses the signed-in user's Google session; downloads are converted
// to MP3 server-side with go-ytdlp and land in the server library.

const BASE_URL =
  import.meta.env.VITE_BACKEND_URL ||
  (import.meta.env.PROD ? self.location.origin : 'http://localhost:8090');

export interface YouTubeVideo {
  videoId: string;
  title: string;
  channelTitle: string;
  description?: string;
  publishedAt?: string;
  thumbnailUrl?: string;
}

export interface YouTubeSearchResponse {
  searchId: string;
  query: string;
  results: YouTubeVideo[];
  nextPageToken?: string;
  prevPageToken?: string;
  error?: string;
}

export interface YouTubeStatus {
  available: boolean;
  error?: string;
}

export interface YouTubeJobFile {
  name: string;
  videoId: string;
  path?: string;
  status: 'pending' | 'downloading' | 'done' | 'failed';
  progress?: number;
  error?: string;
}

export interface YouTubeJob {
  id: string;
  query?: string;
  status: 'running' | 'done' | 'failed' | 'cancelled';
  files: YouTubeJobFile[];
  downloaded: number;
  failed: number;
  destination: string;
  created: string;
}

export const youtube = {
  /** Whether the supplied Google session handle can be used for search. */
  async status(token: string): Promise<YouTubeStatus> {
    const res = await fetch(
      `${BASE_URL}/api/youtube/status?token=${encodeURIComponent(token)}`,
      { signal: AbortSignal.timeout(10000) },
    );
    return res.json() as Promise<YouTubeStatus>;
  },

  /** Search YouTube videos with the YouTube Data API. */
  async search(
    query: string,
    token: string,
    pageToken?: string,
  ): Promise<YouTubeSearchResponse> {
    const res = await fetch(
      `${BASE_URL}/api/youtube/search?token=${encodeURIComponent(token)}`,
      {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          query,
          ...(pageToken ? { pageToken } : {}),
        }),
        signal: AbortSignal.timeout(30000),
      },
    );
    return res.json() as Promise<YouTubeSearchResponse>;
  },

  /** Start an MP3 conversion job for videos from a cached search. */
  async download(
    searchId: string,
    videoIds: string[],
    token: string,
    quality?: string,
  ): Promise<{ jobId: string; files: number; error?: string }> {
    const res = await fetch(
      `${BASE_URL}/api/youtube/download?token=${encodeURIComponent(token)}`,
      {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          searchId,
          videos: videoIds.map((videoId) => ({ videoId })),
          ...(quality ? { quality } : {}),
        }),
        signal: AbortSignal.timeout(15000),
      },
    );
    return res.json();
  },

  /** List download jobs. */
  async jobs(): Promise<YouTubeJob[]> {
    const res = await fetch(`${BASE_URL}/api/youtube/jobs`, {
      signal: AbortSignal.timeout(10000),
    });
    const data = (await res.json()) as { jobs?: YouTubeJob[] };
    return Array.isArray(data.jobs) ? data.jobs : [];
  },

  /** Fetch one download job. */
  async job(id: string): Promise<YouTubeJob | null> {
    const res = await fetch(`${BASE_URL}/api/youtube/jobs/${encodeURIComponent(id)}`, {
      signal: AbortSignal.timeout(10000),
    });
    if (!res.ok) return null;
    return res.json() as Promise<YouTubeJob>;
  },

  /** Cancel a running download job. */
  async cancelJob(id: string): Promise<boolean> {
    const res = await fetch(
      `${BASE_URL}/api/youtube/jobs/${encodeURIComponent(id)}/cancel`,
      { method: 'POST', signal: AbortSignal.timeout(10000) },
    );
    return res.ok;
  },
};
