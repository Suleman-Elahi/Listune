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
          placeholder="Search your library... (Enter for Soulseek)"
          @input="onSearch"
          @keyup.enter="runOnlineSearch"
        />
        <q-btn
          flat
          round
          dense
          icon="tune"
          color="grey-5"
          size="xs"
          title="Download options"
        >
          <q-popup-proxy transition-show="scale" transition-hide="scale">
            <div class="ss-popup">
              <div class="ss-popup-title">Soulseek download options</div>
              <div class="ss-popup-label">Format</div>
              <q-btn-toggle
                v-model="optsDraft.mode"
                dense
                no-caps
                push
                toggle-color="primary"
                color="grey-9"
                text-color="white"
                :options="[
                  { label: 'MP3', value: 'mp3' },
                  { label: 'FLAC', value: 'flac' },
                ]"
              />
              <div class="ss-popup-label">Quality (MP3)</div>
              <q-select
                v-model="optsDraft.quality"
                dark
                outlined
                dense
                emit-value
                map-options
                :disable="optsDraft.mode === 'flac'"
                :options="qualityOptions"
                style="min-width: 180px;"
              />
              <template v-if="!ssStore.status?.loggedIn">
                <q-separator dark class="q-my-sm" />
                <div class="ss-popup-label">Soulseek login</div>
                <q-input v-model="ssAccount" dark outlined dense label="Username" class="q-mb-xs" />
                <q-input
                  v-model="ssPassword"
                  dark
                  outlined
                  dense
                  type="password"
                  label="Password"
                  @keyup.enter="doSsLogin"
                />                <q-btn
                  flat
                  dense
                  no-caps
                  color="primary"
                  label="Save account"
                  :loading="ssLoggingIn"
                  :disable="!ssAccount.trim() || !ssPassword"
                  class="q-my-xs"
                  @click="doSsLogin"
                />
                <div v-if="ssStore.loginError" class="text-negative text-xs">{{ ssStore.loginError }}</div>
              </template>
              <div v-else class="text-positive text-xs q-mt-sm">
                <q-icon name="circle" size="8px" class="q-mr-xs" />Connected as {{ ssStore.status?.account }}
              </div>
              <div class="row justify-between q-mt-sm items-center">
                <div v-if="ssStore.statusError" class="text-negative text-xs col ss-popup-error">{{ ssStore.statusError }}</div>
                <q-btn flat dense no-caps color="grey-4" label="Close" v-close-popup />
              </div>
            </div>
          </q-popup-proxy>
        </q-btn>
      </div>
      <div v-if="ssStore.searchError" class="ss-inline-error">{{ ssStore.searchError }}</div>
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
      <q-linear-progress
        :value="scanRatio"
        :indeterminate="scanUnknownTotal"
        color="light-green-6"
        track-color="grey-9"
        class="q-mb-xs"
      />
      <div class="scan-label">
        <template v-if="scanUnknownTotal">Scanning… listing files</template>
        <template v-else>
          Scanning {{ libraryStore.scanProgress?.scanned ?? 0 }} / {{ libraryStore.scanProgress?.total ?? 0 }}
          <span v-if="(libraryStore.scanProgress?.errors ?? 0) > 0" class="text-warning"> · {{ libraryStore.scanProgress?.errors }} errors</span>
        </template>
      </div>
    </div>
    <div v-else-if="libraryStore.scanError" class="scan-progress">
      <div class="scan-error">
        <q-icon name="error_outline" size="14px" class="q-mr-xs" />
        {{ libraryStore.scanError }}
        <q-btn flat dense size="sm" label="Dismiss" color="primary" class="q-ml-sm" @click="libraryStore.scanError = ''" />
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
        v-if="listRows.length > 0"
        :items="listRows"
        :virtual-scroll-item-size="72"
        style="height: 100%"
        @virtual-scroll="onVirtualScroll"
      >
        <template #default="{ item: row }">
          <!-- Real track -->
          <div
            v-if="row.kind === 'track'"
            :key="row.track.id"
            class="track-row"
            :class="{ 'track-row--active': row.track.id === playerStore.currentTrackId }"
            @click="playTrack(row.track.id)"
          >
            <span class="track-num">{{ String(row.index + 1).padStart(2, '0') }}</span>
            <div class="track-art">
              <img v-if="artworkUrls[row.track.id]" :src="artworkUrls[row.track.id]" />
              <q-icon v-else name="music_note" color="grey-6" size="20px" />
            </div>
            <div class="track-info">
              <div class="track-title">{{ row.track.title || 'Unknown' }}</div>
              <div class="track-artist">{{ row.track.artist || 'Unknown artist' }}</div>
            </div>
            <span class="track-duration">{{ formatDuration(row.track.duration) }}</span>
            <q-btn flat round dense icon="add" color="grey-5" size="xs" @click.stop="showTrackMenu($event, row.track.id)">
              <q-menu anchor="bottom right" self="top right" class="track-menu">
                <q-list dense dark style="min-width: 180px; background: #1e2230;">
                  <q-item clickable v-close-popup @click="enqueueTrack(row.track.id)">
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
                    @click="addToPlaylist(pl.id, row.track.id)"
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
                  <q-item clickable v-close-popup @click="playNext(row.track.id)">
                    <q-item-section avatar><q-icon name="playlist_play" size="18px" color="white" /></q-item-section>
                    <q-item-section style="color: #fff;">Play next</q-item-section>
                  </q-item>
                  <q-item clickable v-close-popup @click="enqueueTrack(row.track.id)">
                    <q-item-section avatar><q-icon name="queue" size="18px" color="white" /></q-item-section>
                    <q-item-section style="color: #fff;">Add to queue</q-item-section>
                  </q-item>
                  <q-separator dark />
                  <q-item clickable v-close-popup @click="identifySingle(row.track)">
                    <q-item-section avatar><q-icon name="fingerprint" size="18px" color="orange-4" /></q-item-section>
                    <q-item-section style="color: #fff;">Identify track</q-item-section>
                  </q-item>
                  <q-item clickable v-close-popup @click="showTrackDetails(row.track)">
                    <q-item-section avatar><q-icon name="info_outline" size="18px" color="light-blue-4" /></q-item-section>
                    <q-item-section style="color: #fff;">Track info</q-item-section>
                  </q-item>
                  <q-separator dark />
                  <q-item clickable v-close-popup @click="removeTrack(row.track.id)">
                    <q-item-section avatar><q-icon name="delete_outline" size="18px" color="red-4" /></q-item-section>
                    <q-item-section style="color: #ef4444;">Remove from library</q-item-section>
                  </q-item>
                </q-list>
              </q-menu>
            </q-btn>
          </div>

          <!-- Section label -->
          <div
            v-else-if="row.kind === 'label'"
            :key="'label-' + row.label"
            class="ss-section-label"
          >
            {{ row.label }}<template v-if="ssStore.searchQuery && row.label.startsWith('Soulseek')"> · "{{ ssStore.searchQuery }}"</template>
          </div>

          <!-- Searching row -->
          <div
            v-else-if="row.kind === 'searching'"
            key="ss-searching"
            class="track-row track-row--ghost"
          >
            <q-spinner-dots color="primary" size="22px" class="q-mr-sm" />
            <div class="track-info">
              <div class="track-title">Searching Soulseek…</div>
              <div class="track-artist">waiting for peer responses</div>
            </div>
          </div>

          <!-- Soulseek result placeholder (faded, not clickable) -->
          <div
            v-else
            :key="row.ghost.key"
            class="track-row track-row--ghost"
          >
            <span class="track-num">--</span>
            <div class="track-art track-art--ghost">
              <q-icon name="cloud_download" color="grey-6" size="18px" />
            </div>
            <div class="track-info">
              <div class="track-title">{{ row.ghost.title }}</div>
              <div class="track-artist">
                {{ row.ghost.artist }}
                <template v-if="row.ghost.bitrate"> · {{ row.ghost.bitrate }}kbps</template>
                <template v-if="row.ghost.sizeMb"> · {{ row.ghost.sizeMb }}</template>
              </div>
              <q-linear-progress
                v-if="row.ghost.status === 'downloading'"
                indeterminate
                color="light-green-6"
                track-color="grey-9"
                class="ss-ghost-progress"
              />
            </div>
            <span class="ss-ghost-status" :class="`ss-ghost-status--${row.ghost.status}`">
              <template v-if="row.ghost.status === 'downloading'">downloading</template>
              <template v-else-if="row.ghost.status === 'failed'">
                failed
                <q-tooltip>{{ row.ghost.error || 'download failed' }}</q-tooltip>
              </template>
              <template v-else>found</template>
            </span>
            <q-btn
              v-if="row.ghost.status !== 'downloading'"
              flat
              round
              dense
              size="xs"
              :icon="row.ghost.status === 'failed' ? 'refresh' : 'download'"
              :color="row.ghost.status === 'failed' ? 'negative' : 'grey-5'"
              :title="row.ghost.status === 'failed' ? 'Retry download' : 'Download'"
              @click.stop="downloadGhost(row.ghost)"
            />
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
import { useQuasar } from 'quasar';
import { useLibraryStore } from 'src/stores/libraryStore';
import { usePlayerStore } from 'src/stores/playerStore';
import { useSoulseekStore, type SoulseekGhost, type SoulseekOpts } from 'src/stores/soulseekStore';
import { db } from 'src/services/db';
import AddSourceDialog from 'src/components/AddSourceDialog.vue';
import BottomNav from 'src/components/BottomNav.vue';
import { backend, type IdentifyResult } from 'src/services/backend';
import { getPresignedTrackUrl } from 'src/services/s3Sources';
import type { Track } from 'src/types/models';

