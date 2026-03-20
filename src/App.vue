<template>
  <router-view />
</template>

<script setup lang="ts">
import { onMounted } from 'vue';
import { usePlayerStore, setupMediaSession } from 'src/stores/playerStore';
import { audioEngine } from 'src/services/audioEngine';

onMounted(() => {
  const playerStore = usePlayerStore();
  setupMediaSession(playerStore);

  // Req 1.2: initialize AudioContext on first user interaction (autoplay policy)
  const initOnClick = () => {
    audioEngine.init();
    document.removeEventListener('click', initOnClick);
  };
  document.addEventListener('click', initOnClick);
});
</script>
