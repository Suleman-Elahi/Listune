// Thin client for the Soulseek endpoints of the Listune backend.
// The backend shells out to the soulseek-bridge (slsk-client) for all
// network traffic; credentials live in the system keychain.

const BASE_URL = import.meta.env.VITE_BACKEND_URL || 'http://localhost:8090';

export interface SoulseekResultFile {
  fileIndex: number;
  name: string;
  size: number;
  bitrate: number | null;
}

export interface SoulseekResult {
  resultIndex: number;
  user: string;
  folder: string;
  files: SoulseekResultFile[];
  avgBitrate: number;
  sizeMb: number;
  speedKbs: number;
  slots: boolean;
}

export interface SoulseekSearchResponse {
  searchId: string;
  query: string;
  mode: string;
  results: SoulseekResult[];
  error?: string;
}

export interface SoulseekStatus {
  loggedIn: boolean;
  account: string;
  activeJobs?: number;
  error?: string;
}

export interface SoulseekJobFile {
  name: string;
  path?: string;
  status: 'pending' | 'downloading' | 'done' | 'skipped' | 'failed';
  error?: string;
}

export interface SoulseekJob {
  id: string;
  query?: string;
  status: 'running' | 'done' | 'failed' | 'cancelled';
  files: SoulseekJobFile[];
  downloaded: number;
  failed: number;
  destination: string;
  created: string;
}

/** One selection inside a cached search: a whole folder or a single file. */
export interface SoulseekSelection {
  resultIndex: number;
  fileIndex?: number;
}

export const soulseek = {
  /** Whether Soulseek credentials are configured (keychain or env). */
  async status(): Promise<SoulseekStatus> {
    const res = await fetch(`${BASE_URL}/api/soulseek/status`, {
      signal: AbortSignal.timeout(20000),
    });
    return res.json() as Promise<SoulseekStatus>;
  },

  /** Store Soulseek credentials in the system keychain. */
  async login(account: string, password: string): Promise<{ success: boolean; error?: string }> {
    const res = await fetch(`${BASE_URL}/api/soulseek/login`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ account, password }),
      signal: AbortSignal.timeout(30000),
    });
    return res.json();
  },

  /** Run a Soulseek search. Takes ~10s: the bridge waits for peer results. */
  async search(params: {
    query: string;
    mode: 'mp3' | 'flac';
    quality?: string;
    timeoutMs?: number;
  }): Promise<SoulseekSearchResponse> {
    const res = await fetch(`${BASE_URL}/api/soulseek/search`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(params),
      signal: AbortSignal.timeout(120000),
    });
    return res.json() as Promise<SoulseekSearchResponse>;
  },

  /** Start a download job for selected search results. */
  async download(searchId: string, items: SoulseekSelection[]): Promise<{ jobId: string; files: number; error?: string }> {
    const res = await fetch(`${BASE_URL}/api/soulseek/download`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ searchId, items }),
      signal: AbortSignal.timeout(15000),
    });
    return res.json();
  },

  /** List download jobs. */
  async jobs(): Promise<SoulseekJob[]> {
    const res = await fetch(`${BASE_URL}/api/soulseek/jobs`, {
      signal: AbortSignal.timeout(10000),
    });
    const data = (await res.json()) as { jobs?: SoulseekJob[] };
    return Array.isArray(data.jobs) ? data.jobs : [];
  },

  /** Fetch one download job. */
  async job(id: string): Promise<SoulseekJob | null> {
    const res = await fetch(`${BASE_URL}/api/soulseek/jobs/${encodeURIComponent(id)}`, {
      signal: AbortSignal.timeout(10000),
    });
    if (!res.ok) return null;
    return res.json() as Promise<SoulseekJob>;
  },

  /** Cancel a running download job. */
  async cancelJob(id: string): Promise<boolean> {
    const res = await fetch(`${BASE_URL}/api/soulseek/jobs/${encodeURIComponent(id)}/cancel`, {
      method: 'POST',
      signal: AbortSignal.timeout(10000),
    });
    return res.ok;
  },
};