const router = useRouter();
const $q = useQuasar();
const libraryStore = useLibraryStore();
const playerStore = usePlayerStore();
const ssStore = useSoulseekStore();

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

/** Total is only known once the source has been fully listed. */
const scanUnknownTotal = computed(() => (libraryStore.scanProgress?.total ?? 0) === 0);

const filteredTracks = computed<Track[]>(() => libraryStore.results);

// ── Soulseek integration (search directly from this view) ──

interface ListRow {
  kind: 'track' | 'label' | 'searching' | 'ghost';
  track?: Track;
  index?: number;
  label?: string;
  ghost?: SoulseekGhost;
}

const listRows = computed<ListRow[]>(() => {
  const rows: ListRow[] = filteredTracks.value.map((track, index) => ({
    kind: 'track',
    track,
    index,
  }));
  // Library matches first, Soulseek results after — only while a query is typed.
  if (!searchQuery.value.trim() || (!ssStore.hasResults && !ssStore.searching)) return rows;

  rows.push({ kind: 'label', label: 'Soulseek results' });
  if (ssStore.searching) {
    rows.push({ kind: 'searching' });
  }
  for (const ghost of ssStore.ghosts) {
    rows.push({ kind: 'ghost', ghost });
  }
  return rows;
});

const qualityOptions = [
  { label: 'Any', value: '' },
  { label: '128 kbps', value: '128' },
  { label: '192 kbps', value: '192' },
  { label: '256 kbps', value: '256' },
  { label: '320 kbps', value: '320' },
];

