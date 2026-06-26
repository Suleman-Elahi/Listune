<template>
  <!-- Req 7.1: Slide-down overlay animated via Vue <Transition> with CSS translateY -->
  <Transition name="drawer">
    <div
      v-if="modelValue"
      class="drawer-panel"
      @touchstart.passive="onTouchStart"
      @touchmove.passive="onTouchMove"
      @touchend.passive="onTouchEnd"
    >
      <!-- Req 7.8: Chevron handle at top — tap closes drawer -->
      <div class="drawer-handle" @click="close">
        <q-icon name="expand_less" size="32px" color="white" />
      </div>

      <!-- Req 7.3: Two tabs: Queue and Playlists -->
      <q-tabs
        v-model="activeTab"
        dense
        align="justify"
        class="drawer-tabs"
        active-color="white"
        indicator-color="white"
      >
        <q-tab name="queue" label="Queue" />
        <q-tab name="playlists" label="Playlists" />
      </q-tabs>

      <q-tab-panels v-model="activeTab" animated class="drawer-panels">
        <!-- Queue tab -->
        <q-tab-panel name="queue" class="queue-panel">
          <!-- Req 7.6: QVirtualScroll over playerStore.queue -->
          <q-virtual-scroll
            v-if="playerStore.queue.length > 0"
            :items="playerStore.queue"
            :virtual-scroll-item-size="56"
            style="height: 100%"
          >
            <template #default="{ item: trackId, index }">
              <!-- Req 7.7: Tapping a queue item calls playerStore.play(trackId) -->
              <q-item
                :key="trackId"
                clickable
                :active="trackId === playerStore.currentTrackId"
                active-class="active-track"
                @click="playerStore.play(trackId)"
              >
                <q-item-section avatar>
                  <q-icon
                    :name="trackId === playerStore.currentTrackId && playerStore.isPlaying ? 'volume_up' : 'music_note'"
                    color="white"
                    size="20px"
                  />
                </q-item-section>
                <q-item-section>
                  <!-- Req 7.4: Display each item as "<title> - <artist>" -->
                  <q-item-label class="track-label">
                    {{ trackLabels[trackId] ?? 'Loading...' }}
                  </q-item-label>
                  <q-item-label caption class="track-index">
                    {{ index + 1 }}
                  </q-item-label>
                </q-item-section>
              </q-item>
            </template>
          </q-virtual-scroll>

          <div v-else class="empty-state">
            <q-icon name="queue_music" size="48px" color="grey-6" />
            <p class="text-grey-6">Queue is empty</p>
          </div>
        </q-tab-panel>

        <!-- Playlists tab (Req 7.5) -->
        <q-tab-panel name="playlists" class="playlists-panel">
          <!-- Save current queue as playlist -->
          <div v-if="playerStore.queue.length > 0" class="save-queue-row">
            <q-btn
              flat
              dense
              icon="playlist_add"
              label="Save queue as playlist"
              color="light-blue-4"
              size="sm"
              @click="showSaveDialog = true"
            />
          </div>

          <q-virtual-scroll
            v-if="playlists.length > 0"
            :items="playlists"
            :virtual-scroll-item-size="64"
            style="height: 100%"
          >
            <template #default="{ item: pl }">
              <q-item
                :key="pl.id"
                clickable
                @click="loadPlaylist(pl)"
              >
                <q-item-section avatar>
                  <q-icon name="queue_music" color="white" size="20px" />
                </q-item-section>
                <q-item-section>
                  <q-item-label class="track-label">{{ pl.name }}</q-item-label>
                  <q-item-label caption class="track-index">
                    {{ pl.trackIds.length }} tracks
                  </q-item-label>
                </q-item-section>
                <q-item-section side>
                  <q-btn flat round dense icon="delete_outline" color="grey-5" size="xs" @click.stop="deletePlaylist(pl.id)" />
                </q-item-section>
              </q-item>
            </template>
          </q-virtual-scroll>

          <div v-else class="empty-state">
            <q-icon name="playlist_play" size="48px" color="grey-6" />
            <p class="text-grey-6">No saved playlists</p>
          </div>

          <!-- Save dialog -->
          <q-dialog v-model="showSaveDialog" no-focus no-refocus>
            <q-card dark style="min-width: 260px; background: #1e2230; border-radius: 16px;">
              <q-card-section>
                <div style="font-size: 15px; font-weight: 700; color: #fff; margin-bottom: 10px;">Save Queue as Playlist</div>
                <input
                  v-model="newPlaylistName"
                  class="save-input"
                  placeholder="Playlist name..."
                  @keyup.enter="saveQueueAsPlaylist"
                />
              </q-card-section>
              <q-card-actions align="right">
                <q-btn flat label="Cancel" color="grey-5" @click="showSaveDialog = false" />
                <q-btn unelevated rounded label="Save" color="primary" @click="saveQueueAsPlaylist" />
              </q-card-actions>
            </q-card>
          </q-dialog>
        </q-tab-panel>
      </q-tab-panels>
    </div>
  </Transition>
</template>

<script setup lang="ts">
// Feature: xp-music-player — DrawerPanel
// Requirements: 7.1, 7.2, 7.3, 7.4, 7.5, 7.6, 7.7, 7.8, 7.9

import { ref, watch, reactive } from 'vue';
import { usePlayerStore } from 'src/stores/playerStore';
import { db } from 'src/services/db';

// Req 7.1: v-model pattern for open/closed state
const props = defineProps<{
  modelValue: boolean;
}>();

const emit = defineEmits<{
  (e: 'update:modelValue', value: boolean): void;
}>();

const playerStore = usePlayerStore();
const activeTab = ref<'queue' | 'playlists'>('queue');

