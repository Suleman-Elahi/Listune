<template>
  <div class="library-view">
    <!-- Header -->
    <div class="lib-header">
      <q-btn flat round icon="arrow_back" color="white" size="sm" @click="$router.push('/')" />
      <span class="lib-app-name">Music Player</span>
      <q-btn unelevated rounded label="Import" color="primary" size="sm" class="import-btn" @click="showAddSource = true" />
      <q-btn
        v-if="untaggedCount > 0"
        unelevated
        rounded
        :label="isIdentifying ? `Identifying...` : `ID ${untaggedCount} tracks`"
        color="orange-8"
        size="sm"
        class="import-btn"
        :loading="isIdentifying"
        @click="identifyUntagged"
      />
      <q-btn flat round class="user-avatar-btn">
        <div class="user-avatar">
          <img v-if="user?.picture" :src="user.picture" referrerpolicy="no-referrer" />
          <q-icon v-else name="account_circle" color="grey-4" size="32px" />
        </div>
        <q-menu anchor="bottom right" self="top right">
          <q-list dense dark style="min-width: 180px; background: #1e2230;">
            <q-item v-if="user">
              <q-item-section>
                <q-item-label style="color: #fff; font-size: 13px;">{{ user.name }}</q-item-label>
                <q-item-label caption style="color: rgba(255,255,255,0.45); font-size: 11px;">{{ user.email }}</q-item-label>
              </q-item-section>
            </q-item>
            <q-separator dark />
            <q-item clickable v-close-popup @click="logout">
              <q-item-section avatar><q-icon name="logout" size="18px" color="red-4" /></q-item-section>
              <q-item-section style="color: #fff;">Log out</q-item-section>
            </q-item>
          </q-list>
        </q-menu>
      </q-btn>
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

    <!-- Track list (All Songs tab) -->
    <div v-if="activeTab === 'All Songs'" class="track-list-wrap">
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
            <q-btn flat round dense icon="add" color="grey-5" size="xs" @click.stop="showTrackMenu($event, track.id)">
              <q-menu anchor="bottom right" self="top right" class="track-menu">
                <q-list dense dark style="min-width: 180px; background: #1e2230;">
                  <q-item clickable v-close-popup @click="enqueueTrack(track.id)">
                    <q-item-section avatar><q-icon name="queue" size="18px" /></q-item-section>
                    <q-item-section>Add to queue</q-item-section>
                  </q-item>
                  <q-separator dark />
                  <q-item-label header style="color: rgba(255,255,255,0.4); font-size: 11px;">ADD TO PLAYLIST</q-item-label>
                  <q-item
                    v-for="pl in playlists"
                    :key="pl.id"
                    clickable
                    v-close-popup
                    @click="addToPlaylist(pl.id, track.id)"
                  >
                    <q-item-section avatar><q-icon name="queue_music" size="18px" /></q-item-section>
                    <q-item-section>{{ pl.name }}</q-item-section>
                  </q-item>
                  <q-item v-if="playlists.length === 0" disable>
                    <q-item-section class="text-grey-6" style="font-size: 12px;">No playlists yet</q-item-section>
                  </q-item>
                </q-list>
              </q-menu>
            </q-btn>
            <q-btn flat round dense icon="more_vert" color="grey-5" size="xs" @click.stop>
              <q-menu anchor="bottom right" self="top right">
                <q-list dense dark style="min-width: 200px; background: #1e2230;">
                  <q-item clickable v-close-popup @click="playNext(track.id)">
                    <q-item-section avatar><q-icon name="playlist_play" size="18px" color="white" /></q-item-section>
                    <q-item-section style="color: #fff;">Play next</q-item-section>
                  </q-item>
                  <q-item clickable v-close-popup @click="enqueueTrack(track.id)">
                    <q-item-section avatar><q-icon name="queue" size="18px" color="white" /></q-item-section>
                    <q-item-section style="color: #fff;">Add to queue</q-item-section>
                  </q-item>
                  <q-separator dark />
                  <q-item clickable v-close-popup @click="identifySingle(track)">
                    <q-item-section avatar><q-icon name="fingerprint" size="18px" color="orange-4" /></q-item-section>
                    <q-item-section style="color: #fff;">Identify track</q-item-section>
                  </q-item>
                  <q-item clickable v-close-popup @click="showTrackDetails(track)">
                    <q-item-section avatar><q-icon name="info_outline" size="18px" color="light-blue-4" /></q-item-section>
                    <q-item-section style="color: #fff;">Track info</q-item-section>
                  </q-item>
                  <q-separator dark />
                  <q-item clickable v-close-popup @click="removeTrack(track.id)">
                    <q-item-section avatar><q-icon name="delete_outline" size="18px" color="red-4" /></q-item-section>
                    <q-item-section style="color: #ef4444;">Remove from library</q-item-section>
                  </q-item>
                </q-list>
              </q-menu>
            </q-btn>
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

    <!-- Artists tab -->
    <div v-if="activeTab === 'Artists'" class="track-list-wrap">
      <!-- Artist detail view -->
      <div v-if="selectedArtist" class="group-detail">
        <div class="group-detail-header">
          <q-btn flat round icon="arrow_back" color="white" size="sm" @click="selectedArtist = null" />
          <div class="group-detail-info">
            <div class="group-detail-title">{{ selectedArtist }}</div>
            <div class="group-detail-sub">{{ artistTracks.length }} tracks</div>
          </div>
          <q-btn unelevated rounded label="Play all" color="primary" size="sm" @click="playAllArtistTracks" />
        </div>
        <div class="group-tracks-scroll">
          <div
            v-for="(track, index) in artistTracks"
            :key="track.id"
            class="track-row"
            :class="{ 'track-row--active': track.id === playerStore.currentTrackId }"
            @click="playArtistTrack(track.id)"
          >
            <span class="track-num">{{ String(index + 1).padStart(2, '0') }}</span>
            <div class="track-art">
              <img v-if="artworkUrls[track.id]" :src="artworkUrls[track.id]" />
              <q-icon v-else name="music_note" color="grey-6" size="20px" />
            </div>
            <div class="track-info">
              <div class="track-title">{{ track.title || 'Unknown' }}</div>
              <div class="track-artist">{{ track.album || 'Unknown album' }}</div>
            </div>
            <span class="track-duration">{{ formatDuration(track.duration) }}</span>
          </div>
        </div>
      </div>

      <!-- Artist list view -->
      <div v-else class="group-grid-scroll">
        <div v-if="artists.length > 0" class="group-grid">
          <div
            v-for="artist in filteredArtists"
            :key="artist.name"
            class="group-card"
            @click="openArtist(artist.name)"
          >
            <div class="group-card-art">
              <img v-if="artist.artworkId && artworkUrls[artist.artworkId]" :src="artworkUrls[artist.artworkId]" />
              <q-icon v-else name="person" color="grey-5" size="36px" />
            </div>
            <div class="group-card-name">{{ artist.name }}</div>
            <div class="group-card-sub">{{ artist.trackCount }} {{ artist.trackCount === 1 ? 'track' : 'tracks' }}</div>
          </div>
        </div>
        <div v-else class="empty-state">
          <q-icon name="person" size="56px" color="grey-7" />
          <p>No artists yet</p>
        </div>
      </div>
    </div>

    <!-- Albums tab -->
    <div v-if="activeTab === 'Albums'" class="track-list-wrap">
      <!-- Album detail view -->
      <div v-if="selectedAlbum" class="group-detail">
        <div class="group-detail-header">
          <q-btn flat round icon="arrow_back" color="white" size="sm" @click="selectedAlbum = null" />
          <div class="group-detail-info">
            <div class="group-detail-title">{{ selectedAlbum }}</div>
            <div class="group-detail-sub">{{ albumTracks.length }} tracks</div>
          </div>
          <q-btn unelevated rounded label="Play all" color="primary" size="sm" @click="playAllAlbumTracks" />
        </div>
        <div class="group-tracks-scroll">
          <div
            v-for="(track, index) in albumTracks"
            :key="track.id"
            class="track-row"
            :class="{ 'track-row--active': track.id === playerStore.currentTrackId }"
            @click="playAlbumTrack(track.id)"
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
          </div>
        </div>
      </div>

      <!-- Album list view -->
      <div v-else class="group-grid-scroll">
        <div v-if="albums.length > 0" class="group-grid">
          <div
            v-for="album in filteredAlbums"
            :key="album.name"
            class="group-card"
            @click="openAlbum(album.name)"
          >
            <div class="group-card-art">
              <img v-if="album.artworkId && artworkUrls[album.artworkId]" :src="artworkUrls[album.artworkId]" />
              <q-icon v-else name="album" color="grey-5" size="36px" />
            </div>
            <div class="group-card-name">{{ album.name }}</div>
            <div class="group-card-sub">{{ album.artist }} · {{ album.trackCount }} {{ album.trackCount === 1 ? 'track' : 'tracks' }}</div>
          </div>
        </div>
        <div v-else class="empty-state">
          <q-icon name="album" size="56px" color="grey-7" />
          <p>No albums yet</p>
        </div>
      </div>
    </div>

    <AddSourceDialog v-model="showAddSource" />

    <!-- Track info dialog -->
    <q-dialog v-model="trackDetailDialog" no-focus no-refocus>
      <q-card dark style="min-width: 300px; background: #1e2230; border-radius: 16px; color: #fff;">
        <q-card-section>
          <div style="font-size: 16px; font-weight: 700; margin-bottom: 12px;">Track Info</div>
          <div v-if="detailTrack" class="detail-grid">
            <div class="detail-row">
              <span class="detail-label">Title</span>
              <span class="detail-value">{{ detailTrack.title || '—' }}</span>
            </div>
            <div class="detail-row">
              <span class="detail-label">Artist</span>
              <span class="detail-value">{{ detailTrack.artist || '—' }}</span>
            </div>
            <div class="detail-row">
              <span class="detail-label">Album</span>
              <span class="detail-value">{{ detailTrack.album || '—' }}</span>
            </div>
            <div class="detail-row">
              <span class="detail-label">Duration</span>
              <span class="detail-value">{{ formatDuration(detailTrack.duration) || '—' }}</span>
            </div>
            <div class="detail-row">
              <span class="detail-label">Source</span>
              <span class="detail-value">{{ detailTrack.sourceTag }}</span>
            </div>
            <div v-if="detailTrack.localPath" class="detail-row">
              <span class="detail-label">Path</span>
              <span class="detail-value" style="word-break: break-all;">{{ detailTrack.localPath }}</span>
            </div>
            <div v-if="detailTrack.s3Key" class="detail-row">
              <span class="detail-label">S3 Key</span>
              <span class="detail-value" style="word-break: break-all;">{{ detailTrack.s3Key }}</span>
            </div>
          </div>
        </q-card-section>
        <q-card-actions align="right">
          <q-btn flat label="Close" color="grey-4" v-close-popup />
        </q-card-actions>
      </q-card>
    </q-dialog>

    <BottomNav />
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, watch } from 'vue';
import { useRouter } from 'vue-router';
import { useLibraryStore } from 'src/stores/libraryStore';
import { usePlayerStore } from 'src/stores/playerStore';
import { db } from 'src/services/db';
import AddSourceDialog from 'src/components/AddSourceDialog.vue';
import BottomNav from 'src/components/BottomNav.vue';
import { backend } from 'src/services/backend';
import type { Track } from 'src/types/models';

