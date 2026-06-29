# Listune

![Screenshot](screen.png)

A dark-themed, mobile-first music player PWA built with Vue 3, Quasar, and TypeScript. Plays local files and S3-hosted audio with a real-time visualizer, playlist management, audio fingerprinting, and Google account sync.

## Features

### Playback
- Plays MP3, FLAC, OGG, M4A, WAV, AAC files
- Play, pause, seek, previous, next controls
- Shuffle and loop toggle
- Volume control with keyboard shortcuts
- Auto-advances through queue (random pick in shuffle mode)
- Media Session API integration (lock screen controls, artwork, position state)

### Library
- Add local folders via the File System Access API (no upload needed)
- Add S3 buckets as audio sources (AWS SigV4 pre-signed URLs)
- Background scanner worker reads ID3/Vorbis/MP4 tags (title, artist, album, artwork, duration)
- Falls back to filename when tags are missing
- Search and filter tracks in real time
- Virtual scroll for large libraries
- Artwork thumbnails loaded lazily
- Add tracks to playlists directly from the library

### Audio Fingerprinting (AcoustID + MusicBrainz)
- Automatically identifies untagged tracks via audio fingerprinting
- Uses `fpcalc` (Chromaprint) on the backend to generate fingerprints
- Queries AcoustID for recording matches, then MusicBrainz for full metadata (title, artist, album, year)
- One-click "ID untagged tracks" button in the library

### Visualizer
- Real-time frequency bar visualizer using Web Audio API
- XP-style thick lime-green gradient bars with peak-decay dots (fftSize=64)
- Offscreen canvas rendering via a dedicated Web Worker (auto-offloads when FPS drops below 30)
- Displayed inside a glowing green card with corner radial highlights

### Now Playing View
- Full-screen dark player on black background
- Visualizer card with glowing green border
- Track title and artist display
- Green glowing seek bar with draggable thumb
- Metallic transport buttons (shuffle, previous, play/pause, next, loop)
- Volume slider

### Library View
- Dark UI with search bar and All Songs / Artists / Albums filter tabs
- Numbered track list with artwork, title, artist, duration
- Import button to add new sources
- User avatar with logout menu
- Mini player bar at the bottom when a track is loaded

### Playlists
- Create and name playlists
- Save current queue as a playlist from the drawer panel
- Sort by Recently Added, Alphabetical, or By Artist
- 2-column grid of playlist cards
- Bottom sheet to browse and play tracks from a playlist
- Add tracks to playlists from the library context menu
- Persisted to localStorage

### Keyboard Shortcuts
| Key | Action |
|-----|--------|
| Space | Play / Pause |
| → | Seek forward 5s |
| ← | Seek back 5s |
| ↑ | Volume up |
| ↓ | Volume down |
| N | Next track |
| P | Previous track |
| S | Toggle shuffle |
| L | Toggle loop |

### Authentication
- Google SSO login (OAuth 2.0 via backend)
- User avatar and logout in the library header
- Route guard redirects unauthenticated users to login

### Navigation
- Bottom navigation bar: Home, Library, Search, Playlists
- Mini player strip in the bottom nav (artwork, title, prev/play/next) visible on all pages except the player

### PWA
- Service worker caches app shell for offline support
- Audio streams excluded from cache to prevent memory bloat
- Runtime caching for fonts, network-only for presigned URLs

## Tech Stack

- Vue 3 + `<script setup>` + TypeScript
- Quasar Framework v2 (SPA + PWA)
- Pinia for state management
- idb (IndexedDB) for track and artwork storage
- music-metadata for tag parsing (browser-compatible, v11+)
- Web Audio API + OffscreenCanvas Web Worker for the visualizer
- File System Access API for local folder scanning
- AWS SigV4 for S3 pre-signed URL generation (no SDK dependency)
- Go backend for OAuth, AcoustID fingerprinting, and MusicBrainz lookups

## Prerequisites

- Node.js 22+
- Go 1.22+ (for the backend server)
- `fpcalc` (Chromaprint) — required for audio fingerprinting

```bash
# Ubuntu/Debian
sudo apt install libchromaprint-tools

# macOS
brew install chromaprint

# Verify installation
fpcalc -version
```

## Getting Started

### Frontend

```bash
npm install
quasar dev
```

### Backend

```bash
cd server
cp .env.example .env
# Fill in GOOGLE_CLIENT_ID, GOOGLE_CLIENT_SECRET, ACOUSTID_API_KEY
go run .
```

The backend runs on `:8090` by default. The frontend expects it at `http://localhost:8090` (configurable via `VITE_BACKEND_URL` env var).

### Build for Production

```bash
quasar build
cd server && go build -o listune-server .
```

## Planned Features

- [x] ~~Spotify integration~~ (removed — Spotify Web API now requires Premium)
- [x] MusicBrainz + AcoustID — audio fingerprinting to auto-identify untagged tracks
- [x] Google SSO — sign in with Google
- [x] Ultra lightweight build — removed axios, Roboto font, Vue Options API; system font stack, proper PWA caching
- [x] Artist and album grouping views in the library

## License

See [LICENSE](LICENSE).
