<template>
  <!-- Req 6.1: Full-screen layout on black background -->
  <div class="now-playing-view">
    <!-- Chevron handle at top to open DrawerPanel -->
    <div class="drawer-handle" @click="drawerOpen = true">
      <q-icon name="expand_more" size="32px" color="white" />
    </div>

    <!-- Library button -->
    <div class="library-btn">
      <q-btn flat round icon="library_music" color="white" size="sm" @click="$router.push('/library')" />
    </div>

    <!-- Req 6.1: Visualizer in upper ~40% -->
    <div class="visualizer-area">
      <div class="visualizer-card">
        <VisualizerCanvas />
      </div>
    </div>

    <!-- Req 6.2: Track title and artist below visualizer -->
    <div class="track-info">
      <!-- Req 6.9: Placeholder text when no track loaded -->
      <div class="track-title">{{ trackTitle }}</div>
      <div class="track-artist">{{ trackArtist }}</div>
    </div>

    <!-- Req 6.3: ProgressBar with draggable thumb -->
    <div class="progress-area">
      <ProgressBar
        :current-time="playerStore.currentTime"
        :duration="playerStore.duration"
        @seek="playerStore.seek"
      />
    </div>

    <!-- Req 6.4: Transport buttons -->
    <div class="transport-controls">
      <q-btn
        flat
        round
        icon="skip_previous"
        color="white"
        size="sm"
        class="transport-btn"
        @click="playerStore.previous()"
      />
      <!-- Req 6.5: Play/Pause toggles playback -->
      <q-btn
        flat
        round
        :icon="playerStore.isPlaying ? 'pause' : 'play_arrow'"
        color="white"
        size="md"
        class="transport-btn play-btn"
        @click="togglePlayPause"
      />
      <!-- Req 6.7: Next advances to next track -->
      <q-btn
        flat
        round
        icon="skip_next"
        color="white"
        size="sm"
        class="transport-btn"
        @click="playerStore.next()"
      />
      <!-- Loop toggle -->
      <q-btn
        flat
        round
        icon="repeat_one"
        :color="playerStore.isLooping ? 'light-green-4' : 'white'"
        size="sm"
        class="transport-btn"
        :class="{ 'loop-active': playerStore.isLooping }"
        @click="playerStore.toggleLoop()"
      />
    </div>

    <!-- Req 6.8: Volume slider with speaker icons -->
    <div class="volume-area">
      <q-icon name="volume_down" color="white" size="20px" />
      <input
        type="range"
        class="volume-slider"
        min="0"
        max="1"
        step="0.01"
        :value="playerStore.volume"
        @input="onVolumeInput"
      />
      <q-icon name="volume_up" color="white" size="20px" />
    </div>

    <!-- DrawerPanel overlay -->
    <DrawerPanel v-model="drawerOpen" />

    <!-- Bottom navigation -->
    <BottomNav />
  </div>
</template>

<script setup lang="ts">
// Feature: xp-music-player — NowPlayingView
// Requirements: 6.1, 6.2, 6.3, 6.4, 6.5, 6.6, 6.7, 6.8, 6.9

import { ref, computed, watch } from 'vue';
import { usePlayerStore } from 'src/stores/playerStore';
import { db } from 'src/services/db';
import VisualizerCanvas from 'src/components/VisualizerCanvas.vue';
import ProgressBar from 'src/components/ProgressBar.vue';
import DrawerPanel from 'src/components/DrawerPanel.vue';
import BottomNav from 'src/components/BottomNav.vue';
import type { Track } from 'src/types/models';

const playerStore = usePlayerStore();
const drawerOpen = ref(false);

// Current track metadata
const currentTrack = ref<Track | null>(null);

// Req 6.2: Track title (bold white); Req 6.9: placeholder when no track
const trackTitle = computed(() =>
  currentTrack.value ? currentTrack.value.title : 'No track loaded',
);

// Req 6.2: Artist (gray-white); Req 6.9: placeholder when no track
const trackArtist = computed(() =>
  currentTrack.value ? currentTrack.value.artist : 'Select a track to play',
);

// Watch currentTrackId and fetch Track metadata from DB
watch(
  () => playerStore.currentTrackId,
  async (id) => {
    if (!id) {
      currentTrack.value = null;
      return;
    }
    const track = await db.getTrack(id);
    currentTrack.value = track ?? null;
  },
  { immediate: true },
);

// Req 6.5: Toggle play/pause
function togglePlayPause(): void {
  if (playerStore.isPlaying) {
    playerStore.pause();
  } else {
    playerStore.resume();
  }
}

// Req 6.8: Volume slider input handler
function onVolumeInput(e: Event): void {
  const input = e.target as HTMLInputElement;
  playerStore.setVolume(parseFloat(input.value));
}
</script>

<style scoped lang="scss">
/* Req 6.1: Full-screen on black background */
.now-playing-view {
  position: fixed;
  inset: 0;
  background: #000;
  display: flex;
  flex-direction: column;
  align-items: center;
  overflow: hidden;
  color: white;
}

.drawer-handle {
  width: 100%;
  display: flex;
  justify-content: center;
  padding: 8px 0 4px;
  cursor: pointer;
  flex-shrink: 0;
  user-select: none;

  &:active {
    opacity: 0.7;
  }
}