const router = useRouter();
const libraryStore = useLibraryStore();
const playerStore = usePlayerStore();

// User session
interface UserInfo { email: string; name: string; picture?: string; token: string; }
const user = ref<UserInfo | null>(null);

try {
  const stored = localStorage.getItem('listune_user');
  if (stored) user.value = JSON.parse(stored);
} catch { /* ignore */ }

function logout(): void {
  localStorage.removeItem('listune_user');
  router.push('/login');
}

const showAddSource = ref(false);
const searchQuery = ref('');
const artworkUrls = ref<Record<string, string>>({});

const tabs = ['All Songs', 'Artists', 'Albums'] as const;
type Tab = typeof tabs[number];
const activeTab = ref<Tab>('All Songs');

// ── Playlist integration ──
interface Playlist {
  id: string;
  name: string;
  trackIds: string[];
  createdAt: number;
}

function loadPlaylists(): Playlist[] {
  try { return JSON.parse(localStorage.getItem('playlists') || '[]'); } catch { return []; }
}
function savePlaylistsToStorage(pls: Playlist[]): void {
  localStorage.setItem('playlists', JSON.stringify(pls));
}

const playlists = ref<Playlist[]>(loadPlaylists());

function addToPlaylist(playlistId: string, trackId: string): void {
  const pl = playlists.value.find((p) => p.id === playlistId);
  if (!pl) return;
  if (!pl.trackIds.includes(trackId)) {
    pl.trackIds.push(trackId);
    savePlaylistsToStorage(playlists.value);
  }
}

