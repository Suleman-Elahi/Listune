<template>
  <canvas ref="canvasRef" style="width: 100%; height: 100%; display: block" />
</template>

<script setup lang="ts">
// Feature: xp-music-player
// Requirements: 2.1, 2.2, 2.3, 2.4, 2.5, 2.6, 2.7, 2.8, 2.9, 2.10

import { ref, onMounted, onUnmounted } from 'vue';
import { audioEngine } from 'src/services/audioEngine';

const canvasRef = ref<HTMLCanvasElement | null>(null);

// Internal state
let peaks: Float32Array = new Float32Array(0);
const gravity = 1.5; // Req 2.6: decay constant
let rafId = 0;
let worker: Worker | null = null;
let offscreenTransferred = false;

// FPS tracking
const FPS_WINDOW = 60;
const FPS_WARMUP = 30;
const fpsTimestamps: number[] = [];
let frameCount = 0;

function computeFps(): number {
  if (fpsTimestamps.length < 2) return 60;
  const oldest = fpsTimestamps[0]!;
  const newest = fpsTimestamps[fpsTimestamps.length - 1]!;
  const elapsed = (newest - oldest) / 1000;
  return elapsed > 0 ? (fpsTimestamps.length - 1) / elapsed : 60;
}

function trackFps(now: number): void {
  fpsTimestamps.push(now);
  if (fpsTimestamps.length > FPS_WINDOW) {
    fpsTimestamps.shift();
  }
}

// Req 2.9: Transfer canvas to OffscreenCanvas worker
function tryOffloadToWorker(canvas: HTMLCanvasElement, analyser: AnalyserNode): boolean {
  if (typeof OffscreenCanvas === 'undefined' || !canvas.transferControlToOffscreen) {
    return false;
  }
  try {
    const offscreen = canvas.transferControlToOffscreen();
    // SharedArrayBuffer for frequency data (requires COOP/COEP headers)
    // Fall back to passing analyser buffer size only; worker will use its own polling
    worker = new Worker(new URL('../workers/visualizer.worker.ts', import.meta.url), {
      type: 'module',
    });
    worker.postMessage(
      {
        type: 'init',
        canvas: offscreen,
        binCount: analyser.frequencyBinCount,
        gravity,
      },
      [offscreen],
    );
    offscreenTransferred = true;
    return true;
  } catch {
    return false;
  }
}

// Main draw loop (main-thread path)
function drawLoop(now: number): void {
  const canvas = canvasRef.value;
  if (!canvas || offscreenTransferred) return;

  const analyser = audioEngine.analyserNode;

  // Req 2.10: When no analyser, decay peaks to zero then stop
  if (!analyser) {
    let allZero = true;
    for (let i = 0; i < peaks.length; i++) {
      if (peaks[i]! > 0) {
        peaks[i] = Math.max(0, peaks[i]! - gravity);
        allZero = false;
      }
    }
    renderBars(canvas, new Uint8Array(peaks.length));
    if (!allZero) {
      rafId = requestAnimationFrame(drawLoop);
    } else {
      rafId = 0;
    }
    return;
  }

  const binCount = analyser.frequencyBinCount;

  // Re-allocate if bin count changed
  if (peaks.length !== binCount) {
    peaks = new Float32Array(binCount);
  }

  // Req 2.1: Read frequency data every frame
  const freqData = new Uint8Array(binCount);
  analyser.getByteFrequencyData(freqData);

  // Req 2.5 & 2.6: Update peaks
  for (let i = 0; i < binCount; i++) {
    if (freqData[i]! > peaks[i]!) {
      // Req 2.5: raise peak to current frequency
      peaks[i] = freqData[i]!;
    } else {
      // Req 2.6: decay peak by gravity
      peaks[i] = Math.max(0, peaks[i]! - gravity);
    }
  }

  renderBars(canvas, freqData);

  // FPS tracking for Req 2.9
  trackFps(now);
  frameCount++;

  if (frameCount > FPS_WARMUP && !offscreenTransferred) {
    const fps = computeFps();
    if (fps < 30) {
      // Req 2.9: offload to OffscreenCanvas worker
      const offloaded = tryOffloadToWorker(canvas, analyser);
      if (offloaded) {
        // Cancel main-thread RAF; switch to worker-feed loop
        if (rafId) {
          cancelAnimationFrame(rafId);
          rafId = 0;
        }
        // Start a RAF loop that only feeds frequency data to the worker
        const feedLoop = (): void => {
          if (!offscreenTransferred || !worker) return;
          const a = audioEngine.analyserNode;
          if (a) {
            const fd = new Uint8Array(a.frequencyBinCount);
            a.getByteFrequencyData(fd);
            worker.postMessage({ type: 'frame', freqData: fd });
          }
          rafId = requestAnimationFrame(feedLoop);
        };
        rafId = requestAnimationFrame(feedLoop);
        return;
      }
      // Fall back: reset counter so we don't keep trying
      frameCount = 0;
    }
  }

  rafId = requestAnimationFrame(drawLoop);
}

