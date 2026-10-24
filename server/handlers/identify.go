package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	acoustIDLookupURL    = "https://api.acoustid.org/v2/lookup"
	musicBrainzRecording = "https://musicbrainz.org/ws/2/recording/%s?inc=artists+releases+release-groups+genres&fmt=json"
	coverArtArchive      = "https://coverartarchive.org/release/%s"
	maxUploadSize        = 50 << 20  // 50MB
	maxIdentifyDownload  = 200 << 20 // 200MB for server-side S3 downloads
	maxIdentifyBody      = 1 << 20   // 1MB JSON body
	userAgent            = "Listune/1.0 (https://github.com/ilfs/listune)"
)

// identifyHTTPClient is used for external metadata APIs (not S3 downloads).
var identifyHTTPClient = &http.Client{Timeout: 25 * time.Second}

// MusicBrainz asks clients to make at most one request per second.
var (
	musicBrainzMu       sync.Mutex
	lastMusicBrainzCall time.Time
)

func waitForMusicBrainz(ctx context.Context) error {
	musicBrainzMu.Lock()
	defer musicBrainzMu.Unlock()

	if !lastMusicBrainzCall.IsZero() {
		if wait := time.Second - time.Since(lastMusicBrainzCall); wait > 0 {
			timer := time.NewTimer(wait)
			defer timer.Stop()
			select {
			case <-timer.C:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}

	lastMusicBrainzCall = time.Now()
	return nil
}

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

// IdentifyHints are optional title/artist hints used as a MusicBrainz fallback
// when AcoustID has no fingerprint match (common for brand-new releases).
type IdentifyHints struct {
	Title  string
	Artist string
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

	hints := IdentifyHints{
		Title:  strings.TrimSpace(r.FormValue("title")),
		Artist: strings.TrimSpace(r.FormValue("artist")),
	}

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

	identifyFile(w, r.Context(), tmpPath, apiKey, hints)
}

// IdentifyURLRequest is the body for POST /api/identify-url.
type IdentifyURLRequest struct {
	URL      string `json:"url"`
	Filename string `json:"filename,omitempty"`
	Title    string `json:"title,omitempty"`
	Artist   string `json:"artist,omitempty"`
}

// IdentifyURL downloads an audio file from a URL (e.g. a presigned S3 URL),
// then runs the same fingerprinting/lookup pipeline as IdentifyTrack.
func IdentifyURL(w http.ResponseWriter, r *http.Request) {
	apiKey := os.Getenv("ACOUSTID_API_KEY")
	if apiKey == "" {
		writeJSON(w, http.StatusServiceUnavailable, IdentifyResponse{Error: "AcoustID not configured"})
		return
	}

	var req IdentifyURLRequest
	r.Body = http.MaxBytesReader(w, r.Body, maxIdentifyBody)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, IdentifyResponse{Error: "invalid request body"})
		return
	}

	target, err := validateProxyTarget(req.URL)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, IdentifyResponse{Error: "invalid audio URL"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Minute)
	defer cancel()

	downloadReq, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, IdentifyResponse{Error: "invalid audio URL"})
		return
	}

	resp, err := getS3HTTPClient().Do(downloadReq)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, IdentifyResponse{Error: "failed to download audio: " + err.Error()})
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		writeJSON(w, http.StatusBadGateway, IdentifyResponse{Error: fmt.Sprintf("audio download returned %d", resp.StatusCode)})
		return
	}

	ext := safeAudioExt(req.Filename)
	tmpFile, err := os.CreateTemp("", "listune-id-*"+ext)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, IdentifyResponse{Error: "failed to create temp file"})
		return
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	written, err := io.Copy(tmpFile, io.LimitReader(resp.Body, maxIdentifyDownload+1))
	tmpFile.Close()
	if err != nil {
		writeJSON(w, http.StatusBadGateway, IdentifyResponse{Error: "failed to save audio: " + err.Error()})
		return
	}
	if written > maxIdentifyDownload {
		writeJSON(w, http.StatusRequestEntityTooLarge, IdentifyResponse{Error: "audio file is too large"})
		return
	}

	hints := IdentifyHints{
		Title:  strings.TrimSpace(req.Title),
		Artist: strings.TrimSpace(req.Artist),
	}
	identifyFile(w, r.Context(), tmpPath, apiKey, hints)
}

