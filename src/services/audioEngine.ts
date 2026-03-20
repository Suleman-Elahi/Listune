// Feature: xp-music-player — AudioEngine singleton
// Requirements: 1.1, 1.2, 1.3, 1.4, 1.5, 1.9, 1.10

type AudioEventName = 'playback-blocked' | 'ended' | 'error';

interface AudioEngineInterface {
  init(): void;
  destroy(): void;
  loadAndPlay(url: string): Promise<void>;
  pause(): void;
  resume(): void;
  seek(seconds: number): void;
  setVolume(level: number): void;
  readonly analyserNode: AnalyserNode | null;
  on(event: AudioEventName, cb: () => void): void;
  off(event: string, cb: () => void): void;
}

// Req 1.1: Single HTMLAudioElement for the lifetime of the app
const audio = new Audio();

let _audioContext: AudioContext | null = null;
let _analyserNode: AnalyserNode | null = null;
let _sourceNode: MediaElementAudioSourceNode | null = null;
let _isPlaying = false;

// Simple event bus: Map<eventName, Set<callback>>
const _listeners = new Map<string, Set<() => void>>();

function _emit(event: string): void {
  const cbs = _listeners.get(event);
  if (cbs) {
    cbs.forEach((cb) => cb());
  }
}

// Wire audio.ended → emit 'ended'
audio.addEventListener('ended', () => {
  _isPlaying = false;
  _emit('ended');
});

const audioEngine: AudioEngineInterface = {
  // Req 1.2: AudioContext initialized on first user interaction
  init(): void {
    if (_audioContext) return;

    _audioContext = new AudioContext();
    _analyserNode = _audioContext.createAnalyser();
    _analyserNode.fftSize = 2048;
    _analyserNode.smoothingTimeConstant = 0.8;

    _sourceNode = _audioContext.createMediaElementSource(audio);
    _sourceNode.connect(_analyserNode);
    _analyserNode.connect(_audioContext.destination);
  },

  destroy(): void {
    _isPlaying = false;
    audio.pause();
    audio.src = '';
    _sourceNode?.disconnect();
    _analyserNode?.disconnect();
    _audioContext?.close().catch(() => undefined);
    _audioContext = null;
    _analyserNode = null;
    _sourceNode = null;
    _listeners.clear();
  },

  // Req 1.3: loadAndPlay sets audio.src and calls audio.play()
  // Req 1.4: If another track is playing, pause and reset before loading new one
  async loadAndPlay(url: string): Promise<void> {
    if (_isPlaying) {
      audio.pause();
      audio.currentTime = 0;
      _isPlaying = false;
    }

    audio.src = url;

    try {
      await audio.play();
      _isPlaying = true;
    } catch (err: unknown) {
      // Req 1.10: NotAllowedError → set isPlaying=false, emit 'playback-blocked'
      if (err instanceof DOMException && err.name === 'NotAllowedError') {
        _isPlaying = false;
        _emit('playback-blocked');
      } else {
        _isPlaying = false;
        _emit('error');
      }
    }
  },

  pause(): void {
    audio.pause();
    _isPlaying = false;
  },

  resume(): void {
    audio.play().then(() => {
      _isPlaying = true;
    }).catch(() => {
      _isPlaying = false;
      _emit('playback-blocked');
    });
  },

  // Req 1.5: seek sets audio.currentTime
  seek(seconds: number): void {
    audio.currentTime = seconds;
  },

  // Req 1.9: setVolume clamps to [0.0, 1.0]
  setVolume(level: number): void {
    audio.volume = Math.min(1, Math.max(0, level));
  },

  get analyserNode(): AnalyserNode | null {
    return _analyserNode;
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

export { audioEngine, audio as _audioElement };
export type { AudioEngineInterface, AudioEventName };