/** Draft of the persisted Soulseek options, edited in the popup. */
const optsDraft = ref<SoulseekOpts>({ ...ssStore.opts });
watch(() => ssStore.opts, (next) => { optsDraft.value = { ...next }; }, { immediate: true });
watch(optsDraft, (next) => { ssStore.saveOpts(next); }, { deep: true });

const ssAccount = ref('');
const ssPassword = ref('');
const ssLoggingIn = ref(false);

function runOnlineSearch(): void {
  const q = searchQuery.value.trim();
  if (!q) {
    notifyError('Type something to search Soulseek');
    return;
  }
  if (!ssStore.status?.loggedIn) void ssStore.loadStatus();
  void ssStore.runSearch(q).then(() => {
    if (ssStore.searchError) {
      notifyError(ssStore.searchError);
    } else if (ssStore.hasResults) {
      notifySuccess(`Found ${ssStore.ghosts.length} file(s) on Soulseek`);
    }
  });
}

function downloadGhost(ghost: SoulseekGhost): void {
  void ssStore.downloadFile({ resultIndex: ghost.resultIndex, fileIndex: ghost.fileIndex }).then(() => {
    if (ssStore.downloadError) notifyError(ssStore.downloadError);
  });
}

async function doSsLogin(): Promise<void> {
  ssLoggingIn.value = true;
  try {
    const ok = await ssStore.login(ssAccount.value.trim(), ssPassword.value);
    if (ok) {
      ssPassword.value = '';
      notifySuccess(`Soulseek: logged in as ${ssAccount.value.trim()}`);
    } else {
      notifyError(ssStore.loginError);
    }
  } finally {
    ssLoggingIn.value = false;
  }
}

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
  await db.deleteTrack(id);
  // Stops playback if this was the current track and clears any preload.
  playerStore.removeTrack(id);
  libraryStore.search(searchQuery.value).catch(() => undefined);
}

