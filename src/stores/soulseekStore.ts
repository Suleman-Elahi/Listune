import { defineStore } from 'pinia';
import { computed, ref } from 'vue';
import {
  soulseek,
  type SoulseekJob,
  type SoulseekResult,
  type SoulseekSelection,
  type SoulseekStatus,
} from 'src/services/soulseek';
import { useLibraryStore } from 'src/stores/libraryStore';
import { db } from 'src/services/db';

const POLL_MS = 2000;
const OPTS_KEY = 'listune_soulseek_opts';

export interface SoulseekOpts {
  mode: 'mp3' | 'flac';
  quality: string;
}

function loadOpts(): SoulseekOpts {
  try {
    const stored = JSON.parse(localStorage.getItem(OPTS_KEY) || '{}');
    return {
      mode: stored.mode === 'flac' ? 'flac' : 'mp3',
      quality: typeof stored.quality === 'string' ? stored.quality : '',
    };
  } catch {
    return { mode: 'mp3', quality: '' };
  }
}

/** One faded placeholder row: a file found on Soulseek that is not (yet) in the library. */
export interface SoulseekGhost {
  key: string;
  query: string;
  resultIndex: number;
  fileIndex: number;
  name: string;
  title: string;
  artist: string;
  bitrate: number | null;
  sizeMb: string;
  status: 'found' | 'downloading' | 'failed';
  error: string;
}