// identifyFile fingerprints a temp file and resolves it via AcoustID/MusicBrainz.
func identifyFile(w http.ResponseWriter, ctx context.Context, tmpPath, apiKey string, hints IdentifyHints) {
	fingerprint, duration, err := runFpcalc(ctx, tmpPath)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, IdentifyResponse{Error: "fingerprinting failed: " + err.Error()})
		return
	}

	result, err := lookupAcoustID(apiKey, fingerprint, duration)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, IdentifyResponse{Error: "AcoustID lookup failed: " + err.Error()})
		return
	}

	// AcoustID only knows fingerprints users have submitted. New or alternate
	// releases are often missing, so fall back to a MusicBrainz text search
	// using the tags/filename the client already has.
	if result.Error == "no match found" && (hints.Title != "" || hints.Artist != "") {
		if fallback, searchErr := lookupMusicBrainzByText(ctx, hints, duration); searchErr == nil && fallback != nil {
			result = fallback
		}
	}

	result.Fingerprint = fingerprint
	result.Duration = duration
	writeJSON(w, http.StatusOK, result)
}

// lookupMusicBrainzByText searches MusicBrainz by recording title/artist.
func lookupMusicBrainzByText(ctx context.Context, hints IdentifyHints, duration int) (*IdentifyResponse, error) {
	title := strings.TrimSpace(hints.Title)
	if title == "" {
		return nil, nil
	}
	artist := strings.TrimSpace(hints.Artist)

	queryParts := []string{fmt.Sprintf(`recording:"%s"`, luceneEscape(title))}
	if artist != "" {
		queryParts = append(queryParts, fmt.Sprintf(`artist:"%s"`, luceneEscape(artist)))
	}

	searchURL := "https://musicbrainz.org/ws/2/recording/?fmt=json&limit=5&query=" +
		url.QueryEscape(strings.Join(queryParts, " AND "))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, searchURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)

	if err := waitForMusicBrainz(ctx); err != nil {
		return nil, err
	}

	resp, err := identifyHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("MusicBrainz search returned %d", resp.StatusCode)
	}

	type mbRecording struct {
		ID             string `json:"id"`
		Title          string `json:"title"`
		Score          int    `json:"score"`
		Length         int    `json:"length"` // milliseconds
		Video          bool   `json:"video"`
		Disambiguation string `json:"disambiguation"`
		Credit         []struct {
			Name string `json:"name"`
		} `json:"artist-credit"`
	}

	var search struct {
		Recordings []mbRecording `json:"recordings"`
	}

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 5<<20))
	if err := json.Unmarshal(body, &search); err != nil {
		return nil, err
	}

	// Title-only matches are riskier, so require a higher confidence.
	minScore := 85
	if artist == "" {
		minScore = 95
	}

	pickBest := func(requireDuration, excludeVideo bool) *mbRecording {
		var best *mbRecording
		bestDelta := math.MaxFloat64
		for i := range search.Recordings {
			candidate := &search.Recordings[i]
			if candidate.Score < minScore {
				continue
			}
			if excludeVideo && candidate.Video {
				continue
			}
			if looksLikeAlternateVersion(candidate.Disambiguation) {
				continue
			}

			delta, durationOK := durationDelta(candidate.Length, duration)
			if requireDuration && !durationOK {
				continue
			}

			if best == nil || candidate.Score > best.Score ||
				(candidate.Score == best.Score && delta < bestDelta) {
				best = candidate
				bestDelta = delta
			}
		}
		return best
	}

	// Prefer the canonical studio recording and a matching duration.
	best := pickBest(true, true)
	if best == nil {
		// Music videos are often minutes longer than the studio track. Prefer
		// the canonical song metadata even when the duration doesn't line up.
		best = pickBest(false, true)
	}
	if best == nil {
		// Last resort: the recording only exists as a video.
		best = pickBest(true, false)
	}
	if best == nil {
		return nil, nil
	}

	result := &IdentifyResponse{
		Title: best.Title,
		MBID:  best.ID,
		Score: float64(best.Score) / 100,
	}
	for _, credit := range best.Credit {
		result.AllArtists = append(result.AllArtists, credit.Name)
	}
	if len(result.AllArtists) > 0 {
		result.Artist = strings.Join(result.AllArtists, ", ")
	}

	fetchMusicBrainzDetails(best.ID, result)
	return result, nil
}

