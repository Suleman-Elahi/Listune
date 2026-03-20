<template>
  <div class="progress-bar">
    <span class="progress-bar__time">{{ formatTime(currentTime) }}</span>

    <div
      ref="trackRef"
      class="progress-bar__track"
      @pointerdown="onPointerDown"
    >
      <div class="progress-bar__fill" :style="{ width: fillPercent + '%' }" />
      <div
        class="progress-bar__thumb"
        :style="{ left: fillPercent + '%' }"
        @pointerdown.stop="onPointerDown"
      />
    </div>

    <span class="progress-bar__time">{{ formatTime(duration) }}</span>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onUnmounted } from 'vue';

const props = defineProps<{
  currentTime: number;
  duration: number;
}>();

const emit = defineEmits<{
  seek: [seconds: number];
}>();

const trackRef = ref<HTMLElement | null>(null);
const isDragging = ref(false);

const fillPercent = computed(() => {
  if (!props.duration || props.duration <= 0) return 0;
  return Math.min(100, Math.max(0, (props.currentTime / props.duration) * 100));
});

function formatTime(seconds: number): string {
  if (!isFinite(seconds) || seconds < 0) return '0:00';
  const s = Math.floor(seconds);
  const mins = Math.floor(s / 60);
  const secs = s % 60;
  return `${mins}:${secs.toString().padStart(2, '0')}`;
}

function computeSeekTime(clientX: number): number {
  const track = trackRef.value;
  if (!track || !props.duration) return 0;
  const rect = track.getBoundingClientRect();
  const ratio = Math.min(1, Math.max(0, (clientX - rect.left) / rect.width));
  return ratio * props.duration;
}

function onPointerDown(e: PointerEvent) {
  isDragging.value = true;
  (e.currentTarget as HTMLElement).setPointerCapture(e.pointerId);
  emit('seek', computeSeekTime(e.clientX));

  window.addEventListener('pointermove', onPointerMove);
  window.addEventListener('pointerup', onPointerUp, { once: true });
}

function onPointerMove(e: PointerEvent) {
  if (!isDragging.value) return;
  emit('seek', computeSeekTime(e.clientX));
}

function onPointerUp(e: PointerEvent) {
  isDragging.value = false;
  emit('seek', computeSeekTime(e.clientX));
  window.removeEventListener('pointermove', onPointerMove);
}

onUnmounted(() => {
  window.removeEventListener('pointermove', onPointerMove);
  window.removeEventListener('pointerup', onPointerUp);
});
</script>

<style scoped lang="scss">
.progress-bar {
  display: flex;
  align-items: center;
  gap: 10px;
  width: 100%;
  user-select: none;

  &__time {
    font-size: 12px;
    color: rgba(255, 255, 255, 0.7);
    min-width: 36px;
    text-align: center;
    font-variant-numeric: tabular-nums;
  }

  &__track {
    position: relative;
    flex: 1;
    height: 4px;
    background: rgba(0, 80, 0, 0.5);
    border-radius: 2px;
    cursor: pointer;

    &:hover .progress-bar__thumb {
      transform: translateX(-50%) scale(1.3);
    }
  }

  &__fill {
    position: absolute;
    top: 0;
    left: 0;
    height: 100%;
    background: linear-gradient(90deg, #33cc00, #aaff00);
    border-radius: 2px;
    pointer-events: none;
    box-shadow: 0 0 6px #aaff00, 0 0 12px rgba(170, 255, 0, 0.5);
  }

  &__thumb {
    position: absolute;
    top: 50%;
    width: 14px;
    height: 14px;
    background: radial-gradient(circle at 40% 35%, #ccff66, #44cc00);
    border-radius: 50%;
    transform: translateX(-50%) translateY(-50%);
    cursor: grab;
    transition: transform 0.1s ease;
    box-shadow: 0 0 8px #aaff00, 0 0 16px rgba(170, 255, 0, 0.6), 0 1px 3px rgba(0,0,0,0.8);

    &:active {
      cursor: grabbing;
    }
  }
}
</style>
