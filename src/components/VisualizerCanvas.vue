<template>
  <canvas ref="canvasRef" class="vis-canvas" />
</template>

<script setup lang="ts">
// WMP-style bar visualizer.
//
// The analyser's raw FFT bins are linear in frequency, which leaves most of the
// upper bins effectively silent for music and makes the right side of the canvas
// look empty. We aggregate the bins into log-spaced bands instead, so the bars
// spread across the full width and follow what you actually hear.

import { ref, onMounted, onUnmounted } from 'vue';
import { audioEngine } from 'src/services/audioEngine';

const canvasRef = ref<HTMLCanvasElement | null>(null);

const BAR_COUNT = 48;
const GAP = 1;
const DECAY = 3;
const MIN_HZ = 40;
const MAX_HZ = 16000;
// Leave headroom at the top so loud bass doesn't touch the card edge.
const MAX_HEIGHT_RATIO = 0.8;
const BAR_COLOR = '#a4eb0c';
const PEAK_COLOR = '#dfeaf7';

const lastBar = new Float32Array(BAR_COUNT);
const lastTop = new Float32Array(BAR_COUNT);
const topSpeed = new Float32Array(BAR_COUNT);

let idle = false;
let rafId = 0;
let freqBuffer: Uint8Array<ArrayBuffer> | null = null;
let barGradient: CanvasGradient | null = null;

/** Linear interpolation between adjacent FFT bins. */
function sampleBin(data: Uint8Array, position: number): number {
  const lower = Math.floor(position);
  const upper = Math.min(data.length - 1, lower + 1);
  const fraction = position - lower;
  const a = data[Math.max(0, lower)] ?? 0;
  const b = data[upper] ?? a;
  return a + (b - a) * fraction;
}

/** Light spatial smoothing so the bar envelope flows instead of jumping. */
function smoothBands(bands: Float32Array): Float32Array {
  const smoothed = new Float32Array(BAR_COUNT);
  for (let i = 0; i < BAR_COUNT; i++) {
    const current = bands[i]!;
    const previous = bands[i - 1] ?? current;
    const next = bands[i + 1] ?? current;
    smoothed[i] = previous * 0.18 + current * 0.64 + next * 0.18;
  }
  return smoothed;
}

/**
 * Map linear FFT bins onto log-spaced bars between MIN_HZ and MAX_HZ.
 * Values are sampled (not repeated from the nearest bin) at several points per
 * band, then softly saturated so loud bands never clamp into a flat line.
 */
function computeBands(freqData: Uint8Array, sampleRate: number, fftSize: number): Float32Array {
  const bands = new Float32Array(BAR_COUNT);
  const binHz = sampleRate / fftSize;
  const maxHz = Math.min(MAX_HZ, sampleRate / 2);
  const ratio = maxHz / MIN_HZ;
  const samplesPerBand = 4;

  for (let i = 0; i < BAR_COUNT; i++) {
    const f0 = MIN_HZ * Math.pow(ratio, i / BAR_COUNT);
    const f1 = MIN_HZ * Math.pow(ratio, (i + 1) / BAR_COUNT);

    let sum = 0;
    let peak = 0;
    for (let sample = 0; sample < samplesPerBand; sample++) {
      const t = (sample + 0.5) / samplesPerBand;
      const frequency = f0 * Math.pow(f1 / f0, t);
      const value = sampleBin(freqData, Math.max(1, frequency / binHz));
      sum += value;
      if (value > peak) peak = value;
    }

    const average = sum / samplesPerBand;
    const mixed = (average * 0.6 + peak * 0.4) / 255;

    // Gentle high-frequency tilt compensates for the natural spectral roll-off.
    const tilt = 1 + (i / BAR_COUNT) * 0.35;

    // Exponential soft-knee: approaches 1 but never clamps to a flat top.
    bands[i] = 1 - Math.exp(-2.2 * mixed * tilt);
  }

  return smoothBands(bands);
}

