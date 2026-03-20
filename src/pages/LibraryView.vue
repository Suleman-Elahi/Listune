<template>
  <div class="library-view">
    <!-- Header -->
    <div class="lib-header">
      <q-btn flat round icon="arrow_back" color="white" size="sm" @click="$router.push('/')" />
      <span class="lib-app-name">Music Player</span>
      <q-btn unelevated rounded label="Import" color="primary" size="sm" class="import-btn" @click="showAddSource = true" />
    </div>

    <!-- Search -->
    <div class="lib-search">
      <div class="search-input-wrap">
        <q-icon name="search" color="grey-5" size="18px" class="search-icon" />
        <input
          v-model="searchQuery"
          class="search-input"
          placeholder="Search your library..."
          @input="onSearch"
        />
      </div>
    </div>

    <!-- Filter tabs -->
    <div class="lib-tabs">
      <button
        v-for="tab in tabs"
        :key="tab"
        class="lib-tab"
        :class="{ active: activeTab === tab }"
        @click="activeTab = tab"
      >{{ tab }}</button>
    </div>

    <!-- Scan progress -->
    <div v-if="libraryStore.isScanning" class="scan-progress">
      <q-linear-progress :value="scanRatio" color="light-green-6" track-color="grey-9" class="q-mb-xs" />
      <div class="scan-label">
        Scanning {{ libraryStore.scanProgress?.scanned ?? 0 }} / {{ libraryStore.scanProgress?.total ?? '...' }}
        <span v-if="(libraryStore.scanProgress?.errors ?? 0) > 0" class="text-warning"> · {{ libraryStore.scanProgress?.errors }} errors</span>
      </div>
    </div>

    <!-- Library heading -->
    <div class="lib-heading">
      <div class="lib-heading-title">Library</div>
      <div class="lib-heading-sub">{{ libraryStore.results.length }} COLLECTED TRACKS</div>
    </div>

    <!-- Track list -->
    <div class="track-list-wrap">
      <q-virtual-scroll
        v-if="filteredTracks.length > 0"
        :items="filteredTracks"
        :virtual-scroll-item-size="72"
        style="height: 100%"
        @virtual-scroll="onVirtualScroll"
      >
        <template #default="{ item: track, index }">
          <div
            :key="track.id"
            class="track-row"
            :class="{ 'track-row--active': track.id === playerStore.currentTrackId }"
            @click="playTrack(track.id)"
          >
            <span class="track-num">{{ String(index + 1).padStart(2, '0') }}</span>
            <div class="track-art">
              <img v-if="artworkUrls[track.id]" :src="artworkUrls[track.id]" />
              <q-icon v-else name="music_note" color="grey-6" size="20px" />
            </div>
            <div class="track-info">
              <div class="track-title">{{ track.title || 'Unknown' }}</div>
              <div class="track-artist">{{ track.artist || 'Unknown artist' }}</div>
            </div>
            <span class="track-duration">{{ formatDuration(track.duration) }}</span>
            <q-btn flat round dense icon="add" color="grey-5" size="xs" @click.stop="enqueueTrack(track.id)" />
            <q-btn flat round dense icon="more_vert" color="grey-5" size="xs" @click.stop />
          </div>
        </template>
      </q-virtual-scroll>

      <!-- Empty state -->
      <div v-else-if="!libraryStore.isScanning" class="empty-state">
        <q-icon name="library_music" size="56px" color="grey-7" />
        <p>No tracks yet</p>
        <q-btn unelevated rounded color="primary" label="Add a source" @click="showAddSource = true" />
      </div>
    </div>

    <AddSourceDialog v-model="showAddSource" />
    <BottomNav />
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, watch } from 'vue';
import { useLibraryStore } from 'src/stores/libraryStore';
import { usePlayerStore } from 'src/stores/playerStore';
import { db } from 'src/services/db';
import AddSourceDialog from 'src/components/AddSourceDialog.vue';
import BottomNav from 'src/components/BottomNav.vue';
import type { Track } from 'src/types/models';

const libraryStore = useLibraryStore();
const playerStore = usePlayerStore();

const showAddSource = ref(false);
const searchQuery = ref('');
const artworkUrls = ref<Record<string, string>>({});

const tabs = ['All Songs', 'Artists', 'Albums'] as const;
type Tab = typeof tabs[number];
const activeTab = ref<Tab>('All Songs');

const scanRatio = computed(() => {
  const p = libraryStore.scanProgress;
  if (!p || p.total === 0) return 0;
  return p.scanned / p.total;
});

const filteredTracks = computed<Track[]>(() => libraryStore.results);

function onSearch() {
  libraryStore.search(searchQuery.value);
}

function formatDuration(seconds: number): string {
  if (!seconds || !isFinite(seconds)) return '';
  const m = Math.floor(seconds / 60);
  const s = Math.floor(seconds % 60);
  return `${m}:${s.toString().padStart(2, '0')}`;
}

async function playTrack(id: string) {
  if (!playerStore.queue.includes(id)) {
    playerStore.setQueue(libraryStore.results.map((t) => t.id));
  }
  await playerStore.play(id);
}

