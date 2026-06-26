package handlers

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/dhowden/tag"
)

var audioExtensions = map[string]bool{
	".mp3": true, ".flac": true, ".ogg": true,
	".m4a": true, ".wav": true, ".aac": true,
	".wma": true, ".opus": true,
}

// TrackMeta is the metadata returned for each scanned track.
type TrackMeta struct {
	ID         string  `json:"id"`
	Title      string  `json:"title"`
	Artist     string  `json:"artist"`
	Album      string  `json:"album"`
	Duration   float64 `json:"duration"`
	Year       int     `json:"year,omitempty"`
	Genre      string  `json:"genre,omitempty"`
	HasArtwork bool    `json:"hasArtwork"`
	ServerPath string  `json:"serverPath"`
}

// ScanRequest is the body for POST /api/library/scan.
type ScanRequest struct {
	FolderPath string `json:"folderPath"`
}

// ScanResponse contains the list of discovered tracks.
type ScanResponse struct {
	Tracks []TrackMeta `json:"tracks"`
	Total  int         `json:"total"`
	Errors int         `json:"errors"`
	Error  string      `json:"error,omitempty"`
}

// ScanFolder scans a server-side directory for audio files and returns metadata.
func ScanFolder(w http.ResponseWriter, r *http.Request) {
	var req ScanRequest
	body, _ := io.ReadAll(r.Body)
	if err := json.Unmarshal(body, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, ScanResponse{Error: "invalid request body"})
		return
	}

	if req.FolderPath == "" {
		writeJSON(w, http.StatusBadRequest, ScanResponse{Error: "folderPath is required"})
		return
	}

	// Validate path exists and is a directory
	info, err := os.Stat(req.FolderPath)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, ScanResponse{Error: "path not found: " + err.Error()})
		return
	}
	if !info.IsDir() {
		writeJSON(w, http.StatusBadRequest, ScanResponse{Error: "path is not a directory"})
		return
	}

	tracks := make([]TrackMeta, 0, 100)
	errors := 0

	filepath.Walk(req.FolderPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			errors++
			return nil
		}
		if info.IsDir() {
			return nil
		}

		ext := strings.ToLower(filepath.Ext(path))
		if !audioExtensions[ext] {
			return nil
		}

		track := parseTrackFile(path)
		tracks = append(tracks, track)
		return nil
	})

	writeJSON(w, http.StatusOK, ScanResponse{
		Tracks: tracks,
		Total:  len(tracks),
		Errors: errors,
	})
}

func parseTrackFile(path string) TrackMeta {
	// Generate a stable ID from the file path
	hash := sha256.Sum256([]byte(path))
	id := hex.EncodeToString(hash[:16])

	track := TrackMeta{
		ID:         id,
		ServerPath: path,
		Title:      filenameWithoutExt(path),
	}

	f, err := os.Open(path)
	if err != nil {
		return track
	}
	defer f.Close()

	metadata, err := tag.ReadFrom(f)
	if err != nil {
		return track
	}

	if metadata.Title() != "" {
		track.Title = metadata.Title()
	}
	track.Artist = metadata.Artist()
	track.Album = metadata.Album()
	track.Year = metadata.Year()
	track.Genre = metadata.Genre()
	track.HasArtwork = metadata.Picture() != nil

	return track
}

func filenameWithoutExt(path string) string {
	base := filepath.Base(path)
	ext := filepath.Ext(base)
	if ext != "" {
		return base[:len(base)-len(ext)]
	}
	return base
}

// StreamFile serves an audio file by its server path.
// GET /api/stream?path=/path/to/file.mp3
func StreamFile(w http.ResponseWriter, r *http.Request) {
	filePath := r.URL.Query().Get("path")
	if filePath == "" {
		http.Error(w, "missing path parameter", http.StatusBadRequest)
		return
	}

	// Security: only serve audio files
	ext := strings.ToLower(filepath.Ext(filePath))
	if !audioExtensions[ext] {
		http.Error(w, "not an audio file", http.StatusForbidden)
		return
	}

	// Check file exists
	if _, err := os.Stat(filePath); err != nil {
		http.Error(w, "file not found", http.StatusNotFound)
		return
	}

	// Determine content type
	contentType := "application/octet-stream"
	switch ext {
	case ".mp3":
		contentType = "audio/mpeg"
	case ".flac":
		contentType = "audio/flac"
	case ".ogg", ".opus":
		contentType = "audio/ogg"
	case ".m4a", ".aac":
		contentType = "audio/mp4"
	case ".wav":
		contentType = "audio/wav"
	}

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Accept-Ranges", "bytes")

	http.ServeFile(w, r, filePath)
}

// GetArtwork serves the embedded artwork from an audio file.
// GET /api/artwork?path=/path/to/file.mp3
func GetArtwork(w http.ResponseWriter, r *http.Request) {
	filePath := r.URL.Query().Get("path")
	if filePath == "" {
		http.Error(w, "missing path parameter", http.StatusBadRequest)
		return
	}

	f, err := os.Open(filePath)
	if err != nil {
		http.Error(w, "file not found", http.StatusNotFound)
		return
	}
	defer f.Close()

	metadata, err := tag.ReadFrom(f)
	if err != nil {
		http.Error(w, "cannot read tags", http.StatusInternalServerError)
		return
	}

	pic := metadata.Picture()
	if pic == nil {
		http.Error(w, "no artwork", http.StatusNotFound)
		return
	}

	mimeType := pic.MIMEType
	if mimeType == "" {
		mimeType = "image/jpeg"
	}

	w.Header().Set("Content-Type", mimeType)
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(pic.Data)))
	w.Write(pic.Data)
}
