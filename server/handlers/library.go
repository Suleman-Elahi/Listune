package handlers

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
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

// LibraryRoot returns the absolute path of the local library folder
// configured via LIBRARY_PATH (default ./data/library). The folder is
// created when missing. All scans, streaming, artwork and tag writes are
// confined to this folder.
func LibraryRoot() string {
	root := os.Getenv("LIBRARY_PATH")
	if root == "" {
		root = filepath.Join("data", "library")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return root
	}
	_ = os.MkdirAll(abs, 0o755)
	return abs
}

// resolveLibraryPath maps a client-supplied path (relative to the library
// root, or absolute inside it) to a clean absolute path inside the library
// root. Paths that escape the root are rejected.
func resolveLibraryPath(p string) (string, error) {
	root := LibraryRoot()
	var abs string
	if filepath.IsAbs(p) {
		abs = filepath.Clean(p)
	} else {
		abs = filepath.Clean(filepath.Join(root, p))
	}
	if abs != root && !strings.HasPrefix(abs, root+string(filepath.Separator)) {
		return "", fmt.Errorf("path outside library")
	}
	return abs, nil
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

// ScanResponse contains the list of discovered tracks.
type ScanResponse struct {
	Tracks []TrackMeta `json:"tracks"`
	Total  int         `json:"total"`
	Errors int         `json:"errors"`
	Error  string      `json:"error,omitempty"`
}

// ScanFolder scans the configured library folder (including subfolders)
// for audio files and returns metadata. ServerPath values are relative to
// the library root.
func ScanFolder(w http.ResponseWriter, r *http.Request) {
	root := LibraryRoot()

	info, err := os.Stat(root)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, ScanResponse{Error: "library folder not available: " + err.Error()})
		return
	}
	if !info.IsDir() {
		writeJSON(w, http.StatusInternalServerError, ScanResponse{Error: "library path is not a directory"})
		return
	}

	tracks := make([]TrackMeta, 0, 100)
	errors := 0

	filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
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

		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			errors++
			return nil
		}

		track := parseTrackFile(path, filepath.ToSlash(rel))
		tracks = append(tracks, track)
		return nil
	})

	writeJSON(w, http.StatusOK, ScanResponse{
		Tracks: tracks,
		Total:  len(tracks),
		Errors: errors,
	})
}

func parseTrackFile(absPath, relPath string) TrackMeta {
	// Stable ID from the path relative to the library root
	hash := sha256.Sum256([]byte(relPath))
	id := hex.EncodeToString(hash[:16])

	track := TrackMeta{
		ID:         id,
		ServerPath: relPath,
		Title:      filenameWithoutExt(absPath),
	}

	f, err := os.Open(absPath)
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

// StreamFile serves an audio file from the library folder.
// GET /api/stream?path=relative/or/absolute/path.mp3
func StreamFile(w http.ResponseWriter, r *http.Request) {
	filePath := r.URL.Query().Get("path")
	if filePath == "" {
		http.Error(w, "missing path parameter", http.StatusBadRequest)
		return
	}

	resolved, err := resolveLibraryPath(filePath)
	if err != nil {
		http.Error(w, "path outside library", http.StatusForbidden)
		return
	}

	// Security: only serve audio files
	ext := strings.ToLower(filepath.Ext(resolved))
	if !audioExtensions[ext] {
		http.Error(w, "not an audio file", http.StatusForbidden)
		return
	}

	// Check file exists
	if _, err := os.Stat(resolved); err != nil {
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

	http.ServeFile(w, r, resolved)
}

// GetArtwork serves the embedded artwork from an audio file in the library.
// GET /api/artwork?path=relative/or/absolute/path.mp3
func GetArtwork(w http.ResponseWriter, r *http.Request) {
	filePath := r.URL.Query().Get("path")
	if filePath == "" {
		http.Error(w, "missing path parameter", http.StatusBadRequest)
		return
	}

	resolved, err := resolveLibraryPath(filePath)
	if err != nil {
		http.Error(w, "path outside library", http.StatusForbidden)
		return
	}

	f, err := os.Open(resolved)
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
