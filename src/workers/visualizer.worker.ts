// Feature: xp-music-player
// Requirements: 2.9
// VisualizerWorker — runs the peak-decay draw loop on an OffscreenCanvas.
//
// Message protocol (main → worker):
//   { type: 'init', canvas: OffscreenCanvas, binCount: number, gravity: number }
//   { type: 'frame', freqData: Uint8Array }
//   { type: 'stop' }

let ctx: OffscreenCanvasRenderingContext2D | null = null;
let peaks: Float32Array = new Float32Array(0);
let gravity = 1.5;
let binCount = 0;
let running = false;

function renderBars(freqData: Uint8Array): void {
  if (!ctx) return;

  const canvas = ctx.canvas;
  const w = canvas.width;
  const h = canvas.height;

  if (binCount === 0) return;

  const gap = 1;
  const barWidth = 3;
  const totalBars = Math.min(binCount, Math.floor(w / (barWidth + gap)));

  ctx.clearRect(0, 0, w, h);

  for (let i = 0; i < totalBars; i++) {
    const x = i * (barWidth + gap);
    const amplitude = freqData[i] ?? 0;
    const barHeight = Math.floor((amplitude / 255) * h);

    if (barHeight > 0) {
      const gradient = ctx.createLinearGradient(0, h - barHeight, 0, h);
      gradient.addColorStop(0, '#AAFF00');
      gradient.addColorStop(1, '#33CC00');
      ctx.fillStyle = gradient;
      ctx.fillRect(x, h - barHeight, barWidth, barHeight);
    }

    // 2px-tall bright lime-green peak dot
    const peakVal = peaks[i] ?? 0;
    if (peakVal > 0) {
      const peakY = Math.floor(h - (peakVal / 255) * h) - 3;
      ctx.fillStyle = '#CCFF00';
      ctx.fillRect(x, Math.max(0, peakY), barWidth, 2);
    }
  }
}

self.addEventListener('message', (event: MessageEvent) => {
  const data = event.data as
    | { type: 'init'; canvas: OffscreenCanvas; binCount: number; gravity: number }
    | { type: 'frame'; freqData: Uint8Array }
    | { type: 'stop' };

  if (data.type === 'init') {
    const offscreen = data.canvas;
    ctx = offscreen.getContext('2d') as OffscreenCanvasRenderingContext2D | null;
    binCount = data.binCount;
    gravity = data.gravity ?? 1.5;
    peaks = new Float32Array(binCount);
    running = true;
  } else if (data.type === 'frame') {
    if (!running || !ctx) return;

    const freqData = data.freqData;

    // Sync binCount if it changed
    if (freqData.length !== binCount) {
      binCount = freqData.length;
      peaks = new Float32Array(binCount);
    }

    // Update peaks: raise or decay
    for (let i = 0; i < binCount; i++) {
      if ((freqData[i] ?? 0) > (peaks[i] ?? 0)) {
        peaks[i] = freqData[i]!;
      } else {
        peaks[i] = Math.max(0, (peaks[i] ?? 0) - gravity);
      }
    }

    renderBars(freqData);
  } else if (data.type === 'stop') {
    running = false;
    self.close();
  }
});
