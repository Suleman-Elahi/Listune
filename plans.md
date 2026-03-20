high-performance Music Player PWA using Quasar (Vue 3).
Project Phase 1: Core Audio Engine (The Singleton)
Goal: Create a robust, low-level audio controller that handles both streaming and visualization.
Implementation:
Create a src/services/audioEngine.js singleton.
Use a single native new Audio() element.
Initialize AudioContext only on user interaction (to satisfy browser policies).
Set up a MediaElementSource → AnalyserNode → Destination chain.
Performance Key: Use analyser.fftSize = 64 (or 128) and smoothingTimeConstant = 0.8 to get that specific "thick bar" XP movement without wasting CPU cycles.
Phase 2: The Data Layer (IndexedDB + Pinia)
Goal: Handle thousands of tracks with zero UI lag.
Implementation:
IndexedDB (idb): Stores the "Library." Each entry includes metadata, album art blobs, and a "source" tag (local, s3, or nextcloud).
Pinia: Stores only the currentTrackId, isPlaying state, and an array of trackIds for the current queue.
Performance Key: Never store large metadata objects in Pinia. Fetch details from IndexedDB by ID only when needed for the "Now Playing" view. Use markRaw() for any audio-related objects in Vue state.
Phase 3: Cloud & Local File Integration
Goal: Stream audio without pre-downloading the entire file.
Implementation:
S3/S3-Compatible: Use a lightweight backend (or Lambda) to generate Pre-signed URLs. The PWA fetches the URL and sets it as audio.src.
NextCloud: Use WebDAV protocol. Perform a PROPFIND request to list files and use the direct file URL with Authorization headers for streaming.
Local Files: Implement the File System Access API.
window.showDirectoryPicker() to get a folder handle.
Store the handle in IndexedDB for persistence.
Stream via URL.createObjectURL(file).
Performance Key: Use HTTP Range headers to fetch only the first 128KB of cloud files to extract ID3 tags without downloading the whole song.
Phase 4: The "XP-Style" Visualization Engine
Goal: Recreate the animated bars + falling peaks.
Implementation:
Create a custom Vue component with a <canvas>.
The Peak Decay Algorithm:
Get frequency data via analyser.getByteFrequencyData.
Maintain a peaks[] array.
If currentValue > peaks[i], update peak. Else, peaks[i] -= gravity.
Visual Style: Use ctx.fillRect() with a 2px gap between bars.
Performance Key: Wrap the drawing logic in requestAnimationFrame. If performance dips on mobile, implement OffscreenCanvas in a Web Worker to move rendering off the main UI thread.
Phase 5: Background Worker (The Tagger)
Goal: Parse MP3 tags without freezing the UI.
Implementation:
Setup a Web Worker (src/workers/scanner.worker.js).
Use music-metadata-browser inside the worker.
When a folder (Local or Cloud) is added, the worker "walks" the file list, extracts tags, and writes them directly to IndexedDB in batches.
Performance Key: Process files in chunks (e.g., 20 at a time) to ensure the device remains responsive and doesn't overheat.
Phase 6: PWA & OS Integration
Goal: Background playback and lock-screen control.
Implementation:
Media Session API: Sync navigator.mediaSession.metadata whenever the track changes. Map "Next/Prev/Play/Pause" hardware keys to your Audio Engine.
Quasar PWA Mode: Configure src-pwa/register-service-worker.js to cache the app shell but exclude audio streams (to avoid memory bloat).
Optimization: Use QVirtualScroll for the library list to ensure only the visible ~10-15 rows are rendered in the DOM.
Architecture Summary Table
Component	Technology	Optimization Strategy
UI Framework	Quasar (Vue 3)	Virtualized lists + markRaw reactivity
Audio Core	Native <audio>	Direct streaming (no Howler.js overhead)
Visualization	HTML5 Canvas	Peak Decay Algorithm + requestAnimationFrame
Database	IndexedDB	Store Blobs for art, file handles for local access
Metadata	Web Workers	Off-main-thread parsing using Range requests
Streaming	S3 / WebDAV	Pre-signed URLs + native browser buffering