export const useSoulseekStore = defineStore('soulseek', () => {
  // Persisted options (mp3/flac + quality), edited in the search options popup.
  const opts = ref<SoulseekOpts>(loadOpts());

  function saveOpts(next: SoulseekOpts): void {
    opts.value = {
      mode: next.mode === 'flac' ? 'flac' : 'mp3',
      quality: next.mode === 'flac' ? '' : next.quality,
    };
    localStorage.setItem(OPTS_KEY, JSON.stringify(opts.value));
  }

  const status = ref<SoulseekStatus | null>(null);
  const statusError = ref('');

  // Search state
  const searching = ref(false);
  const searchError = ref('');
  const searchingFor = ref('');
  const searchId = ref('');
  const searchQuery = ref('');
  const lastResults = ref<SoulseekResult[]>([]);

  // Download jobs
  const jobs = ref<SoulseekJob[]>([]);
  const downloadError = ref('');
  let pollTimer: ReturnType<typeof setInterval> | null = null;

  // Normalized titles of tracks already in the library (server downloads),
  // used to drop ghost placeholders whose song is now available for real.
  const libraryTitles = ref<Set<string>>(new Set());

  async function refreshLibraryTitles(): Promise<void> {
    try {
      const tracks = await db.queryTracks({});
      libraryTitles.value = new Set(
        tracks
          .filter((t) => t.sourceTag === 'server')
          .map((t) => normalizeTitle(t.title)),
      );
    } catch {
      // keep last known set
    }
  }

  // Last login error, shown in the options popup.
  const loginError = ref('');

  async function loadStatus(): Promise<void> {
    try {
      status.value = await soulseek.status();
      statusError.value = status.value.error || '';
    } catch {
      status.value = null;
      statusError.value = 'Backend unreachable';
    }
  }

  async function login(account: string, password: string): Promise<boolean> {
    loginError.value = '';
    const res = await soulseek.login(account, password);
    if (res.success) {
      await loadStatus();
      return true;
    }
    loginError.value = res.error || 'Login failed';
    return false;
  }

  async function runSearch(query: string): Promise<void> {
    const q = query.trim();
    if (!q) return;
    searching.value = true;
    searchingFor.value = q;
    searchError.value = '';
    try {
      const res = await soulseek.search({
        query: q,
        mode: opts.value.mode,
        ...(opts.value.quality ? { quality: opts.value.quality } : {}),
      });
      if (res.error) {
        searchError.value = res.error;
        clearSearch();
      } else {
        await refreshLibraryTitles();
        lastResults.value = res.results.map(normalizeResult);
        if (lastResults.value.length === 0) {
          searchError.value = `Nothing found on Soulseek for "${q}"`;
        }
      }
      searchQuery.value = q;
      searchId.value = res.searchId || '';
    } catch {
      searchError.value = 'Soulseek search failed';
    } finally {
      searching.value = false;
    }
  }

  interface rawResult {
    resultIndex: number;
    user: string;
    folder: string;
    files: { fileIndex: number; name: string; size: number; bitrate: number | null }[];
    avgBitrate: number;
    sizeMb: number;
    speedKbs: number;
    slots: boolean;
  }

  function normalizeResult(r: rawResult): SoulseekResult {
    return {
      resultIndex: r.resultIndex,
      user: r.user || '',
      folder: r.folder || '',
      files: (r.files || []).map((f) => ({
        fileIndex: f.fileIndex,
        name: f.name || '',
        size: f.size || 0,
        bitrate: f.bitrate ?? null,
      })),
      avgBitrate: r.avgBitrate || 0,
      sizeMb: r.sizeMb || 0,
      speedKbs: r.speedKbs || 0,
      slots: r.slots !== false,
    };
  }

  function clearSearch(): void {
    searchId.value = '';
    searchQuery.value = '';
    lastResults.value = [];
  }

  async function download(selections: SoulseekSelection[]): Promise<void> {
    if (!searchId.value || selections.length === 0) return;
    downloadError.value = '';
    try {
      const res = await soulseek.download(searchId.value, selections);
      if (res.error) {
        downloadError.value = res.error;
        return;
      }
      await refreshJobs();
      startPolling();
    } catch {
      downloadError.value = 'Failed to start download';
    }
  }

  async function downloadFile(ghost: { resultIndex: number; fileIndex: number }): Promise<void> {
    await download([{ resultIndex: ghost.resultIndex, fileIndex: ghost.fileIndex }]);
  }

  async function downloadResult(resultIndex: number): Promise<void> {
    await download([{ resultIndex }]);
  }

  async function refreshJobs(): Promise<void> {
    try {
      const list = await soulseek.jobs();
      const hadRunning = jobs.value.some((j) => j.status === 'running');
      jobs.value = list.sort((a, b) => (a.created < b.created ? 1 : -1));
      const hasRunning = list.some((j) => j.status === 'running');
      if (hadRunning && !hasRunning) {
        // A job just finished: pull new songs into the library, then let the
        // ghost list drop the placeholders whose songs are now real tracks.
        void (async () => {
          await useLibraryStore().syncServerLibrary();
          await refreshLibraryTitles();
        })();
      }
      if (!hasRunning) stopPolling();
    } catch {
      // keep last known jobs
    }
  }

  function startPolling(): void {
    if (pollTimer !== null) return;
    void refreshJobs();
    pollTimer = setInterval(() => {
      void refreshJobs();
    }, POLL_MS);
  }

  function stopPolling(): void {
    if (pollTimer !== null) {
      clearInterval(pollTimer);
      pollTimer = null;
    }
  }

  async function cancelJob(id: string): Promise<void> {
    await soulseek.cancelJob(id);
    await refreshJobs();
  }

  // ── Ghost rows (faded placeholders under the library results) ──

  /** Map "file name" -> job file status so ghosts track their own progress. */
  const fileStatusByName = computed<Record<string, { status: string; error: string }>>(() => {
    const map: Record<string, { status: string; error: string }> = {};
    for (const job of jobs.value) {
      for (const file of job.files) {
        // Later jobs win: a re-download overwrites an older failed attempt.
        map[file.name] = { status: file.status, error: file.error || '' };
      }
    }
    return map;
  });

  const hasResults = computed(() => lastResults.value.length > 0);

  /** Ghost rows for all searched files that have not finished downloading. */
  const ghosts = computed<SoulseekGhost[]>(() => {
    if (!searchQuery.value) return [];
    const out: SoulseekGhost[] = [];
    for (const result of lastResults.value) {
      for (const file of result.files) {
        const jobFile = fileStatusByName.value[file.name];
        // Finished files disappear: the library shows the real track instead.
        if (jobFile && (jobFile.status === 'done' || jobFile.status === 'skipped')) continue;
        const title = cleanTrackTitle(file.name);
        // Song is already in the library (e.g. from an older session) -> skip.
        if (libraryTitles.value.has(normalizeTitle(title))) continue;
        out.push({
          key: `${result.resultIndex}:${file.fileIndex}:${file.name}`,
          query: searchQuery.value,
          resultIndex: result.resultIndex,
          fileIndex: file.fileIndex,
          name: file.name,
          title,
          artist: guessArtist(result, file.name),
          bitrate: file.bitrate,
          sizeMb: formatMb(file.size),
          status: toGhostStatus(jobFile?.status),
          error: jobFile?.error || '',
        });
      }
    }
    return out;
  });

  function toGhostStatus(jobStatus?: string): SoulseekGhost['status'] {
    if (jobStatus === 'downloading' || jobStatus === 'pending') return 'downloading';
    if (jobStatus === 'failed') return 'failed';
    return 'found';
  }

  return {
    opts,
    saveOpts,
    status,
    statusError,
    loginError,
    searching,
    searchingFor,
    searchError,
    searchId,
    searchQuery,
    jobs,
    downloadError,
    hasResults,
    ghosts,
    loadStatus,
    login,
    runSearch,
    clearSearch,
    download,
    downloadFile,
    downloadResult,
    refreshJobs,
    refreshLibraryTitles,
    cancelJob,
    startPolling,
  };
});

/** "01 - Pink Floyd - Time.mp3" -> clean title without extension. */
function cleanTrackTitle(name: string): string {
  return name
    .replace(/\.(mp3|flac)$/i, '')
    .replace(/^\d{1,3}[._ -]+/, '')
    .replace(/_/g, ' ')
    .trim() || name;
}

/** Case/punctuation-insensitive form used to compare ghost vs library titles. */
function normalizeTitle(title: string): string {
  return (title || '').toLowerCase().replace(/[^a-z0-9]+/g, ' ').trim();
}

/** Artist guess: "Artist - Title" in the file name, else the folder name. */
function guessArtist(result: SoulseekResult, name: string): string {
  const base = name.replace(/\.(mp3|flac)$/i, '').replace(/_/g, ' ').trim();
  const dash = base.match(/^(.{1,40}?)\s+-\s+/);
  if (dash && dash[1]) return dash[1].trim();
  return result.folder.trim() || result.user;
}

function formatMb(bytes: number): string {
  if (bytes >= 1024 * 1024) return `${Math.round(bytes / 1024 / 1024)} mb`;
  if (bytes > 0) return `${Math.round(bytes / 1024)} kb`;
  return '';
}
