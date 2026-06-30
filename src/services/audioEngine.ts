// Feature: xp-music-player — AudioEngine singleton
// Gapless playback via dual audio elements with preloading

type AudioEventName = 'playback-blocked' | 'ended' | 'error';

interface AudioEngineInterface {
  init(): void;
  destroy(): void;
  loadAndPlay(url: string): Promise<void>;
  preloadNext(url: string): void;
  playPreloaded(): Promise<boolean>;
  pause(): void;
  resume(): void;
  seek(seconds: number): void;
  setVolume(level: number): void;
  readonly analyserNode: AnalyserNode | null;
  readonly currentAudio: HTMLAudioElement;
  on(event: AudioEventName, cb: () => void): void;
  off(event: string, cb: () => void): void;
}

// Dual audio elements for gapless transitions
const audioA = new Audio();
audioA.crossOrigin = 'anonymous';
const audioB = new Audio();
audioB.crossOrigin = 'anonymous';

// Which element is currently active
let _activeAudio: HTMLAudioElement = audioA;
let _preloadAudio: HTMLAudioElement = audioB;
let _preloadedUrl: string | null = null;

let _audioContext: AudioContext | null = null;
let _analyserNode: AnalyserNode | null = null;
let _sourceNodeA: MediaElementAudioSourceNode | null = null;
let _sourceNodeB: MediaElementAudioSourceNode | null = null;
let _gainA: GainNode | null = null;
let _gainB: GainNode | null = null;
let _isPlaying = false;

// Simple event bus
const _listeners = new Map<string, Set<() => void>>();

function _emit(event: string): void {
  const cbs = _listeners.get(event);
  if (cbs) cbs.forEach((cb) => cb());
}

function _onEnded(): void {
  _isPlaying = false;
  _emit('ended');
}

// Wire ended event to both elements
audioA.addEventListener('ended', () => {
  if (_activeAudio === audioA) _onEnded();
});
audioB.addEventListener('ended', () => {
  if (_activeAudio === audioB) _onEnded();
});

function _getActiveGain(): GainNode | null {
  return _activeAudio === audioA ? _gainA : _gainB;
}

function _getPreloadGain(): GainNode | null {
  return _preloadAudio === audioA ? _gainA : _gainB;
}

const audioEngine: AudioEngineInterface = {
  init(): void {
    if (_audioContext) return;

    _audioContext = new AudioContext();
    _analyserNode = _audioContext.createAnalyser();
    _analyserNode.fftSize = 128;
    _analyserNode.smoothingTimeConstant = 0.8;

    // Create gain nodes for crossfade control
    _gainA = _audioContext.createGain();
    _gainB = _audioContext.createGain();

    // Connect both audio elements through gain nodes to analyser
    _sourceNodeA = _audioContext.createMediaElementSource(audioA);
    _sourceNodeA.connect(_gainA);
    _gainA.connect(_analyserNode);

    _sourceNodeB = _audioContext.createMediaElementSource(audioB);
    _sourceNodeB.connect(_gainB);
    _gainB.connect(_analyserNode);

    _analyserNode.connect(_audioContext.destination);

    // Start with A active, B silent
    _gainA.gain.value = 1;
    _gainB.gain.value = 0;
  },

  destroy(): void {
    _isPlaying = false;
    audioA.pause();
    audioA.src = '';
    audioB.pause();
    audioB.src = '';
    _sourceNodeA?.disconnect();
    _sourceNodeB?.disconnect();
    _gainA?.disconnect();
    _gainB?.disconnect();
    _analyserNode?.disconnect();
    _audioContext?.close().catch(() => undefined);
    _audioContext = null;
    _analyserNode = null;
    _sourceNodeA = null;
    _sourceNodeB = null;
    _gainA = null;
    _gainB = null;
    _listeners.clear();
  },

  async loadAndPlay(url: string): Promise<void> {
    // Stop preloaded track if any
    _preloadAudio.pause();
    _preloadAudio.src = '';
    _preloadedUrl = null;

    if (_isPlaying) {
      _activeAudio.pause();
      _activeAudio.currentTime = 0;
      _isPlaying = false;
    }

    // Ensure gains are correct
    const activeGain = _getActiveGain();
    const preloadGain = _getPreloadGain();
    if (activeGain) activeGain.gain.value = 1;
    if (preloadGain) preloadGain.gain.value = 0;

    _activeAudio.src = url;

    try {
      await _activeAudio.play();
      _isPlaying = true;
    } catch (err: unknown) {
      if (err instanceof DOMException && err.name === 'NotAllowedError') {
        _isPlaying = false;
        _emit('playback-blocked');
      } else {
        _isPlaying = false;
        _emit('error');
      }
    }
  },

  // Preload the next track URL into the inactive audio element
  preloadNext(url: string): void {
    _preloadAudio.src = url;
    _preloadAudio.preload = 'auto';
    _preloadedUrl = url;
    // Load but don't play
    _preloadAudio.load();
  },

  // Instantly switch to the preloaded track (gapless transition)
  async playPreloaded(): Promise<boolean> {
    if (!_preloadedUrl) return false;

    // Swap active/preload
    const oldActive = _activeAudio;
    const oldActiveGain = _getActiveGain();
    _activeAudio = _preloadAudio;
    _preloadAudio = oldActive;

    // Set gains: new active = 1, old = 0
    const newActiveGain = _getActiveGain();
    const newPreloadGain = _getPreloadGain();
    if (newActiveGain) newActiveGain.gain.value = 1;
    if (newPreloadGain) newPreloadGain.gain.value = 0;

    // Stop old element
    oldActive.pause();
    oldActive.src = '';

    _preloadedUrl = null;

    try {
      await _activeAudio.play();
      _isPlaying = true;
      return true;
    } catch {
      _isPlaying = false;
      _emit('error');
      return false;
    }
  },

  pause(): void {
    _activeAudio.pause();
    _isPlaying = false;
  },

  resume(): void {
    _activeAudio.play().then(() => {
      _isPlaying = true;
    }).catch(() => {
      _isPlaying = false;
      _emit('playback-blocked');
    });
  },

  seek(seconds: number): void {
    _activeAudio.currentTime = seconds;
  },

  setVolume(level: number): void {
    const vol = Math.min(1, Math.max(0, level));
    audioA.volume = vol;
    audioB.volume = vol;
  },

  get analyserNode(): AnalyserNode | null {
    return _analyserNode;
  },

  get currentAudio(): HTMLAudioElement {
    return _activeAudio;
  },

  on(event: AudioEventName, cb: () => void): void {
    if (!_listeners.has(event)) {
      _listeners.set(event, new Set());
    }
    _listeners.get(event)!.add(cb);
  },

  off(event: string, cb: () => void): void {
    _listeners.get(event)?.delete(cb);
  },
};

export { audioEngine, audioA as _audioElement };
export type { AudioEngineInterface, AudioEventName };
