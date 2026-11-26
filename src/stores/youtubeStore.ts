import { defineStore } from 'pinia';
import { computed, ref } from 'vue';
import { youtube, type YouTubeJob, type YouTubeVideo } from 'src/services/youtube';
import { useLibraryStore } from 'src/stores/libraryStore';
import { db } from 'src/services/db';

const POLL_MS = 2000;

function getGoogleToken(): string | null {
  try {
    const raw = localStorage.getItem('listune_user');
    if (!raw) return null;
    const user = JSON.parse(raw) as { token?: string };
    return user.token || null;
  } catch {
    return null;
  }
}

/** One faded placeholder row: a YouTube video not (yet) in the library. */
export interface YouTubeGhost {
  key: string;
  videoId: string;
  title: string;
  artist: string;
  thumbnailUrl: string;
  status: 'found' | 'downloading' | 'failed';
  progress: number;
  error: string;
}

export const useYouTubeStore = defineStore('youtube', () => {
  const available = ref(false);
  const availabilityError = ref('');
  const searching = ref(false);
  const searchError = ref('');
  const searchQuery = ref('');
  const searchId = ref('');
  const results = ref<YouTubeVideo[]>([]);

  const jobs = ref<YouTubeJob[]>([]);
  const downloadError = ref('');
  let pollTimer: ReturnType<typeof setInterval> | null = null;
  let initializedJobs = false;

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

  async function loadStatus(): Promise<void> {
    const token = getGoogleToken();
    if (!token) {
      available.value = false;
      availabilityError.value = 'Sign in with Google to search YouTube';
      return;
    }
    try {
      const res = await youtube.status(token);
      available.value = res.available;
      availabilityError.value = res.available ? '' : res.error || 'Sign in again to search YouTube';
    } catch {
      available.value = false;
      availabilityError.value = 'Backend unreachable';
    }
  }

  async function runSearch(query: string): Promise<void> {
    const q = query.trim();
    if (!q) return;
    const token = getGoogleToken();
    if (!token) {
      searchError.value = 'Sign in with Google to search YouTube';
      return;
    }
    searching.value = true;
    searchError.value = '';
    try {
      const res = await youtube.search(q, token);
      if (res.error) {
        searchError.value = res.error;
        clearSearch();
      } else {
        await refreshLibraryTitles();
        results.value = (res.results || []).map(normalizeVideo);
        if (results.value.length === 0) {
          searchError.value = `Nothing found on YouTube for "${q}"`;
        }
        searchQuery.value = q;
        searchId.value = res.searchId || '';
      }
    } catch {
      searchError.value = 'YouTube search failed';
    } finally {
      searching.value = false;
    }
  }

  function normalizeVideo(v: YouTubeVideo): YouTubeVideo {
    return {
      videoId: v.videoId || '',
      title: v.title || '',
      channelTitle: v.channelTitle || '',
      description: v.description || '',
      publishedAt: v.publishedAt || '',
      thumbnailUrl: v.thumbnailUrl || '',
    };
  }

  function clearSearch(): void {
    searchId.value = '';
    searchQuery.value = '';
    results.value = [];
  }

  const hasResults = computed(() => results.value.length > 0);

  /** Map "videoId" -> job file status so ghosts track their own progress. */
  const fileStatusById = computed<Record<string, YouTubeJobFileView>>(() => {
    const map: Record<string, YouTubeJobFileView> = {};
    for (const job of jobs.value) {
      for (const file of job.files) {
        map[file.videoId] = {
          status: file.status,
          progress: file.progress || 0,
          error: file.error || '',
        };
      }
    }
    return map;
  });

  interface YouTubeJobFileView {
    status: string;
    progress: number;
    error: string;
  }

  const ghosts = computed<YouTubeGhost[]>(() => {
    if (!searchQuery.value) return [];
    const out: YouTubeGhost[] = [];
    for (const video of results.value) {
      if (!video.videoId) continue;
      const jobFile = fileStatusById.value[video.videoId];
      if (jobFile && (jobFile.status === 'done' || jobFile.status === 'skipped')) continue;
      if (libraryTitles.value.has(normalizeTitle(video.title))) continue;
      out.push({
        key: video.videoId,
        videoId: video.videoId,
        title: video.title,
        artist: video.channelTitle,
        thumbnailUrl: video.thumbnailUrl || '',
        status: toGhostStatus(jobFile?.status),
        progress: jobFile?.progress || 0,
        error: jobFile?.error || '',
      });
    }
    return out;
  });

  function toGhostStatus(jobStatus?: string): YouTubeGhost['status'] {
    if (jobStatus === 'downloading' || jobStatus === 'pending') return 'downloading';
    if (jobStatus === 'failed') return 'failed';
    return 'found';
  }

  async function download(ghost: YouTubeGhost): Promise<void> {
    const token = getGoogleToken();
    if (!token || !searchId.value) {
      downloadError.value = 'Sign in with Google to download from YouTube';
      return;
    }
    downloadError.value = '';
    try {
      const res = await youtube.download(searchId.value, [ghost.videoId], token);
      if (res.error) {
        downloadError.value = res.error;
        return;
      }
      initializedJobs = true;
      await refreshJobs();
      startPolling();
    } catch {
      downloadError.value = 'Failed to start download';
    }
  }

  async function refreshJobs(): Promise<void> {
    try {
      const list = await youtube.jobs();
      const hadRunning = jobs.value.some((j) => j.status === 'running');
      const prevDone = jobs.value.reduce((n, j) => n + (j.downloaded || 0), 0);
      jobs.value = list.sort((a, b) => (a.created < b.created ? 1 : -1));
      const hasRunning = list.some((j) => j.status === 'running');
      const nextDone = list.reduce((n, j) => n + (j.downloaded || 0), 0);
      const finishedMore = initializedJobs && nextDone > prevDone;
      initializedJobs = true;
      if (finishedMore || (hadRunning && !hasRunning)) {
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
    await youtube.cancelJob(id);
    await refreshJobs();
  }

  return {
    available,
    availabilityError,
    searching,
    searchError,
    searchQuery,
    searchId,
    results,
    hasResults,
    ghosts,
    jobs,
    downloadError,
    loadStatus,
    runSearch,
    clearSearch,
    download,
    refreshJobs,
    refreshLibraryTitles,
    cancelJob,
  };
});

/** Case/punctuation-insensitive form used to compare ghosts vs library titles. */
function normalizeTitle(title: string): string {
  return (title || '').toLowerCase().replace(/[^a-z0-9]+/g, ' ').trim();
}