const trackDetailDialog = ref(false);
const detailTrack = ref<Track | null>(null);

function showTrackDetails(track: Track) {
  detailTrack.value = track;
  trackDetailDialog.value = true;
}

function notifyError(message: string): void {
  $q.notify({ type: 'negative', message, position: 'top', timeout: 5000 });
}

function notifySuccess(message: string): void {
  $q.notify({ type: 'positive', message, position: 'top', timeout: 3000 });
}

/** Strip common download/upload noise from a filename or tag for search hints. */
function cleanIdentifyText(value: string): string {
  return value
    .replace(
      /[([][^\])]*(official|lyrics?|audio|video|visuali[sz]er|hd|hq|explicit|clean)[^\])]*[\])]/gi,
      ' ',
    )
    .replace(/_/g, ' ')
    .replace(/\s+/g, ' ')
    .trim();
}

const TITLE_SEPARATORS = [' - ', ' \u2013 ', ' \u2014 '];

/** Remove a leading/trailing "Artist - " / " - Artist" from a title. */
function removeArtistFromTitle(title: string, artist: string): string {
  if (!artist) return title;
  const lowerTitle = title.toLowerCase();
  const lowerArtist = artist.toLowerCase();

  for (const separator of TITLE_SEPARATORS) {
    if (lowerTitle.startsWith(lowerArtist + separator)) {
      return title.slice(artist.length + separator.length).trim();
    }
    if (lowerTitle.endsWith(separator + lowerArtist)) {
      return title.slice(0, title.length - separator.length - artist.length).trim();
    }
  }
  return title;
}

function getIdentifyHints(track: Track): { title?: string; artist?: string } {
  let title = cleanIdentifyText(track.title || '');
  let artist = cleanIdentifyText(track.artist || '');

  // Tags often contain "Artist - Title (Official Video)" in the title field.
  title = removeArtistFromTitle(title, artist);

  // Filename-style "Artist - Title" when no artist tag exists.
  if (!artist) {
    for (const separator of TITLE_SEPARATORS) {
      const index = title.indexOf(separator);
      if (index > 0) {
        artist = title.slice(0, index).trim();
        title = title.slice(index + separator.length).trim();
        title = removeArtistFromTitle(title, artist);
        break;
      }
    }
  }

  const hints: { title?: string; artist?: string } = {};
  if (title) hints.title = title;
  if (artist) hints.artist = artist;
  return hints;
}

/** Resolve whichever source a track belongs to and send it for identification. */
async function identifyTrackSource(track: Track): Promise<IdentifyResult | null> {
  const hints = getIdentifyHints(track);

  if (track.sourceTag === 'server' && track.serverPath) {
    return backend.identifyServerFile(track.serverPath, hints);
  }

  if (track.sourceTag === 'local' && track.localPath) {
    const sources = await db.getSources();
    for (const source of sources) {
      if (source.type !== 'local' || !('handle' in source.config)) continue;
      const handle = source.config.handle;
      try {
        const parts = track.localPath.split('/').filter(Boolean);
        let dir: FileSystemDirectoryHandle = handle;
        for (let i = 0; i < parts.length - 1; i++) {
          dir = await dir.getDirectoryHandle(parts[i]!);
        }
        const fileHandle = await dir.getFileHandle(parts[parts.length - 1]!);
        const blob = await fileHandle.getFile();
        const filename = parts[parts.length - 1] || 'track.mp3';
        return backend.identifyTrack(blob, filename, hints);
      } catch {
        // try the next local source
      }
    }
    return null;
  }

  if (track.sourceTag === 's3' && track.s3Key) {
    const url = await getPresignedTrackUrl(track);
    if (!url) return null;
    const filename = track.s3Key.split('/').pop() || 'track.mp3';
    return backend.identifyUrl(url, filename, hints);
  }

  return null;
}

async function applyIdentifyResult(track: Track, result: IdentifyResult): Promise<Track> {
  const updated: Track = {
    ...track,
    title: result.title || track.title,
    artist: result.artist || track.artist,
    album: result.album || track.album,
  };

  if (result.coverArtUrl && !track.artworkId) {
    try {
      const artResp = await fetch(result.coverArtUrl);
      if (artResp.ok) {
        await db.putArtwork(track.id, await artResp.blob());
        updated.artworkId = track.id;
      }
    } catch {
      // cover art is optional
    }
  }

  await db.putTrack(updated);
  return updated;
}

