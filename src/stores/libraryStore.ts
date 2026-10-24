import { defineStore } from 'pinia';
import { ref, toRaw } from 'vue';
import type { Track, ScanProgress, S3Config, LocalSource } from 'src/types/models';
import { db } from 'src/services/db';

export const useLibraryStore = defineStore('library', () => {
  const searchQuery = ref('');
  const results = ref<Track[]>([]);
  const scanProgress = ref<ScanProgress | null>(null);
  const isScanning = ref(false);

  async function search(query: string): Promise<void> {
    searchQuery.value = query;
    results.value = await db.queryTracks({ search: query, limit: 50 });
  }

  async function loadMore(): Promise<void> {
    const offset = results.value.length;
    const more = await db.queryTracks({
      search: searchQuery.value,
      limit: 50,
      offset,
    });
    results.value = [...results.value, ...more];
  }

  function startScan(source: S3Config | LocalSource, sourceName?: string): void {
    isScanning.value = true;
    scanProgress.value = null;

    const worker = new Worker(
      new URL('../workers/scanner.worker.ts', import.meta.url),
      { type: 'module' },
    );

    worker.onmessage = (event: MessageEvent) => {
      const msg = event.data;
      if (msg.type === 'progress') {
        scanProgress.value = {
          scanned: msg.scanned,
          total: msg.total,
          errors: msg.errors,
        };
      } else if (msg.type === 'complete') {
        isScanning.value = false;
        worker.terminate();
      } else if (msg.type === 'error') {
        isScanning.value = false;
        worker.terminate();
      }
    };

    worker.onerror = () => {
      isScanning.value = false;
      worker.terminate();
    };

    // Sources can come from reactive UI state; proxies are not structured-cloneable.
    worker.postMessage({ type: 'scan', source: toRaw(source), sourceName });
  }

  return {
    searchQuery,
    results,
    scanProgress,
    isScanning,
    search,
    loadMore,
    startScan,
  };
});
