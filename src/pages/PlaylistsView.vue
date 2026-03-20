<template>
  <div class="playlists-view">
    <!-- Header -->
    <div class="pl-header">
      <q-btn flat round icon="arrow_back" color="white" size="sm" @click="$router.push('/')" />
      <span class="pl-app-name">Music Player</span>
    </div>

    <!-- Heading -->
    <div class="pl-heading">
      <div class="pl-heading-sub">YOUR COLLECTIONS</div>
      <div class="pl-heading-title">Playlists</div>
    </div>

    <!-- Create button -->
    <div class="pl-create-wrap">
      <button class="pl-create-btn" @click="showCreate = true">
        <q-icon name="add_circle_outline" size="22px" />
        <span>Create New Playlist</span>
      </button>
    </div>

    <!-- Search -->
    <div class="pl-search">
      <div class="search-input-wrap">
        <q-icon name="search" color="grey-5" size="18px" />
        <input v-model="searchQuery" class="search-input" placeholder="Search your library..." />
      </div>
    </div>

    <!-- Sort tabs -->
    <div class="pl-tabs">
      <button
        v-for="tab in sortTabs"
        :key="tab"
        class="pl-tab"
        :class="{ active: activeSort === tab }"
        @click="activeSort = tab"
      >{{ tab }}</button>
    </div>

    <!-- Playlist grid -->
    <div class="pl-list-wrap">
      <div v-if="filteredPlaylists.length === 0" class="pl-empty">
        <q-icon name="queue_music" size="56px" color="grey-7" />
        <p>No playlists yet</p>
        <q-btn unelevated rounded color="primary" label="Create one" @click="showCreate = true" />
      </div>

      <div v-else class="pl-grid">
        <div
          v-for="pl in filteredPlaylists"
          :key="pl.id"
          class="pl-card"
          @click="openPlaylist(pl)"
        >
          <div class="pl-card-art">
            <img v-if="pl.coverUrl" :src="pl.coverUrl" />
            <q-icon v-else name="queue_music" color="grey-5" size="28px" />
          </div>
          <div class="pl-card-name">{{ pl.name }}</div>
          <div class="pl-card-count">{{ pl.trackIds.length }} tracks</div>
        </div>
      </div>
    </div>

    <!-- Create playlist dialog -->
    <q-dialog v-model="showCreate" no-focus no-refocus>
      <q-card dark style="min-width: 280px; background: #1e2230; border-radius: 16px;">
        <q-card-section>
          <div style="font-size: 16px; font-weight: 700; color: #fff; margin-bottom: 12px;">New Playlist</div>
          <input
            v-model="newPlaylistName"
            class="create-input"
            placeholder="Playlist name..."
            @keyup.enter="createPlaylist"
          />
        </q-card-section>
        <q-card-actions align="right">
          <q-btn flat label="Cancel" color="grey-5" v-close-popup />
          <q-btn unelevated rounded label="Create" color="primary" @click="createPlaylist" />
        </q-card-actions>
      </q-card>
    </q-dialog>

    <!-- Playlist detail dialog -->
    <q-dialog v-model="showDetail" full-height position="bottom" no-focus no-refocus>
      <q-card dark style="width: 100%; max-width: 100%; background: #111318; border-radius: 20px 20px 0 0; max-height: 80vh;">
        <q-card-section class="pl-detail-header">
          <q-btn flat round icon="close" color="white" size="sm" v-close-popup />
          <span class="pl-detail-name">{{ activePlaylist?.name }}</span>
          <q-btn flat round icon="more_vert" color="white" size="sm" />
        </q-card-section>
        <q-card-section style="padding: 0; overflow-y: auto; max-height: calc(80vh - 60px);">
          <div v-if="activePlaylistTracks.length === 0" style="text-align:center; padding: 32px; color: rgba(255,255,255,0.4);">
            No tracks in this playlist
          </div>
          <div
            v-for="(track, idx) in activePlaylistTracks"
            :key="track.id"
            class="pl-track-row"
            @click="playFromPlaylist(idx)"
          >
            <span class="track-num">{{ String(idx + 1).padStart(2, '0') }}</span>
            <div class="track-art">
              <img v-if="detailArtwork[track.id]" :src="detailArtwork[track.id]" />
              <q-icon v-else name="music_note" color="grey-6" size="18px" />
            </div>
            <div class="track-info">
              <div class="track-title">{{ track.title }}</div>
              <div class="track-artist">{{ track.artist }}</div>
            </div>
            <span class="track-dur">{{ formatDuration(track.duration) }}</span>
            <q-btn flat round dense icon="more_vert" color="grey-5" size="xs" @click.stop />
          </div>
        </q-card-section>
      </q-card>
    </q-dialog>

    <BottomNav />
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch } from 'vue';
import { useRouter } from 'vue-router';
import { usePlayerStore } from 'src/stores/playerStore';
import { db } from 'src/services/db';
import BottomNav from 'src/components/BottomNav.vue';
import type { Track } from 'src/types/models';

