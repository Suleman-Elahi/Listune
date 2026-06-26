package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	acoustIDLookupURL    = "https://api.acoustid.org/v2/lookup"
	musicBrainzRecording = "https://musicbrainz.org/ws/2/recording/%s?inc=artists+releases+genres&fmt=json"
	coverArtArchive      = "https://coverartarchive.org/release/%s"
	maxUploadSize        = 50 << 20 // 50MB
	userAgent            = "Listune/1.0 (https://github.com/ilfs/listune)"
)

// IdentifyResponse is what we return to the frontend.
type IdentifyResponse struct {
	Title       string   `json:"title,omitempty"`
	Artist      string   `json:"artist,omitempty"`
	Album       string   `json:"album,omitempty"`
	Year        string   `json:"year,omitempty"`
	Genre       string   `json:"genre,omitempty"`
	CoverArtURL string   `json:"coverArtUrl,omitempty"`
	MBID        string   `json:"mbid,omitempty"`
	ReleaseID   string   `json:"releaseId,omitempty"`
	Score       float64  `json:"score,omitempty"`
	Fingerprint string   `json:"fingerprint,omitempty"`
	Duration    int      `json:"duration,omitempty"`
	AllArtists  []string `json:"allArtists,omitempty"`
	Error       string   `json:"error,omitempty"`
}

// IdentifyTrack accepts an audio file upload, fingerprints it with fpcalc,
// then queries AcoustID and MusicBrainz for metadata including cover art.
func IdentifyTrack(w http.ResponseWriter, r *http.Request) {
	apiKey := os.Getenv("ACOUSTID_API_KEY")
	if apiKey == "" {
		writeJSON(w, http.StatusServiceUnavailable, IdentifyResponse{Error: "AcoustID not configured"})
		return
	}

	// Parse multipart form (file upload)
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadSize)
	if err := r.ParseMultipartForm(maxUploadSize); err != nil {
		writeJSON(w, http.StatusBadRequest, IdentifyResponse{Error: "file too large or invalid form"})
		return
	}

	file, header, err := r.FormFile("audio")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, IdentifyResponse{Error: "missing 'audio' file field"})
		return
	}
	defer file.Close()

	// Write to a temp file (fpcalc needs a file path)
	ext := filepath.Ext(header.Filename)
	if ext == "" {
		ext = ".mp3"
	}
	tmpFile, err := os.CreateTemp("", "listune-*"+ext)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, IdentifyResponse{Error: "failed to create temp file"})
		return
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	if _, err := io.Copy(tmpFile, file); err != nil {
		tmpFile.Close()
		writeJSON(w, http.StatusInternalServerError, IdentifyResponse{Error: "failed to write temp file"})
		return
	}
	tmpFile.Close()

	// Run fpcalc to get compressed fingerprint + duration
	fingerprint, duration, err := runFpcalc(tmpPath)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, IdentifyResponse{Error: "fingerprinting failed: " + err.Error()})
		return
	}

	// Query AcoustID → MusicBrainz → Cover Art Archive
	result, err := lookupAcoustID(apiKey, fingerprint, duration)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, IdentifyResponse{Error: "AcoustID lookup failed: " + err.Error()})
		return
	}

	result.Fingerprint = fingerprint
	result.Duration = duration
	writeJSON(w, http.StatusOK, result)
}

// runFpcalc executes fpcalc and returns the compressed fingerprint string and duration.
func runFpcalc(filePath string) (string, int, error) {
	out, err := exec.Command("fpcalc", filePath).Output()
	if err != nil {
		return "", 0, fmt.Errorf("fpcalc execution failed: %w", err)
	}

	var fingerprint string
	var duration int

	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if strings.HasPrefix(line, "DURATION=") {
			val := strings.TrimPrefix(line, "DURATION=")
			duration, _ = strconv.Atoi(val)
		} else if strings.HasPrefix(line, "FINGERPRINT=") {
			fingerprint = strings.TrimPrefix(line, "FINGERPRINT=")
		}
	}

	if fingerprint == "" {
		return "", 0, fmt.Errorf("fpcalc returned no fingerprint")
	}

	return fingerprint, duration, nil
}