function showTrackMenu(_event: Event, _trackId: string): void {
  // Refresh playlists in case they changed
  playlists.value = loadPlaylists();
}

const scanRatio = computed(() => {
  const p = libraryStore.scanProgress;
  if (!p || p.total === 0) return 0;
  return p.scanned / p.total;
});

const filteredTracks = computed<Track[]>(() => libraryStore.results);

// ── Artist & Album grouping ──
interface ArtistGroup { name: string; trackCount: number; artworkId: string | null; }
interface AlbumGroup { name: string; artist: string; trackCount: number; artworkId: string | null; }

const artists = ref<ArtistGroup[]>([]);
const albums = ref<AlbumGroup[]>([]);
const selectedArtist = ref<string | null>(null);
const selectedAlbum = ref<string | null>(null);
const artistTracks = ref<Track[]>([]);
const albumTracks = ref<Track[]>([]);

const filteredArtists = computed(() => {
  if (!searchQuery.value) return artists.value;
  const q = searchQuery.value.toLowerCase();
  return artists.value.filter((a) => a.name.toLowerCase().includes(q));
});

const filteredAlbums = computed(() => {
  if (!searchQuery.value) return albums.value;
  const q = searchQuery.value.toLowerCase();
  return albums.value.filter((a) => a.name.toLowerCase().includes(q) || a.artist.toLowerCase().includes(q));
});