async function identifySingle(track: Track): Promise<void> {
  const dismiss = $q.notify({
    type: 'ongoing',
    message: `Identifying ${track.title}...`,
    timeout: 0,
    position: 'top',
  });

  try {
    const result = await identifyTrackSource(track);
    if (!result) {
      notifyError('No readable audio source for this track');
      return;
    }
    if (result.error) {
      notifyError(result.error);
      return;
    }

    const updated = await applyIdentifyResult(track, result);

    // Write tags to the actual file when it lives on the server.
    if (track.sourceTag === 'server' && track.serverPath) {
      try {
        await backend.writeTags({
          filePath: track.serverPath,
          title: result.title,
          artist: result.artist,
          album: result.album,
          year: result.year,
          genre: result.genre,
          coverArtUrl: result.coverArtUrl,
        });
      } catch {
        // best effort
      }
    }

    notifySuccess(`Identified: ${updated.title}`);
    libraryStore.search(searchQuery.value).catch(() => undefined);
  } catch (err) {
    notifyError(`Identification failed: ${err instanceof Error ? err.message : String(err)}`);
  } finally {
    dismiss();
  }
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
  // Keep the ghost list in sync with what is already downloadable.
  void ssStore.refreshLibraryTitles();
}, { immediate: true });

watch(() => libraryStore.isScanning, (scanning) => {
  if (scanning) return;
  libraryStore.search(searchQuery.value).catch(() => undefined);
  // Also refresh the grouped views so newly scanned tracks appear there.
  if (activeTab.value === 'Artists') loadArtists().catch(() => undefined);
  else if (activeTab.value === 'Albums') loadAlbums().catch(() => undefined);
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
  let identified = 0;
  let failed = 0;

  for (const track of untagged) {
    try {
      const result = await identifyTrackSource(track);
      if (!result || result.error) {
        failed++;
        continue;
      }
      await applyIdentifyResult(track, result);
      identified++;
    } catch {
      failed++;
    }
  }

  isIdentifying.value = false;
  await libraryStore.search(searchQuery.value).catch(() => undefined);

  if (identified > 0) {
    notifySuccess(`Identified ${identified} track${identified === 1 ? '' : 's'}`);
  }
  if (failed > 0) {
    notifyError(`${failed} track${failed === 1 ? '' : 's'} could not be identified`);
  }
}

onMounted(() => {
  void libraryStore.search('');
  // Pull in everything from the server library folder (LIBRARY_PATH).
  void libraryStore.syncServerLibrary();
  // Soulseek status + any running download jobs.
  void ssStore.loadStatus();
  void ssStore.refreshJobs();
  void ssStore.refreshLibraryTitles();
});
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

.scan-error {
  display: flex;
  align-items: center;
  font-size: 12px;
  color: #f87171;
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

/* ── Soulseek search options popup ── */
.ss-popup {
  background: #1e2230;
  color: #fff;
  padding: 14px;
  border-radius: 12px;
  min-width: 260px;
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.ss-popup-title {
  font-size: 14px;
  font-weight: 700;
  margin-bottom: 2px;
}

.ss-popup-label {
  font-size: 11px;
  color: rgba(255, 255, 255, 0.5);
  text-transform: uppercase;
  letter-spacing: 0.05em;
}

.ss-popup-error {
  max-width: 150px;
  overflow-wrap: anywhere;
}

.ss-inline-error {
  font-size: 12px;
  color: #f87171;
  padding: 6px 14px 0;
}

/* ── Soulseek ghost rows (faded placeholders) ── */
.ss-section-label {
  padding: 12px 16px 4px;
  font-size: 11px;
  text-transform: uppercase;
  letter-spacing: 0.08em;
  color: #4ade80;
  font-weight: 700;
}

.track-row--ghost {
  opacity: 0.55;
  cursor: default;

  &:hover {
    background: transparent;
  }
}

.track-art--ghost {
  background: rgba(255, 255, 255, 0.03);
  border: 1px dashed rgba(255, 255, 255, 0.12);
  box-sizing: border-box;
}

.ss-ghost-progress {
  margin-top: 4px;
  max-width: 320px;
}

.ss-ghost-status {
  font-size: 11px;
  color: rgba(255, 255, 255, 0.4);
  flex-shrink: 0;
  min-width: 68px;
  text-align: right;
}

.ss-ghost-status--downloading {
  color: #a3e635;
}

.ss-ghost-status--failed {
  color: #f87171;
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