const router = useRouter();
const playerStore = usePlayerStore();

// ── Playlist data model (stored in localStorage for simplicity) ──
interface Playlist {
  id: string;
  name: string;
  trackIds: string[];
  coverUrl?: string;
  createdAt: number;
}

function loadPlaylists(): Playlist[] {
  try { return JSON.parse(localStorage.getItem('playlists') || '[]'); } catch { return []; }
}
function savePlaylists(pls: Playlist[]) {
  localStorage.setItem('playlists', JSON.stringify(pls));
}

const playlists = ref<Playlist[]>(loadPlaylists());
const searchQuery = ref('');
const sortTabs = ['Recently Played', 'Alphabetical', 'By Artist'] as const;
type SortTab = typeof sortTabs[number];
const activeSort = ref<SortTab>('Recently Played');

const showCreate = ref(false);
const newPlaylistName = ref('');

const showDetail = ref(false);
const activePlaylist = ref<Playlist | null>(null);
const activePlaylistTracks = ref<Track[]>([]);
const detailArtwork = ref<Record<string, string>>({});

const filteredPlaylists = computed(() => {
  let list = playlists.value;
  if (searchQuery.value) {
    const q = searchQuery.value.toLowerCase();
    list = list.filter((p) => p.name.toLowerCase().includes(q));
  }
  if (activeSort.value === 'Alphabetical') {
    list = [...list].sort((a, b) => a.name.localeCompare(b.name));
  }
  return list;
});

function createPlaylist() {
  const name = newPlaylistName.value.trim();
  if (!name) return;
  const pl: Playlist = { id: crypto.randomUUID(), name, trackIds: [], createdAt: Date.now() };
  playlists.value.unshift(pl);
  savePlaylists(playlists.value);
  newPlaylistName.value = '';
  showCreate.value = false;
}

async function openPlaylist(pl: Playlist) {
  activePlaylist.value = pl;
  showDetail.value = true;
  const tracks: Track[] = [];
  for (const id of pl.trackIds) {
    const t = await db.getTrack(id);
    if (t) tracks.push(t);
  }
  activePlaylistTracks.value = tracks;
  // load artwork
  for (const t of tracks) {
    if (t.artworkId && !detailArtwork.value[t.id]) {
      const blob = await db.getArtwork(t.artworkId);
      if (blob) detailArtwork.value[t.id] = URL.createObjectURL(blob);
    }
  }
}

async function playFromPlaylist(startIdx: number) {
  if (!activePlaylist.value) return;
  playerStore.setQueue(activePlaylist.value.trackIds);
  const id = activePlaylist.value.trackIds[startIdx];
  if (id) await playerStore.play(id);
  showDetail.value = false;
  router.push('/');
}

function formatDuration(seconds: number): string {
  if (!seconds || !isFinite(seconds)) return '';
  const m = Math.floor(seconds / 60);
  const s = Math.floor(seconds % 60);
  return `${m}:${s.toString().padStart(2, '0')}`;
}
</script>

<style scoped lang="scss">
.playlists-view {
  position: fixed;
  inset: 0;
  background: #111318;
  display: flex;
  flex-direction: column;
  color: white;
  overflow: hidden;
}

.pl-header {
  display: flex;
  align-items: center;
  padding: 12px 16px 4px;
  gap: 8px;
  flex-shrink: 0;
}

.pl-app-name {
  flex: 1;
  font-size: 20px;
  font-weight: 700;
}

.pl-heading {
  padding: 12px 20px 4px;
  flex-shrink: 0;
}