async function loadArtists(): Promise<void> {
  artists.value = await db.getAllArtists();
  // Load artwork for artists
  for (const a of artists.value.slice(0, 30)) {
    if (a.artworkId) loadArtwork(a.artworkId);
  }
}

async function loadAlbums(): Promise<void> {
  albums.value = await db.getAllAlbums();
  // Load artwork for albums
  for (const a of albums.value.slice(0, 30)) {
    if (a.artworkId) loadArtwork(a.artworkId);
  }
}

async function openArtist(name: string): Promise<void> {
  selectedArtist.value = name;
  artistTracks.value = await db.getTracksByArtist(name === 'Unknown Artist' ? '' : name);
  artistTracks.value.forEach((t) => { if (t.artworkId) loadArtwork(t.id); });
}

async function openAlbum(name: string): Promise<void> {
  selectedAlbum.value = name;
  albumTracks.value = await db.getTracksByAlbum(name === 'Unknown Album' ? '' : name);
  albumTracks.value.forEach((t) => { if (t.artworkId) loadArtwork(t.id); });
}

async function playArtistTrack(id: string): Promise<void> {
  playerStore.setQueue(artistTracks.value.map((t) => t.id));
  await playerStore.play(id);
}

async function playAlbumTrack(id: string): Promise<void> {
  playerStore.setQueue(albumTracks.value.map((t) => t.id));
  await playerStore.play(id);
}

function playAllArtistTracks(): void {
  if (artistTracks.value.length === 0) return;
  playerStore.setQueue(artistTracks.value.map((t) => t.id));
  playerStore.play(artistTracks.value[0]!.id);
}

function playAllAlbumTracks(): void {
  if (albumTracks.value.length === 0) return;
  playerStore.setQueue(albumTracks.value.map((t) => t.id));
  playerStore.play(albumTracks.value[0]!.id);
}

watch(activeTab, (tab) => {
  selectedArtist.value = null;
  selectedAlbum.value = null;
  if (tab === 'Artists') loadArtists();
  else if (tab === 'Albums') loadAlbums();
});

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

function playNext(id: string) {
  const currentIdx = playerStore.currentTrackId
    ? playerStore.queue.indexOf(playerStore.currentTrackId)
    : -1;
  const insertAt = currentIdx + 1;
  const newQueue = [...playerStore.queue];
  // Remove if already in queue to avoid duplicates
  const existing = newQueue.indexOf(id);
  if (existing !== -1) newQueue.splice(existing, 1);
  newQueue.splice(insertAt, 0, id);
  playerStore.setQueue(newQueue);
}