.library-btn {
  position: absolute;
  top: 4px;
  right: 8px;
}

/* Req 6.1: Visualizer in upper ~40% */
.visualizer-area {
  width: 100%;
  flex: 0 0 40%;
  min-height: 0;
  display: flex;
  align-items: flex-start;
  justify-content: center;
  padding: 8px 16px;
  box-sizing: border-box;
}

.visualizer-card {
  width: 100%;
  height: 75%;
  min-height: 0;
  border-radius: 18px;
  border: 1px solid rgba(0, 255, 80, 0.35);
  background: rgba(0, 20, 0, 0.6);
  box-shadow:
    0 0 12px rgba(0, 255, 80, 0.4),
    0 0 28px rgba(0, 200, 60, 0.25),
    0 0 60px rgba(0, 150, 40, 0.12),
    inset 0 0 20px rgba(0, 80, 20, 0.2);
  overflow: hidden;
  position: relative;

  /* Glitter sparkle corners via pseudo-element */
  &::before {
    content: '';
    position: absolute;
    inset: 0;
    border-radius: 18px;
    background:
      radial-gradient(circle at 5% 5%, rgba(170, 255, 0, 0.15) 0%, transparent 40%),
      radial-gradient(circle at 95% 5%, rgba(0, 255, 120, 0.12) 0%, transparent 40%),
      radial-gradient(circle at 5% 95%, rgba(0, 255, 120, 0.1) 0%, transparent 40%),
      radial-gradient(circle at 95% 95%, rgba(170, 255, 0, 0.12) 0%, transparent 40%);
    pointer-events: none;
    z-index: 1;
  }
}

.track-info {
  width: 100%;
  padding: 12px 24px 4px;
  text-align: center;
  flex-shrink: 0;
}

/* Req 6.2: Track title bold white */
.track-title {
  font-size: 18px;
  font-weight: 700;
  color: #ffffff;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

/* Req 6.2: Artist gray-white */
.track-artist {
  font-size: 14px;
  color: rgba(255, 255, 255, 0.65);
  margin-top: 2px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.progress-area {
  width: 100%;
  padding: 12px 20px;
  flex-shrink: 0;
}

/* Req 6.4: Transport buttons */
.transport-controls {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 10px;
  padding: 4px 0;
  flex-shrink: 0;
}

.transport-btn {
  background: linear-gradient(180deg, #555 0%, #1a1a1a 100%) !important;
  border-radius: 50% !important;
  border: 1px solid #444 !important;
  box-shadow:
    0 3px 10px rgba(0, 0, 0, 0.8),
    inset 0 1px 0 rgba(255, 255, 255, 0.18),
    inset 0 -1px 0 rgba(0, 0, 0, 0.4) !important;

  &:hover {
    background: linear-gradient(180deg, #666 0%, #2a2a2a 100%) !important;
    box-shadow:
      0 3px 14px rgba(0, 0, 0, 0.9),
      inset 0 1px 0 rgba(255, 255, 255, 0.25),
      inset 0 -1px 0 rgba(0, 0, 0, 0.4) !important;
  }

  &:active {
    background: linear-gradient(180deg, #1a1a1a 0%, #333 100%) !important;
    box-shadow:
      0 1px 4px rgba(0, 0, 0, 0.8),
      inset 0 2px 4px rgba(0, 0, 0, 0.5) !important;
  }
}

.play-btn {
  background: linear-gradient(180deg, #666 0%, #222 100%) !important;
  box-shadow:
    0 4px 14px rgba(0, 0, 0, 0.9),
    inset 0 1px 0 rgba(255, 255, 255, 0.22),
    inset 0 -1px 0 rgba(0, 0, 0, 0.5) !important;
}

.loop-active {
  background: linear-gradient(180deg, #1a4a1a 0%, #0a2a0a 100%) !important;
  border-color: rgba(170, 255, 0, 0.5) !important;
  box-shadow:
    0 0 8px rgba(170, 255, 0, 0.4),
    0 3px 10px rgba(0, 0, 0, 0.8),
    inset 0 1px 0 rgba(170, 255, 0, 0.2) !important;
}

/* Req 6.8: Volume slider row */
.volume-area {
  display: flex;
  align-items: center;
  gap: 10px;
  width: 100%;
  padding: 8px 24px 80px;
  flex-shrink: 0;
}

.volume-slider {
  flex: 1;
  -webkit-appearance: none;
  appearance: none;
  height: 4px;
  border-radius: 2px;
  background: rgba(255, 255, 255, 0.25);
  outline: none;
  cursor: pointer;

  &::-webkit-slider-thumb {
    -webkit-appearance: none;
    appearance: none;
    width: 14px;
    height: 14px;
    border-radius: 50%;
    background: #ffffff;
    cursor: grab;
    box-shadow: 0 0 4px rgba(0, 0, 0, 0.5);
  }

  &::-moz-range-thumb {
    width: 14px;
    height: 14px;
    border-radius: 50%;
    background: #ffffff;
    cursor: grab;
    border: none;
    box-shadow: 0 0 4px rgba(0, 0, 0, 0.5);
  }
}
</style>
