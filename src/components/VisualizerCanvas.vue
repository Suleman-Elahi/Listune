<template>
  <canvas ref="canvasRef" class="vis-canvas" />
</template>

<script setup lang="ts">
// WMP-style bar visualizer ported from wmpvis.js logic
// Single canvas with bar + peak dot rendering, physics-based decay

import { ref, onMounted, onUnmounted } from 'vue';
import { audioEngine } from 'src/services/audioEngine';

const canvasRef = ref<HTMLCanvasElement | null>(null);

// Config
const DEC_SPEED = 2;
const PRIMARY_SCALE = 1.0;
const DIFF_SCALE = 0.07;
const BAR_COLOR = '#a4eb0c';
const TOP_COLOR = '#dfeaf7';
const GAP = 1;

// State arrays (pre-allocated for up to 128 bins)
const lastAud: number[] = new Array(128).fill(0);
const lastBar: number[] = new Array(128).fill(0);
const lastTop: number[] = new Array(128).fill(0);
const topSpeed: number[] = new Array(128).fill(0);

let idle = false;
let rafId = 0;

function drawLoop(): void {
  const canvas = canvasRef.value;
  if (!canvas) return;

  const ctx = canvas.getContext('2d');
  if (!ctx) return;

  // Sync canvas buffer to its displayed CSS size every frame
  // This is the bulletproof way to ensure drawing fills the element
  const displayW = canvas.clientWidth;
  const displayH = canvas.clientHeight;
  if (canvas.width !== displayW || canvas.height !== displayH) {
    canvas.width = displayW;
    canvas.height = displayH;
    // Reset top positions after resize
    lastTop.fill(displayH);
  }

  const w = canvas.width;
  const h = canvas.height;

  if (w === 0 || h === 0) {
    rafId = requestAnimationFrame(drawLoop);
    return;
  }

  const analyser = audioEngine.analyserNode;

  // Get frequency data
  let binCount = 63;
  let audioArray: number[];

  if (analyser) {
    const rawCount = analyser.frequencyBinCount;
    const freqData = new Uint8Array(rawCount);
    analyser.getByteFrequencyData(freqData);
    // Skip bin 0 (DC offset, always maxed), normalize to 0–1
    audioArray = [];
    for (let i = 1; i < rawCount; i++) {
      audioArray.push(freqData[i]! / 255);
    }
    binCount = audioArray.length;
  } else {
    audioArray = new Array(binCount).fill(0);
  }

  // Idle optimization
  if (idle) {
    const randomIdx = Math.floor(Math.random() * binCount);
    if ((audioArray[randomIdx] ?? 0) <= 0.0001) {
      rafId = requestAnimationFrame(drawLoop);
      return;
    }
    idle = false;
  }

  // Clear entire canvas
  ctx.clearRect(0, 0, w, h);

  // Bar width: uniform, filling the full canvas width
  const barWidth = w / binCount;

  let allZero = true;

  for (let i = 0; i < binCount; i++) {
    const amplitude = audioArray[i] ?? 0;
    const x = barWidth * i;
    const drawWidth = barWidth - GAP;

    // Bar height with decay
    const height = Math.min(Math.round(h * amplitude * PRIMARY_SCALE), h);

    if (height > lastBar[i]!) {
      lastBar[i] = height;
    } else {
      lastBar[i]! -= DEC_SPEED;
      const diff = amplitude - (lastAud[i] ?? 0);
      if (diff > 0.1) {
        lastBar[i]! += Math.round(360 * diff * DIFF_SCALE);
        if (lastBar[i]! > h) lastBar[i] = h;
      }
    }
    if (lastBar[i]! < 0) lastBar[i] = 0;

    // Draw bar
    ctx.fillStyle = BAR_COLOR;
    ctx.fillRect(x, h - lastBar[i]!, drawWidth, lastBar[i]!);

    // Peak dot with gravity
    const topPos = h - lastBar[i]! - 2;
    if (topPos < lastTop[i]!) {
      lastTop[i] = topPos;
      topSpeed[i] = 0;
      allZero = false;
    } else if (lastTop[i]! < h - 2) {
      // Accelerating fall (wmpvis physics)
      if (topSpeed[i]! > 38) {
        lastTop[i]! += 5;
      } else if (topSpeed[i]! > 26) {
        lastTop[i]! += 4;
        topSpeed[i]! += 1;
      } else if (topSpeed[i]! > 18) {
        lastTop[i]! += 3;
        topSpeed[i]! += 1;
      } else if (topSpeed[i]! > 10) {
        lastTop[i]! += 2;
        topSpeed[i]! += 1;
      } else {
        topSpeed[i]! += 1;
      }
      allZero = false;
    }

    // Draw peak dot
    if (lastTop[i]! < h - 2) {
      ctx.fillStyle = TOP_COLOR;
      ctx.fillRect(x, lastTop[i]!, drawWidth, 2);
    }

    lastAud[i] = amplitude;
  }

  if (allZero) idle = true;

  rafId = requestAnimationFrame(drawLoop);
}

onMounted(() => {
  const canvas = canvasRef.value;
  if (!canvas) return;

  // Initialize canvas buffer to match CSS size
  canvas.width = canvas.clientWidth;
  canvas.height = canvas.clientHeight;
  lastTop.fill(canvas.height);

  rafId = requestAnimationFrame(drawLoop);
});

onUnmounted(() => {
  if (rafId) {
    cancelAnimationFrame(rafId);
    rafId = 0;
  }
});
</script>

<style scoped>
.vis-canvas {
  width: 100%;
  height: 100%;
  display: block;
}
</style>