async function removeTrack(id: string) {
  // Remove from IndexedDB
  const database = await import('src/services/db').then((m) => m.db);
  const track = await database.getTrack(id);
  if (track?.artworkId) {
    // Remove artwork too
    try {
      const idbModule = await import('idb');
      const dbInstance = await idbModule.openDB('music-player-db', 1);
      await dbInstance.delete('artwork', track.artworkId);
    } catch { /* ignore */ }
  }
  // Delete the track record
  const idbModule = await import('idb');
  const dbInstance = await idbModule.openDB('music-player-db', 1);
  await dbInstance.delete('tracks', id);

  // Remove from queue if present
  const queueIdx = playerStore.queue.indexOf(id);
  if (queueIdx !== -1) {
    const newQueue = [...playerStore.queue];
    newQueue.splice(queueIdx, 1);
    playerStore.setQueue(newQueue);
  }

  // Refresh library
  libraryStore.search(searchQuery.value);
}

const trackDetailDialog = ref(false);
const detailTrack = ref<Track | null>(null);

function showTrackDetails(track: Track) {
  detailTrack.value = track;
  trackDetailDialog.value = true;
}

async function identifySingle(track: Track) {
  let result;

  if (track.sourceTag === 'server' && track.serverPath) {
    // Server-side file — backend fetches and fingerprints it
    result = await backend.identifyServerFile(track.serverPath);
  } else if (track.sourceTag === 'local' && track.localPath) {
    let blob: Blob | null = null;
    const sources = await db.getSources();
    for (const source of sources) {
      if (source.type !== 'local') continue;
      const handle = (source.config as { handle: FileSystemDirectoryHandle }).handle;
      try {
        const parts = track.localPath.split('/').filter(Boolean);
        let dir: FileSystemDirectoryHandle = handle;
        for (let i = 0; i < parts.length - 1; i++) {
          dir = await dir.getDirectoryHandle(parts[i]!);
        }
        const fileHandle = await dir.getFileHandle(parts[parts.length - 1]!);
        blob = await fileHandle.getFile();
        break;
      } catch { /* try next */ }
    }
    if (!blob) return;
    const filename = track.localPath.split('/').pop() || 'track.mp3';
    result = await backend.identifyTrack(blob, filename);
  } else {
    return;
  }

  if (!result || result.error) return;

  const updated: Track = {
    ...track,
    title: result.title || track.title,
    artist: result.artist || track.artist,
    album: result.album || track.album,
  };

  // Download and store cover art if available
  if (result.coverArtUrl && !track.artworkId) {
    try {
      const artResp = await fetch(result.coverArtUrl);
      if (artResp.ok) {
        const artBlob = await artResp.blob();
        await db.putArtwork(track.id, artBlob);
        updated.artworkId = track.id;
      }
    } catch { /* cover art fetch failed, non-fatal */ }
  }

  await db.putTrack(updated);

  // Write tags to the actual file if it's on the server
  if (track.sourceTag === 'server' && track.serverPath) {
    await backend.writeTags({
      filePath: track.serverPath,
      title: result.title,
      artist: result.artist,
      album: result.album,
      year: result.year,
      genre: result.genre,
      coverArtUrl: result.coverArtUrl,
    });
  }

  libraryStore.search(searchQuery.value);
}

