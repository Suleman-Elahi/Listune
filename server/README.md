# Listune Backend

Lightweight Go server that keeps secrets and rate-limited API calls off the frontend.

## Prerequisites

- Go 1.22+
- `fpcalc` (Chromaprint) — required for audio fingerprinting

```bash
# Ubuntu/Debian
sudo apt install libchromaprint-tools

# macOS
brew install chromaprint

# Verify
fpcalc -version
```

## Endpoints

| Method | Endpoint | Purpose |
|--------|----------|---------|
| GET | `/health` | Returns `{"status":"ok"}` |
| GET | `/auth/google` | Redirects to Google OAuth consent |
| GET | `/auth/google/callback` | Exchanges code, fetches profile, redirects to frontend |
| POST | `/api/identify` | Accepts audio file upload, fingerprints with fpcalc, queries AcoustID → MusicBrainz |
| POST | `/api/identify-url` | Downloads audio from a URL (e.g. presigned S3), fingerprints it, queries AcoustID → MusicBrainz |

Both identify endpoints accept optional `title`/`artist` hints. When AcoustID has no fingerprint match (common for new or alternate releases), the backend falls back to a MusicBrainz text search using those hints, with duration/version filtering to avoid DJ mixes, live versions, and edits.

### POST /api/identify

Multipart form upload with field `audio` containing the audio file.

```bash
curl -X POST http://localhost:8090/api/identify \
  -F "audio=@song.mp3"
```

Returns:
```json
{
  "title": "Song Title",
  "artist": "Artist Name",
  "album": "Album Name",
  "year": "2020",
  "mbid": "recording-uuid",
  "score": 0.98,
  "fingerprint": "AQAAx...",
  "duration": 245
}
```

## Setup

```bash
cp .env.example .env
# Fill in your API keys

go mod tidy
go run .
```

## Docker

```bash
docker build -t listune-server .
docker run --env-file .env -p 8090:8090 listune-server
```

Note: The Docker image needs `fpcalc` installed. Update the Dockerfile to include it:
```dockerfile
RUN apk add --no-cache ca-certificates chromaprint
```

## Environment Variables

| Variable | Required | Description |
|----------|----------|-------------|
| `PORT` | No | Server port (default: 8090) |
| `GOOGLE_CLIENT_ID` | For auth | Google OAuth client ID |
| `GOOGLE_CLIENT_SECRET` | For auth | Google OAuth client secret |
| `GOOGLE_REDIRECT_URI` | For auth | OAuth callback URL |
| `ACOUSTID_API_KEY` | For identify | AcoustID application API key |
| `FRONTEND_ORIGIN` | No | CORS origin (default: http://localhost:9000) |
| `S3_ALLOWED_HOSTS` | No | Comma-separated hosts allowed for `/api/s3proxy` and S3 tag writes. When empty, only public hosts are allowed. Add local S3 hosts here (e.g. `minio.local:9000`). Link-local/cloud-metadata addresses are always blocked. |