// Track label cache: trackId → "title - artist"
const trackLabels = reactive<Record<string, string>>({});

// ── Playlist support ──
interface Playlist {
  id: string;
  name: string;
  trackIds: string[];
  createdAt: number;
}

function loadPlaylists(): Playlist[] {
  try { return JSON.parse(localStorage.getItem('playlists') || '[]'); } catch { return []; }
}
function savePlaylists(pls: Playlist[]): void {
  localStorage.setItem('playlists', JSON.stringify(pls));
}

const playlists = ref<Playlist[]>(loadPlaylists());
const showSaveDialog = ref(false);
const newPlaylistName = ref('');

function saveQueueAsPlaylist(): void {
  const name = newPlaylistName.value.trim();
  if (!name || playerStore.queue.length === 0) return;
  const pl: Playlist = {
    id: crypto.randomUUID(),
    name,
    trackIds: [...playerStore.queue],
    createdAt: Date.now(),
  };
  playlists.value.unshift(pl);
  savePlaylists(playlists.value);
  newPlaylistName.value = '';
  showSaveDialog.value = false;
}

function loadPlaylist(pl: Playlist): void {
  playerStore.setQueue(pl.trackIds);
  if (pl.trackIds.length > 0) {
    playerStore.play(pl.trackIds[0]!);
  }
  close();
}

function deletePlaylist(id: string): void {
  playlists.value = playlists.value.filter((p) => p.id !== id);
  savePlaylists(playlists.value);
}

function close(): void {
  emit('update:modelValue', false);
}

// Req 7.4: Fetch Track from DB and build "<title> - <artist>" label
async function loadTrackLabel(trackId: string): Promise<void> {
  if (trackLabels[trackId]) return;
  const track = await db.getTrack(trackId);
  if (track) {
    trackLabels[trackId] = `${track.title} - ${track.artist}`;
  } else {
    trackLabels[trackId] = trackId;
  }
}

// Load labels whenever queue changes
watch(
  () => playerStore.queue,
  (queue) => {
    for (const id of queue) {
      void loadTrackLabel(id);
    }
  },
  { immediate: true, deep: true },
);

// Also load labels + refresh playlists when drawer opens
watch(
  () => props.modelValue,
  (open) => {
    if (open) {
      for (const id of playerStore.queue) {
        void loadTrackLabel(id);
      }
      // Refresh playlists from localStorage (may have changed from PlaylistsView)
      playlists.value = loadPlaylists();
    }
  },
);

// Req 7.8: Swipe-down gesture to close drawer
const touchStartY = ref(0);
const touchCurrentY = ref(0);
const SWIPE_THRESHOLD = 50;

function onTouchStart(e: TouchEvent): void {
  touchStartY.value = e.touches[0]?.clientY ?? 0;
  touchCurrentY.value = touchStartY.value;
}

function onTouchMove(e: TouchEvent): void {
  touchCurrentY.value = e.touches[0]?.clientY ?? touchCurrentY.value;
}

function onTouchEnd(): void {
  const delta = touchCurrentY.value - touchStartY.value;
  if (delta > SWIPE_THRESHOLD) {
    close();
  }
  touchStartY.value = 0;
  touchCurrentY.value = 0;
}
</script>

<style scoped>
/* Req 7.1: Slide-down overlay using CSS transform: translateY */
.drawer-panel {
  position: fixed;
  top: 0;
  left: 0;
  right: 0;
  /* Req 7.9: Transport controls remain visible below drawer — leave bottom space */
  bottom: 120px;
  z-index: 100;
  display: flex;
  flex-direction: column;
  /* Req 7.2: Frosted/semi-transparent background */
  background: rgba(10, 10, 20, 0.88);
  backdrop-filter: blur(20px) saturate(1.4);
  -webkit-backdrop-filter: blur(20px) saturate(1.4);
  border-bottom-left-radius: 16px;
  border-bottom-right-radius: 16px;
  overflow: hidden;
}

/* Req 7.1: Vue <Transition> enter/leave using translateY */
.drawer-enter-active,
.drawer-leave-active {
  transition: transform 0.35s cubic-bezier(0.4, 0, 0.2, 1);
}

.drawer-enter-from,
.drawer-leave-to {
  transform: translateY(-100%);
}

.drawer-enter-to,
.drawer-leave-from {
  transform: translateY(0);
}

.drawer-handle {
  display: flex;
  justify-content: center;
  align-items: center;
  padding: 8px 0 4px;
  cursor: pointer;
  user-select: none;
  flex-shrink: 0;
}

.drawer-handle:active {
  opacity: 0.7;
}

.drawer-tabs {
  flex-shrink: 0;
  background: transparent;
  color: rgba(255, 255, 255, 0.7);
}

.drawer-panels {
  flex: 1;
  overflow: hidden;
  background: transparent;
}

.queue-panel,
.playlists-panel {
  padding: 0;
  height: 100%;
  background: transparent;
}

.track-label {
  color: white;
  font-size: 14px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.track-index {
  color: rgba(255, 255, 255, 0.4);
  font-size: 11px;
}

.active-track {
  background: rgba(255, 255, 255, 0.12);
}

.empty-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  height: 100%;
  gap: 12px;
}

.empty-state p {
  margin: 0;
  font-size: 14px;
}

.save-queue-row {
  display: flex;
  justify-content: center;
  padding: 10px 16px 6px;
}

.save-input {
  width: 100%;
  background: #2a2d3e;
  border: 1px solid rgba(255, 255, 255, 0.1);
  border-radius: 10px;
  padding: 10px 14px;
  color: #fff;
  font-size: 15px;
  outline: none;
}

.save-input::placeholder {
  color: rgba(255, 255, 255, 0.3);
}
</style>