async function loadArtwork(trackId: string) {
  if (artworkUrls.value[trackId]) return;
  try {
    // Check IndexedDB first
    const blob = await db.getArtwork(trackId);
    if (blob) {
      artworkUrls.value[trackId] = URL.createObjectURL(blob);
      return;
    }
    // For server-sourced tracks, use the backend artwork endpoint
    const track = libraryStore.results.find((t) => t.id === trackId);
    if (track?.sourceTag === 'server' && track.serverPath) {
      artworkUrls.value[trackId] = backend.artworkUrl(track.serverPath);
    }
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

// ── AcoustID identification ──
const isIdentifying = ref(false);

const untaggedCount = computed(() =>
  libraryStore.results.filter((t) => !t.artist && !t.title).length
    + libraryStore.results.filter((t) => t.title && !t.artist).length,
);

async function identifyUntagged(): Promise<void> {
  const untagged = libraryStore.results.filter((t) => !t.artist);
  if (untagged.length === 0) return;

  isIdentifying.value = true;

  for (const track of untagged) {
    try {
      // Get the audio file blob
      let blob: Blob | null = null;

      if (track.sourceTag === 'local' && track.localPath) {
        const sources = await db.getSources();
        for (const source of sources) {
          if (source.type !== 'local') continue;
          const handle = (source.config as { handle: FileSystemDirectoryHandle }).handle;
          try {
            const parts = track.localPath.split('/').filter(Boolean);
            let dir: FileSystemDirectoryHandle = handle;
            for (let i = 0; i < parts.length - 1; i++) {
              dir = await dir.getDirectoryHandle(parts[i]!);
            }
            const fileHandle = await dir.getFileHandle(parts[parts.length - 1]!);
            blob = await fileHandle.getFile();
            break;
          } catch { /* try next source */ }
        }
      }

      if (!blob) continue;

      const filename = track.localPath?.split('/').pop() || 'track.mp3';
      const result = await backend.identifyTrack(blob, filename);

      if (result.error) continue;

      // Update track in DB with identified metadata
      const updated: Track = {
        ...track,
        title: result.title || track.title,
        artist: result.artist || track.artist,
        album: result.album || track.album,
      };

      // Download and store cover art if available
      if (result.coverArtUrl && !track.artworkId) {
        try {
          const artResp = await fetch(result.coverArtUrl);
          if (artResp.ok) {
            const artBlob = await artResp.blob();
            await db.putArtwork(track.id, artBlob);
            updated.artworkId = track.id;
          }
        } catch { /* cover art fetch failed, non-fatal */ }
      }

      await db.putTrack(updated);
    } catch {
      // skip failed tracks
    }
  }

  isIdentifying.value = false;
  // Refresh library results
  libraryStore.search(searchQuery.value);
}

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

.user-avatar-btn {
  padding: 0 !important;
  min-width: 34px !important;
  min-height: 34px !important;
}

.user-avatar {
  width: 34px;
  height: 34px;
  border-radius: 50%;
  overflow: hidden;
  cursor: pointer;
  flex-shrink: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  background: #2a2d3e;

  img {
    width: 100%;
    height: 100%;
    object-fit: cover;
  }
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

/* ── Group grid (Artists / Albums) ── */
.group-grid-scroll {
  height: 100%;
  overflow-y: auto;
  padding: 0 16px 80px;
}

.group-grid {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 12px;
}

.group-card {
  background: #1a1d28;
  border-radius: 14px;
  padding: 16px 12px;
  display: flex;
  flex-direction: column;
  align-items: center;
  cursor: pointer;
  transition: background 0.15s ease;

  &:hover {
    background: #252838;
  }
}

.group-card-art {
  width: 72px;
  height: 72px;
  border-radius: 50%;
  background: #252838;
  overflow: hidden;
  display: flex;
  align-items: center;
  justify-content: center;
  margin-bottom: 10px;

  img {
    width: 100%;
    height: 100%;
    object-fit: cover;
  }
}

.group-card-name {
  font-size: 14px;
  font-weight: 600;
  color: #fff;
  text-align: center;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  max-width: 100%;
}

.group-card-sub {
  font-size: 11px;
  color: rgba(255, 255, 255, 0.4);
  text-align: center;
  margin-top: 2px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  max-width: 100%;
}

/* ── Group detail view ── */
.group-detail {
  display: flex;
  flex-direction: column;
  height: 100%;
}

.group-detail-header {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 12px 16px;
  flex-shrink: 0;
}

.group-detail-info {
  flex: 1;
  min-width: 0;
}

.group-detail-title {
  font-size: 18px;
  font-weight: 700;
  color: #fff;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.group-detail-sub {
  font-size: 12px;
  color: rgba(255, 255, 255, 0.4);
}

.group-tracks-scroll {
  flex: 1;
  overflow-y: auto;
  padding-bottom: 80px;
}

/* ── Mini player styles removed — handled by BottomNav ── */

/* ── Track detail dialog ── */
.detail-grid {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.detail-row {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.detail-label {
  font-size: 11px;
  color: rgba(255, 255, 255, 0.4);
  text-transform: uppercase;
  letter-spacing: 0.05em;
}

.detail-value {
  font-size: 14px;
  color: #fff;
}
</style>