func looksLikeAlternateVersion(disambiguation string) bool {
	value := strings.ToLower(disambiguation)
	for _, marker := range []string{
		"dj-mix", "dj mix", "live", "remix", "instrumental", "karaoke",
		"demo", "edit", "sped up", "slowed", "nightcore", "medley",
		"mixed", "mix", "extended", "atmos", "acoustic", "piano",
		"orchestral", "cover", "rehearsal", "commentary", "bass boosted",
	} {
		if strings.Contains(value, marker) {
			return true
		}
	}
	return false
}

// durationDelta returns how far the MusicBrainz length is from the fingerprint
// duration, and whether it is close enough to be the same recording.
func durationDelta(lengthMs, durationSeconds int) (float64, bool) {
	if lengthMs <= 0 || durationSeconds <= 0 {
		return 0, true
	}
	delta := math.Abs(float64(lengthMs)/1000 - float64(durationSeconds))
	tolerance := math.Max(10, float64(durationSeconds)*0.15)
	return delta, delta <= tolerance
}

func luceneEscape(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	return strings.ReplaceAll(value, `"`, `\"`)
}

func safeAudioExt(filename string) string {
	ext := strings.ToLower(filepath.Ext(filename))
	if len(ext) < 2 || len(ext) > 6 {
		return ".mp3"
	}
	for _, c := range ext[1:] {
		if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9') {
			return ".mp3"
		}
	}
	return ext
}

// runFpcalc executes fpcalc and returns the compressed fingerprint string and duration.
func runFpcalc(ctx context.Context, filePath string) (string, int, error) {
	out, err := exec.CommandContext(ctx, "fpcalc", filePath).Output()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return "", 0, fmt.Errorf("fpcalc timed out")
		}
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			message := strings.TrimSpace(string(exitErr.Stderr))
			if message != "" {
				return "", 0, fmt.Errorf("fpcalc failed: %s", message)
			}
		}
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

	resp, err := identifyHTTPClient.PostForm(acoustIDLookupURL, data)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var acoustResult struct {
		Status string `json:"status"`
		Error  *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
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

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 5<<20))
	json.Unmarshal(body, &acoustResult)

	if acoustResult.Status == "error" && acoustResult.Error != nil {
		return nil, fmt.Errorf("%s", acoustResult.Error.Message)
	}
	if resp.StatusCode != http.StatusOK && acoustResult.Status == "" {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

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

	if err := waitForMusicBrainz(context.Background()); err != nil {
		return
	}

	mbResp, err := identifyHTTPClient.Do(mbReq)
	if err != nil {
		return
	}
	defer mbResp.Body.Close()

	type mbRelease struct {
		ID           string `json:"id"`
		Title        string `json:"title"`
		Date         string `json:"date"`
		Status       string `json:"status"`
		ReleaseGroup struct {
			PrimaryType    string   `json:"primary-type"`
			SecondaryTypes []string `json:"secondary-types"`
		} `json:"release-group"`
	}

	var mbData struct {
		Genres []struct {
			Name string `json:"name"`
		} `json:"genres"`
		Releases []mbRelease `json:"releases"`
	}
	mbBody, _ := io.ReadAll(io.LimitReader(mbResp.Body, 5<<20))
	json.Unmarshal(mbBody, &mbData)

	// Prefer the album release over singles/EPs. MusicBrainz lists the single
	// first for many tracks, which previously made the single title the album.
	releaseRank := func(release mbRelease) int {
		rank := 0
		switch release.ReleaseGroup.PrimaryType {
		case "Album":
			rank += 100
		case "EP":
			rank += 50
		case "Single":
			rank += 10
		case "Broadcast":
			rank += 5
		}
		if release.Status == "Official" {
			rank += 20
		}
		rank -= len(release.ReleaseGroup.SecondaryTypes) * 5
		if len(release.Date) >= 4 {
			rank += 5
		}
		return rank
	}

	var bestRelease *mbRelease
	bestRank := -1
	for i := range mbData.Releases {
		release := &mbData.Releases[i]
		rank := releaseRank(*release)
		if bestRelease == nil || rank > bestRank {
			bestRelease = release
			bestRank = rank
		}
	}

	if bestRelease != nil {
		result.Album = bestRelease.Title
		result.ReleaseID = bestRelease.ID
		if len(bestRelease.Date) >= 4 {
			result.Year = bestRelease.Date[:4]
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

	resp, err := identifyHTTPClient.Do(req)
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
	caaBody, _ := io.ReadAll(io.LimitReader(resp.Body, 5<<20))
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