.pl-heading-sub {
  font-size: 11px;
  color: rgba(255, 255, 255, 0.4);
  letter-spacing: 0.08em;
  margin-bottom: 4px;
}

.pl-heading-title {
  font-size: 36px;
  font-weight: 800;
}

.pl-create-wrap {
  padding: 12px 16px;
  flex-shrink: 0;
}

.pl-create-btn {
  width: 100%;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 10px;
  padding: 16px;
  background: linear-gradient(135deg, #1e3a8a 0%, #3b82f6 100%);
  border: none;
  border-radius: 14px;
  color: #fff;
  font-size: 16px;
  font-weight: 600;
  cursor: pointer;
  transition: opacity 0.15s;

  &:hover { opacity: 0.9; }
}

.pl-search {
  padding: 0 16px 10px;
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

.search-input {
  flex: 1;
  background: transparent;
  border: none;
  outline: none;
  color: #fff;
  font-size: 14px;

  &::placeholder { color: rgba(255, 255, 255, 0.3); }
}

.pl-tabs {
  display: flex;
  gap: 8px;
  padding: 0 16px 12px;
  flex-shrink: 0;
  overflow-x: auto;

  &::-webkit-scrollbar { display: none; }
}

.pl-tab {
  padding: 7px 16px;
  border-radius: 20px;
  border: 1px solid rgba(255, 255, 255, 0.15);
  background: transparent;
  color: rgba(255, 255, 255, 0.55);
  font-size: 13px;
  white-space: nowrap;
  cursor: pointer;
  transition: all 0.15s;

  &.active {
    background: #fff;
    border-color: #fff;
    color: #111;
    font-weight: 700;
  }
}

.pl-list-wrap {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  padding: 0 16px 120px;
}

.pl-empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  height: 200px;
  gap: 12px;
  color: rgba(255, 255, 255, 0.4);

  p { margin: 0; font-size: 15px; }
}

.pl-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 14px;
  padding-top: 4px;
}

.pl-card {
  background: #1e2028;
  border-radius: 14px;
  overflow: hidden;
  cursor: pointer;
  transition: transform 0.15s;

  &:hover { transform: scale(1.02); }
}

.pl-card-art {
  width: 100%;
  aspect-ratio: 1;
  background: #2a2d3e;
  display: flex;
  align-items: center;
  justify-content: center;
  overflow: hidden;

  img { width: 100%; height: 100%; object-fit: cover; }
}

.pl-card-name {
  font-size: 13px;
  font-weight: 600;
  color: #fff;
  padding: 8px 10px 2px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.pl-card-count {
  font-size: 11px;
  color: rgba(255, 255, 255, 0.4);
  padding: 0 10px 10px;
}

/* Detail sheet */
.pl-detail-header {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 12px 16px;
  border-bottom: 1px solid rgba(255, 255, 255, 0.07);
}

.pl-detail-name {
  flex: 1;
  font-size: 16px;
  font-weight: 700;
}

.pl-track-row {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 10px 16px;
  cursor: pointer;

  &:hover { background: rgba(255, 255, 255, 0.05); }
}

.track-num {
  font-size: 12px;
  color: rgba(255, 255, 255, 0.3);
  min-width: 22px;
  text-align: right;
  flex-shrink: 0;
}

.track-art {
  width: 40px;
  height: 40px;
  border-radius: 8px;
  background: #1e2028;
  flex-shrink: 0;
  overflow: hidden;
  display: flex;
  align-items: center;
  justify-content: center;

  img { width: 100%; height: 100%; object-fit: cover; }
}

.track-info { flex: 1; min-width: 0; }

.track-title {
  font-size: 13px;
  font-weight: 500;
  color: #fff;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.track-artist {
  font-size: 11px;
  color: rgba(255, 255, 255, 0.4);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.track-dur {
  font-size: 12px;
  color: rgba(255, 255, 255, 0.35);
  flex-shrink: 0;
}

.create-input {
  width: 100%;
  background: #2a2d3e;
  border: 1px solid rgba(255, 255, 255, 0.1);
  border-radius: 10px;
  padding: 10px 14px;
  color: #fff;
  font-size: 15px;
  outline: none;

  &::placeholder { color: rgba(255, 255, 255, 0.3); }
}
</style>