function enqueueTrack(id: string) {
  playerStore.enqueue(id);
}

async function loadArtwork(trackId: string) {
  if (artworkUrls.value[trackId]) return;
  try {
    const blob = await db.getArtwork(trackId);
    if (blob) artworkUrls.value[trackId] = URL.createObjectURL(blob);
  } catch { /* no artwork */ }
}

function onVirtualScroll({ to }: { to: number }) {
  const visible = libraryStore.results.slice(Math.max(0, to - 5), to + 10);
  visible.forEach((t) => { if (t.artworkId) loadArtwork(t.id); });
  if (to >= libraryStore.results.length - 10) libraryStore.loadMore();
}

watch(() => libraryStore.results, (results) => {
  results.slice(0, 20).forEach((t) => { if (t.artworkId) loadArtwork(t.id); });
}, { immediate: true });

watch(() => libraryStore.isScanning, (scanning) => {
  if (!scanning) libraryStore.search(searchQuery.value);
});

onMounted(() => { libraryStore.search(''); });
</script>

<style scoped lang="scss">
.library-view {
  position: fixed;
  inset: 0;
  background: #111318;
  display: flex;
  flex-direction: column;
  color: white;
  font-family: inherit;
}

/* ── Header ── */
.lib-header {
  display: flex;
  align-items: center;
  padding: 12px 16px 8px;
  gap: 8px;
  flex-shrink: 0;
}

.lib-app-name {
  flex: 1;
  font-size: 20px;
  font-weight: 700;
  color: #fff;
}

.import-btn {
  background: #3b82f6 !important;
  font-size: 13px;
  padding: 4px 16px;
  border-radius: 20px !important;
}

/* ── Search ── */
.lib-search {
  padding: 4px 16px 12px;
  flex-shrink: 0;
}

.search-input-wrap {
  display: flex;
  align-items: center;
  background: #1e2028;
  border-radius: 12px;
  padding: 10px 14px;
  gap: 10px;
}

.search-icon {
  flex-shrink: 0;
}

.search-input {
  flex: 1;
  background: transparent;
  border: none;
  outline: none;
  color: #fff;
  font-size: 15px;

  &::placeholder {
    color: rgba(255, 255, 255, 0.35);
  }
}

/* ── Tabs ── */
.lib-tabs {
  display: flex;
  gap: 8px;
  padding: 0 16px 16px;
  flex-shrink: 0;
}

.lib-tab {
  padding: 7px 18px;
  border-radius: 20px;
  border: 1px solid rgba(255, 255, 255, 0.15);
  background: transparent;
  color: rgba(255, 255, 255, 0.6);
  font-size: 14px;
  cursor: pointer;
  transition: all 0.15s ease;

  &.active {
    background: #3b82f6;
    border-color: #3b82f6;
    color: #fff;
    font-weight: 600;
  }
}

/* ── Scan progress ── */
.scan-progress {
  padding: 0 16px 8px;
  flex-shrink: 0;
}

.scan-label {
  font-size: 11px;
  color: rgba(255, 255, 255, 0.45);
}

/* ── Library heading ── */
.lib-heading {
  padding: 4px 16px 16px;
  flex-shrink: 0;
}

.lib-heading-title {
  font-size: 36px;
  font-weight: 800;
  line-height: 1.1;
  color: #fff;
}

.lib-heading-sub {
  font-size: 12px;
  color: rgba(255, 255, 255, 0.4);
  letter-spacing: 0.05em;
  margin-top: 2px;
}

/* ── Track list ── */
.track-list-wrap {
  flex: 1;
  min-height: 0;
  overflow: hidden;
}

.track-row {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 10px 16px;
  cursor: pointer;
  transition: background 0.1s ease;

  &:hover {
    background: rgba(255, 255, 255, 0.05);
  }

  &--active {
    background: rgba(59, 130, 246, 0.12);
  }
}

.track-num {
  font-size: 13px;
  color: rgba(255, 255, 255, 0.3);
  min-width: 24px;
  text-align: right;
  flex-shrink: 0;
}

.track-art {
  width: 44px;
  height: 44px;
  border-radius: 8px;
  background: #1e2028;
  flex-shrink: 0;
  overflow: hidden;
  display: flex;
  align-items: center;
  justify-content: center;

  img {
    width: 100%;
    height: 100%;
    object-fit: cover;
  }
}

.track-info {
  flex: 1;
  min-width: 0;
}

.track-title {
  font-size: 14px;
  font-weight: 500;
  color: #fff;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.track-artist {
  font-size: 12px;
  color: rgba(255, 255, 255, 0.45);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  margin-top: 2px;
}

.track-duration {
  font-size: 13px;
  color: rgba(255, 255, 255, 0.4);
  flex-shrink: 0;
  min-width: 32px;
  text-align: right;
}

/* ── Empty state ── */
.empty-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  height: 100%;
  gap: 12px;
  color: rgba(255, 255, 255, 0.4);

  p { margin: 0; font-size: 15px; }
}

/* ── Mini player styles removed — handled by BottomNav ── */
</style>