function drawLoop(): void {
  const canvas = canvasRef.value;
  if (!canvas) return;

  const ctx = canvas.getContext('2d');
  if (!ctx) return;

  const displayW = canvas.clientWidth;
  const displayH = canvas.clientHeight;
  if (displayW === 0 || displayH === 0) {
    rafId = requestAnimationFrame(drawLoop);
    return;
  }

  // Render at device pixel ratio, but draw in CSS pixels.
  const dpr = Math.min(window.devicePixelRatio || 1, 2);
  const pixelW = Math.round(displayW * dpr);
  const pixelH = Math.round(displayH * dpr);
  if (canvas.width !== pixelW || canvas.height !== pixelH) {
    canvas.width = pixelW;
    canvas.height = pixelH;
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    barGradient = ctx.createLinearGradient(0, displayH * 0.1, 0, displayH);
    barGradient.addColorStop(0, '#d9ff66');
    barGradient.addColorStop(0.55, BAR_COLOR);
    barGradient.addColorStop(1, '#2e9e00');
    lastTop.fill(displayH);
    topSpeed.fill(0);
  }

  const w = displayW;
  const h = displayH;
  const maxBarHeight = h * MAX_HEIGHT_RATIO;

  const analyser = audioEngine.analyserNode;
  let bands: Float32Array;

  if (analyser) {
    if (!freqBuffer || freqBuffer.length !== analyser.frequencyBinCount) {
      freqBuffer = new Uint8Array(analyser.frequencyBinCount);
    }
    analyser.getByteFrequencyData(freqBuffer);
    bands = computeBands(freqBuffer, analyser.context.sampleRate, analyser.fftSize);
  } else {
    bands = new Float32Array(BAR_COUNT);
  }

  let maxBand = 0;
  for (let i = 0; i < BAR_COUNT; i++) {
    if (bands[i]! > maxBand) maxBand = bands[i]!;
  }

  // Pause drawing when the signal is silent and all peaks have fallen.
  if (idle && maxBand < 0.01) {
    rafId = requestAnimationFrame(drawLoop);
    return;
  }
  if (maxBand >= 0.01) idle = false;

  ctx.clearRect(0, 0, w, h);

  let anythingVisible = false;

  for (let i = 0; i < BAR_COUNT; i++) {
    const target = bands[i]! * maxBarHeight;

    // Instant attack, gradual decay.
    if (target > lastBar[i]!) {
      lastBar[i] = target;
    } else {
      lastBar[i] = Math.max(target, lastBar[i]! - DECAY);
    }
    if (lastBar[i]! < 0) lastBar[i] = 0;

    const barHeight = Math.round(lastBar[i]!);

    // Integer boundaries guarantee the bars tile the full canvas width.
    const x0 = Math.round((i * w) / BAR_COUNT);
    const x1 = Math.round(((i + 1) * w) / BAR_COUNT);
    const barWidth = Math.max(1, x1 - x0 - GAP);

    if (barHeight > 0) {
      ctx.fillStyle = barGradient ?? BAR_COLOR;
      ctx.fillRect(x0, h - barHeight, barWidth, barHeight);
      anythingVisible = true;
    }

    // Peak dot with accelerating fall.
    const barTop = h - barHeight;
    if (barTop < lastTop[i]!) {
      lastTop[i] = barTop;
      topSpeed[i] = 0;
    } else if (lastTop[i]! < h) {
      topSpeed[i] = Math.min(topSpeed[i]! + 0.7, 8);
      lastTop[i] = Math.min(h, lastTop[i]! + topSpeed[i]!);
    }

    if (lastTop[i]! < h - 1) {
      ctx.fillStyle = PEAK_COLOR;
      ctx.fillRect(x0, lastTop[i]!, barWidth, 2);
      anythingVisible = true;
    }
  }

  if (!anythingVisible && maxBand < 0.005) idle = true;

  rafId = requestAnimationFrame(drawLoop);
}

onMounted(() => {
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
