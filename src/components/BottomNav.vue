<template>
  <div class="bottom-nav">
    <!-- Mini player bar (shown when a track is loaded and not on player page) -->
    <div v-if="playerStore.currentTrackId && route.path !== '/'" class="mini-player" @click="router.push('/')">
      <div class="mini-art">
        <img v-if="currentArtwork" :src="currentArtwork" />
        <q-icon v-else name="music_note" color="grey-5" size="16px" />
      </div>
      <div class="mini-info">
        <div class="mini-title">{{ currentTitle }}</div>
        <div class="mini-artist">{{ currentArtist }}</div>
      </div>
      <q-btn flat round icon="skip_previous" color="white" size="xs" @click.stop="playerStore.previous()" />
      <q-btn
        flat round
        :icon="playerStore.isPlaying ? 'pause' : 'play_arrow'"
        color="white" size="sm"
        class="mini-play-btn"
        @click.stop="togglePlay"
      />
      <q-btn flat round icon="skip_next" color="white" size="xs" @click.stop="playerStore.next()" />
    </div>

    <!-- Nav tabs -->
    <div class="nav-tabs">
      <button class="nav-tab" :class="{ active: route.path === '/' }" @click="router.push('/')">
        <q-icon name="home" size="22px" />
        <span>HOME</span>
      </button>
      <button class="nav-tab" :class="{ active: route.path === '/library' }" @click="router.push('/library')">
        <q-icon name="library_music" size="22px" />
        <span>LIBRARY</span>
      </button>
      <button class="nav-tab" :class="{ active: route.path === '/search' }" @click="router.push('/search')">
        <q-icon name="search" size="22px" />
        <span>SEARCH</span>
      </button>
      <button class="nav-tab" :class="{ active: route.path === '/playlists' }" @click="router.push('/playlists')">
        <q-icon name="queue_music" size="22px" />
        <span>PLAYLISTS</span>
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { usePlayerStore } from 'src/stores/playerStore';
import { db } from 'src/services/db';

const route = useRoute();
const router = useRouter();
const playerStore = usePlayerStore();

const currentTitle = ref('');
const currentArtist = ref('');
const currentArtwork = ref<string | null>(null);

function togglePlay() {
  if (playerStore.isPlaying) playerStore.pause();
  else playerStore.resume();
}

watch(() => playerStore.currentTrackId, async (id) => {
  if (!id) { currentTitle.value = ''; currentArtist.value = ''; currentArtwork.value = null; return; }
  const track = await db.getTrack(id);
  if (!track) return;
  currentTitle.value = track.title;
  currentArtist.value = track.artist;
  if (track.artworkId) {
    const blob = await db.getArtwork(track.artworkId);
    currentArtwork.value = blob ? URL.createObjectURL(blob) : null;
  } else {
    currentArtwork.value = null;
  }
}, { immediate: true });
</script>

<style scoped lang="scss">
.bottom-nav {
  position: fixed;
  bottom: 0;
  left: 0;
  right: 0;
  z-index: 100;
  display: flex;
  flex-direction: column;
}

/* Mini player */
.mini-player {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 10px 14px;
  background: #1e2230;
  border-radius: 16px 16px 0 0;
  cursor: pointer;
  box-shadow: 0 -4px 20px rgba(0, 0, 0, 0.5);
}

.mini-art {
  width: 38px;
  height: 38px;
  border-radius: 8px;
  background: #2a2d3e;
  flex-shrink: 0;
  overflow: hidden;
  display: flex;
  align-items: center;
  justify-content: center;

  img { width: 100%; height: 100%; object-fit: cover; }
}

.mini-info {
  flex: 1;
  min-width: 0;
}

.mini-title {
  font-size: 13px;
  font-weight: 600;
  color: #fff;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.mini-artist {
  font-size: 10px;
  color: rgba(255, 255, 255, 0.45);
  text-transform: uppercase;
  letter-spacing: 0.05em;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.mini-play-btn {
  background: #3b82f6 !important;
  border-radius: 50% !important;
  width: 36px;
  height: 36px;
}

/* Nav tabs */
.nav-tabs {
  display: flex;
  background: #111318;
  border-top: 1px solid rgba(255, 255, 255, 0.07);
  padding: 6px 0 8px;
}

.nav-tab {
  flex: 1;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 3px;
  background: transparent;
  border: none;
  color: rgba(255, 255, 255, 0.35);
  font-size: 9px;
  letter-spacing: 0.06em;
  cursor: pointer;
  padding: 4px 0;
  transition: color 0.15s ease;

  &.active {
    color: #fff;

    .q-icon {
      background: rgba(255, 255, 255, 0.1);
      border-radius: 12px;
      padding: 4px 14px;
    }
  }
}
</style>