func lookupAcoustID(apiKey, fingerprint string, duration int) (*IdentifyResponse, error) {
	// Use POST for large fingerprints; request recordings meta
	data := url.Values{
		"client":      {apiKey},
		"fingerprint": {fingerprint},
		"duration":    {fmt.Sprintf("%d", duration)},
		"meta":        {"recordings"},
	}

	resp, err := http.PostForm(acoustIDLookupURL, data)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var acoustResult struct {
		Results []struct {
			Score      float64 `json:"score"`
			Recordings []struct {
				ID      string `json:"id"`
				Title   string `json:"title"`
				Artists []struct {
					Name string `json:"name"`
				} `json:"artists"`
			} `json:"recordings"`
		} `json:"results"`
	}

	body, _ := io.ReadAll(resp.Body)
	json.Unmarshal(body, &acoustResult)

	if len(acoustResult.Results) == 0 || len(acoustResult.Results[0].Recordings) == 0 {
		return &IdentifyResponse{Error: "no match found"}, nil
	}

	bestResult := acoustResult.Results[0]
	bestRecording := bestResult.Recordings[0]

	result := &IdentifyResponse{
		Title: bestRecording.Title,
		MBID:  bestRecording.ID,
		Score: bestResult.Score,
	}

	// Collect all artists
	for _, a := range bestRecording.Artists {
		result.AllArtists = append(result.AllArtists, a.Name)
	}
	if len(result.AllArtists) > 0 {
		result.Artist = strings.Join(result.AllArtists, ", ")
	}

	// Fetch full recording details from MusicBrainz (includes releases + genres)
	if bestRecording.ID != "" {
		fetchMusicBrainzDetails(bestRecording.ID, result)
	}

	return result, nil
}

func fetchMusicBrainzDetails(recordingID string, result *IdentifyResponse) {
	mbURL := fmt.Sprintf(musicBrainzRecording, recordingID)
	mbReq, _ := http.NewRequest("GET", mbURL, nil)
	mbReq.Header.Set("User-Agent", userAgent)

	mbResp, err := http.DefaultClient.Do(mbReq)
	if err != nil {
		return
	}
	defer mbResp.Body.Close()

	var mbData struct {
		Genres []struct {
			Name string `json:"name"`
		} `json:"genres"`
		Releases []struct {
			ID    string `json:"id"`
			Title string `json:"title"`
			Date  string `json:"date"`
		} `json:"releases"`
	}
	mbBody, _ := io.ReadAll(mbResp.Body)
	json.Unmarshal(mbBody, &mbData)

	// Pick the first release with a date
	for _, release := range mbData.Releases {
		if result.Album == "" {
			result.Album = release.Title
		}
		if result.ReleaseID == "" {
			result.ReleaseID = release.ID
		}
		if len(release.Date) >= 4 && result.Year == "" {
			result.Year = release.Date[:4]
		}
		if result.Album != "" && result.Year != "" && result.ReleaseID != "" {
			break
		}
	}

	// Genre
	if len(mbData.Genres) > 0 {
		genres := make([]string, 0, len(mbData.Genres))
		for _, g := range mbData.Genres {
			genres = append(genres, g.Name)
		}
		result.Genre = strings.Join(genres, ", ")
	}

	// Fetch cover art from Cover Art Archive
	if result.ReleaseID != "" {
		fetchCoverArt(result.ReleaseID, result)
	}
}

func fetchCoverArt(releaseID string, result *IdentifyResponse) {
	caaURL := fmt.Sprintf(coverArtArchive, releaseID)
	req, _ := http.NewRequest("GET", caaURL, nil)
	req.Header.Set("User-Agent", userAgent)

	resp, err := http.DefaultClient.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		// Try a few more releases if the first has no art
		return
	}
	defer resp.Body.Close()

	var caaData struct {
		Images []struct {
			Front      bool   `json:"front"`
			Image      string `json:"image"`
			Thumbnails struct {
				Small string `json:"small"`
				Large string `json:"large"`
				P250  string `json:"250"`
				P500  string `json:"500"`
			} `json:"thumbnails"`
		} `json:"images"`
	}
	caaBody, _ := io.ReadAll(resp.Body)
	json.Unmarshal(caaBody, &caaData)

	// Prefer the front cover, use 500px thumbnail for reasonable size
	for _, img := range caaData.Images {
		if img.Front {
			if img.Thumbnails.P500 != "" {
				result.CoverArtURL = img.Thumbnails.P500
			} else if img.Thumbnails.Large != "" {
				result.CoverArtURL = img.Thumbnails.Large
			} else {
				result.CoverArtURL = img.Image
			}
			return
		}
	}

	// No front cover found, use first image
	if len(caaData.Images) > 0 {
		img := caaData.Images[0]
		if img.Thumbnails.P500 != "" {
			result.CoverArtURL = img.Thumbnails.P500
		} else {
			result.CoverArtURL = img.Image
		}
	}
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