// Req 2.3: Draw bars with 1px gaps; Req 2.7: lime-green peak dots
function renderBars(canvas: HTMLCanvasElement, freqData: Uint8Array): void {
  const ctx = canvas.getContext('2d');
  if (!ctx) return;

  const w = canvas.width;
  const h = canvas.height;
  const binCount = freqData.length || peaks.length;
  if (binCount === 0) return;

  // Narrow bars: 3px wide, 1px gap — matching XP/classic visualizer style
  const gap = 1;
  const barWidth = 3;
  const totalBars = Math.min(binCount, Math.floor(w / (barWidth + gap)));

  ctx.clearRect(0, 0, w, h);

  for (let i = 0; i < totalBars; i++) {
    const x = i * (barWidth + gap);
    const amplitude = freqData[i] ?? 0;
    const barHeight = Math.floor((amplitude / 255) * h);

    if (barHeight > 0) {
      // Lime-green bar with slight brightness gradient (bright top, slightly darker bottom)
      const gradient = ctx.createLinearGradient(0, h - barHeight, 0, h);
      gradient.addColorStop(0, '#AAFF00');
      gradient.addColorStop(1, '#33CC00');
      ctx.fillStyle = gradient;
      ctx.fillRect(x, h - barHeight, barWidth, barHeight);
    }

    // Req 2.7: 2px-tall bright lime-green peak dot
    const peakVal = peaks[i] ?? 0;
    if (peakVal > 0) {
      const peakY = Math.floor(h - (peakVal / 255) * h) - 3;
      ctx.fillStyle = '#CCFF00';
      ctx.fillRect(x, Math.max(0, peakY), barWidth, 2);
    }
  }
}

onMounted(() => {
  const canvas = canvasRef.value;
  if (!canvas) return;

  // Size canvas to its CSS display size
  const resizeObserver = new ResizeObserver((entries) => {
    for (const entry of entries) {
      const { width, height } = entry.contentRect;
      canvas.width = Math.floor(width);
      canvas.height = Math.floor(height);
    }
  });
  resizeObserver.observe(canvas);

  // Store observer for cleanup
  (canvas as HTMLCanvasElement & { _resizeObserver?: ResizeObserver })._resizeObserver =
    resizeObserver;

  // Req 2.2: analyser settings already applied in audioEngine.init()
  const analyser = audioEngine.analyserNode;
  if (analyser) {
    peaks = new Float32Array(analyser.frequencyBinCount);
  }

  // Req 2.8: Start RAF loop
  rafId = requestAnimationFrame(drawLoop);
});

onUnmounted(() => {
  // Req 2.8: Cancel RAF on unmount
  if (rafId) {
    cancelAnimationFrame(rafId);
    rafId = 0;
  }

  // Terminate worker if offloaded
  if (worker) {
    worker.postMessage({ type: 'stop' });
    worker.terminate();
    worker = null;
  }

  // Clean up resize observer
  const canvas = canvasRef.value as (HTMLCanvasElement & { _resizeObserver?: ResizeObserver }) | null;
  canvas?._resizeObserver?.disconnect();
});

// Exposed for testing
defineExpose({ _peaks: peaks, _gravity: gravity });
</script>
