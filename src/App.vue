<template>
  <router-view />
</template>

<script setup lang="ts">
import { onMounted, onUnmounted, watch } from 'vue';
import { usePlayerStore, setupMediaSession } from 'src/stores/playerStore';
import { audioEngine } from 'src/services/audioEngine';

let playerStore: ReturnType<typeof usePlayerStore>;
let persistTimeout: ReturnType<typeof setTimeout> | null = null;

function onKeydown(e: KeyboardEvent): void {
  // Don't capture when typing in input/textarea
  const tag = (e.target as HTMLElement)?.tagName;
  if (tag === 'INPUT' || tag === 'TEXTAREA' || (e.target as HTMLElement)?.isContentEditable) return;

  switch (e.code) {
    case 'Space':
      e.preventDefault();
      if (playerStore.isPlaying) playerStore.pause();
      else playerStore.resume();
      break;
    case 'ArrowRight':
      e.preventDefault();
      playerStore.seek(Math.min(playerStore.duration, playerStore.currentTime + 5));
      break;
    case 'ArrowLeft':
      e.preventDefault();
      playerStore.seek(Math.max(0, playerStore.currentTime - 5));
      break;
    case 'ArrowUp':
      e.preventDefault();
      playerStore.setVolume(Math.min(1, playerStore.volume + 0.05));
      break;
    case 'ArrowDown':
      e.preventDefault();
      playerStore.setVolume(Math.max(0, playerStore.volume - 0.05));
      break;
    case 'KeyN':
      playerStore.next();
      break;
    case 'KeyP':
      playerStore.previous();
      break;
    case 'KeyS':
      playerStore.toggleShuffle();
      break;
    case 'KeyL':
      playerStore.toggleLoop();
      break;
  }
}

onMounted(() => {
  playerStore = usePlayerStore();
  setupMediaSession(playerStore);

  // Restore queue from backend
  playerStore.restoreQueue();

  // Register service worker for PWA
  if ('serviceWorker' in navigator) {
    navigator.serviceWorker.register('/sw.js').catch(() => {});
  }

  // Debounced persist: save queue state 1s after last change
  watch(
    () => [playerStore.queue, playerStore.currentTrackId, playerStore.isShuffling, playerStore.isLooping, playerStore.volume] as const,
    () => {
      if (persistTimeout) clearTimeout(persistTimeout);
      persistTimeout = setTimeout(() => {
        playerStore.persistQueue();
      }, 1000);
    },
    { deep: true },
  );

  // Req 1.2: initialize AudioContext on first user interaction (autoplay policy)
  const initOnClick = () => {
    audioEngine.init();
    document.removeEventListener('click', initOnClick);
  };
  document.addEventListener('click', initOnClick);

  // Global keyboard shortcuts
  document.addEventListener('keydown', onKeydown);
});

onUnmounted(() => {
  document.removeEventListener('keydown', onKeydown);
});
</script>
