package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	ytdlp "github.com/lrstanley/go-ytdlp"
)

// YouTube search is a paid-in-quota API (~100 units per call), so results are
// cached server-side and downloads may only reference videos from a cached
// search. That also keeps arbitrary URLs out of yt-dlp.
const (
	youtubeSearchTTL = 15 * time.Minute
	youtubeAPIBaseV3 = "https://www.googleapis.com/youtube/v3"
)

var (
	youtubeAPIBase = youtubeAPIBaseV3
	youtubeIDRe    = regexp.MustCompile(`^[A-Za-z0-9_-]{11}$`)
)

type youtubeVideo struct {
	VideoID      string `json:"videoId"`
	Title        string `json:"title"`
	ChannelTitle string `json:"channelTitle"`
	Description  string `json:"description,omitempty"`
	PublishedAt  string `json:"publishedAt,omitempty"`
	ThumbnailURL string `json:"thumbnailUrl,omitempty"`
}

type youtubeSearchEntry struct {
	query   string
	videos  []youtubeVideo
	created time.Time
}

var (
	youtubeSearchMu sync.Mutex
	youtubeSearches = map[string]youtubeSearchEntry{}
)

func storeYouTubeSearch(query string, videos []youtubeVideo) string {
	youtubeSearchMu.Lock()
	defer youtubeSearchMu.Unlock()
	for id, e := range youtubeSearches {
		if time.Since(e.created) > youtubeSearchTTL {
			delete(youtubeSearches, id)
		}
	}
	id := strconv.FormatInt(time.Now().UnixNano(), 36)
	youtubeSearches[id] = youtubeSearchEntry{query: query, videos: videos, created: time.Now()}
	return id
}

// cachedYouTubeSearch avoids spending YouTube quota on the same query twice
// in a short window. Paged requests are never deduplicated.
func cachedYouTubeSearch(query string) (string, []youtubeVideo, bool) {
	youtubeSearchMu.Lock()
	defer youtubeSearchMu.Unlock()
	for id, e := range youtubeSearches {
		if time.Since(e.created) > youtubeSearchTTL {
			delete(youtubeSearches, id)
			continue
		}
		if e.query == query && time.Since(e.created) < 30*time.Second {
			return id, e.videos, true
		}
	}
	return "", nil, false
}

func getYouTubeSearch(id string) (youtubeSearchEntry, bool) {
	youtubeSearchMu.Lock()
	defer youtubeSearchMu.Unlock()
	e, ok := youtubeSearches[id]
	if ok && time.Since(e.created) > youtubeSearchTTL {
		delete(youtubeSearches, id)
		return youtubeSearchEntry{}, false
	}
	return e, ok
}

// youtubeToken resolves the same opaque Google session handle used by Drive.
func youtubeToken(r *http.Request) (string, error) {
	raw := r.URL.Query().Get("token")
	if raw == "" {
		raw = extractBearer(r.Header.Get("Authorization"))
	}
	return resolveAccessToken(r.Context(), raw)
}

// YouTubeStatus reports whether the caller has a usable Google session.
// GET /api/youtube/status
func YouTubeStatus(w http.ResponseWriter, r *http.Request) {
	if _, err := youtubeToken(r); err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"available": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"available": true})
}

// YouTubeSearch calls search.list with the signed-in user's access token.
// POST /api/youtube/search
func YouTubeSearch(w http.ResponseWriter, r *http.Request) {
	token, err := youtubeToken(r)
	if err != nil {
		writeDriveError(w, err)
		return
	}

	var req struct {
		Query      string `json:"query"`
		PageToken  string `json:"pageToken"`
		MaxResults int    `json:"maxResults"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid search request")
		return
	}
	q := strings.TrimSpace(req.Query)
	if q == "" {
		writeJSONError(w, http.StatusBadRequest, "query is required")
		return
	}
	maxResults := req.MaxResults
	if maxResults <= 0 || maxResults > 50 {
		maxResults = 10
	}

	if req.PageToken == "" {
		if id, videos, ok := cachedYouTubeSearch(q); ok {
			writeJSON(w, http.StatusOK, map[string]any{
				"searchId": id,
				"query":    q,
				"results":  videos,
			})
			return
		}
	}

	params := url.Values{
		"part":       {"snippet"},
		"type":       {"video"},
		"q":          {q},
		"maxResults": {strconv.Itoa(maxResults)},
		"order":      {"relevance"},
	}
	if req.PageToken != "" {
		params.Set("pageToken", req.PageToken)
	}

	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()

	apiURL := youtubeAPIBase + "/search?" + params.Encode()
	apiReq, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		writeJSONError(w, http.StatusBadGateway, "could not build youtube request")
		return
	}
	apiReq.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(apiReq)
	if err != nil {
		writeJSONError(w, http.StatusBadGateway, "youtube request failed")
		return
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		writeJSONError(w, http.StatusBadGateway, "could not read youtube response")
		return
	}
	if resp.StatusCode == http.StatusUnauthorized {
		writeDriveError(w, errGoogleUnauthorized)
		return
	}
	if resp.StatusCode != http.StatusOK {
		msg := fmt.Sprintf("youtube api returned %d", resp.StatusCode)
		var apiErr struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if json.Unmarshal(body, &apiErr) == nil && apiErr.Error.Message != "" {
			msg = fmt.Sprintf("%s: %s", msg, apiErr.Error.Message)
		}
		writeJSONError(w, http.StatusBadGateway, msg)
		return
	}

	var apiResp struct {
		NextPageToken string `json:"nextPageToken"`
		PrevPageToken string `json:"prevPageToken"`
		Items         []struct {
			ID struct {
				Kind    string `json:"kind"`
				VideoID string `json:"videoId"`
			} `json:"id"`
			Snippet struct {
				Title         string `json:"title"`
				Description   string `json:"description"`
				ChannelTitle  string `json:"channelTitle"`
				PublishedAt   string `json:"publishedAt"`
				LiveBroadcast string `json:"liveBroadcastContent"`
				Thumbnails    map[string]struct {
					URL string `json:"url"`
				} `json:"thumbnails"`
			} `json:"snippet"`
		} `json:"items"`
	}
	if err := json.Unmarshal(body, &apiResp); err != nil {
		writeJSONError(w, http.StatusBadGateway, "invalid youtube response")
		return
	}

	videos := make([]youtubeVideo, 0, len(apiResp.Items))
	for _, item := range apiResp.Items {
		id := item.ID.VideoID
		if !youtubeIDRe.MatchString(id) || item.Snippet.LiveBroadcast == "live" {
			continue
		}
		thumb := item.Snippet.Thumbnails["medium"].URL
		if thumb == "" {
			thumb = item.Snippet.Thumbnails["default"].URL
		}
		videos = append(videos, youtubeVideo{
			VideoID:      id,
			Title:        item.Snippet.Title,
			ChannelTitle: item.Snippet.ChannelTitle,
			Description:  item.Snippet.Description,
			PublishedAt:  item.Snippet.PublishedAt,
			ThumbnailURL: thumb,
		})
	}

	searchID := storeYouTubeSearch(q, videos)
	writeJSON(w, http.StatusOK, map[string]any{
		"searchId":      searchID,
		"query":         q,
		"results":       videos,
		"nextPageToken": apiResp.NextPageToken,
		"prevPageToken": apiResp.PrevPageToken,
	})
}

// --- download jobs ---------------------------------------------------------

type youtubeJobFile struct {
	Name     string `json:"name"`
	VideoID  string `json:"videoId"`
	Path     string `json:"path,omitempty"`
	Status   string `json:"status"` // pending | downloading | done | failed
	Progress int    `json:"progress,omitempty"`
	Error    string `json:"error,omitempty"`
}

type youtubeJob struct {
	ID          string           `json:"id"`
	Query       string           `json:"query,omitempty"`
	Status      string           `json:"status"` // running | done | failed | cancelled
	Files       []*youtubeJobFile `json:"files"`
	Downloaded  int              `json:"downloaded"`
	Failed      int              `json:"failed"`
	Destination string           `json:"destination"`
	Created     time.Time        `json:"created"`

	mu     sync.Mutex
	cancel context.CancelFunc
}

func (j *youtubeJob) snapshot() youtubeJob {
	j.mu.Lock()
	defer j.mu.Unlock()
	files := make([]*youtubeJobFile, 0, len(j.Files))
	for _, f := range j.Files {
		copy := *f
		files = append(files, &copy)
	}
	return youtubeJob{
		ID:          j.ID,
		Query:       j.Query,
		Status:      j.Status,
		Files:       files,
		Downloaded:  j.Downloaded,
		Failed:      j.Failed,
		Destination: j.Destination,
		Created:     j.Created,
	}
}

var (
	youtubeJobsMu sync.Mutex
	youtubeJobs   = map[string]*youtubeJob{}
)

func addYouTubeJob(j *youtubeJob) {
	youtubeJobsMu.Lock()
	defer youtubeJobsMu.Unlock()
	youtubeJobs[j.ID] = j
}

func findYouTubeJob(id string) (*youtubeJob, bool) {
	youtubeJobsMu.Lock()
	defer youtubeJobsMu.Unlock()
	j, ok := youtubeJobs[id]
	return j, ok
}

func listYouTubeJobs() []youtubeJob {
	youtubeJobsMu.Lock()
	defer youtubeJobsMu.Unlock()
	out := make([]youtubeJob, 0, len(youtubeJobs))
	for _, j := range youtubeJobs {
		out = append(out, j.snapshot())
	}
	return out
}

// YouTubeDownload converts selected cached videos to MP3 into the library.
// POST /api/youtube/download
func YouTubeDownload(w http.ResponseWriter, r *http.Request) {
	if _, err := youtubeToken(r); err != nil {
		writeDriveError(w, err)
		return
	}

	var req struct {
		SearchID string `json:"searchId"`
		Videos   []struct {
			VideoID string `json:"videoId"`
		} `json:"videos"`
		Quality string `json:"quality"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.SearchID == "" {
		writeJSONError(w, http.StatusBadRequest, "searchId is required")
		return
	}
	entry, ok := getYouTubeSearch(req.SearchID)
	if !ok {
		writeJSONError(w, http.StatusNotFound, "search expired, search again")
		return
	}

	quality := req.Quality
	switch quality {
	case "128", "192", "256", "320":
	default:
		quality = "192"
	}

	byID := make(map[string]youtubeVideo, len(entry.videos))
	for _, v := range entry.videos {
		byID[v.VideoID] = v
	}
	selected := make([]youtubeVideo, 0, len(req.Videos))
	seen := map[string]bool{}
	for _, sel := range req.Videos {
		v, found := byID[sel.VideoID]
		if !found || seen[v.VideoID] {
			continue
		}
		seen[v.VideoID] = true
		selected = append(selected, v)
	}
	if len(selected) == 0 {
		writeJSONError(w, http.StatusBadRequest, "nothing selected")
		return
	}

	dest := filepath.Join(LibraryRoot(), "YouTube")
	if err := os.MkdirAll(dest, 0o755); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "could not prepare download folder")
		return
	}

	files := make([]*youtubeJobFile, 0, len(selected))
	for _, v := range selected {
		files = append(files, &youtubeJobFile{
			Name:    v.Title,
			VideoID: v.VideoID,
			Status:  "pending",
		})
	}
	job := &youtubeJob{
		ID:          strconv.FormatInt(time.Now().UnixNano(), 36),
		Query:       entry.query,
		Status:      "running",
		Files:       files,
		Destination: dest,
		Created:     time.Now(),
	}
	addYouTubeJob(job)
	go runYouTubeJob(job, dest, selected, quality)

	writeJSON(w, http.StatusOK, map[string]any{"jobId": job.ID, "files": len(selected)})
}

// runYouTubeDownload is a var so tests can substitute a stub instead of
// invoking the real yt-dlp binary.
var runYouTubeDownload = downloadYouTubeAudio

func downloadYouTubeAudio(ctx context.Context, dest string, video youtubeVideo, quality string, onProgress func(int)) error {
	exe := os.Getenv("YTDLP_BINARY")
	if exe == "" {
		installed, err := ytdlp.Install(ctx, nil)
		if err != nil {
			return fmt.Errorf("resolve yt-dlp: %w", err)
		}
		exe = installed.Executable
	}

	ff, err := ytdlp.InstallFFmpeg(ctx, nil)
	if err != nil {
		return fmt.Errorf("resolve ffmpeg: %w", err)
	}
	_, _ = ytdlp.InstallFFprobe(ctx, nil)

	videoCtx, cancel := context.WithTimeout(ctx, 15*time.Minute)
	defer cancel()

	cmd := ytdlp.New().
		SetExecutable(exe).
		FFmpegLocation(filepath.Dir(ff.Executable)).
		ExtractAudio().
		AudioFormat("mp3").
		AudioQuality(quality + "K").
		NoPlaylist().
		EmbedMetadata().
		Output(filepath.Join(dest, "%(id)s - %(title)s.%(ext)s"))
	if nodePath, err := exec.LookPath("node"); err == nil && nodePath != "" {
		cmd = cmd.JsRuntimes("node:" + nodePath)
	}
	if onProgress != nil {
		cmd = cmd.ProgressFunc(500*time.Millisecond, func(u ytdlp.ProgressUpdate) {
			pct := u.Percent()
			if pct >= 0 && pct <= 100 {
				onProgress(int(pct))
			}
		})
	}

	watchURL := "https://www.youtube.com/watch?" + url.Values{"v": {video.VideoID}}.Encode()
	if _, err := cmd.Run(videoCtx, watchURL); err != nil {
		return fmt.Errorf("download %s: %w", video.VideoID, err)
	}
	return nil
}

func runYouTubeJob(job *youtubeJob, dest string, videos []youtubeVideo, quality string) {
	ctx, cancel := context.WithCancel(context.Background())
	job.mu.Lock()
	job.cancel = cancel
	job.mu.Unlock()
	defer cancel()

	for i, video := range videos {
		if ctx.Err() != nil {
			break
		}
		job.mu.Lock()
		if i < len(job.Files) {
			job.Files[i].Status = "downloading"
		}
		job.mu.Unlock()

		onProgress := func(pct int) {
			job.mu.Lock()
			defer job.mu.Unlock()
			if i < len(job.Files) {
				job.Files[i].Progress = pct
			}
		}
		err := runYouTubeDownload(ctx, dest, video, quality, onProgress)

		job.mu.Lock()
		if i < len(job.Files) {
			if err != nil {
				job.Files[i].Status = "failed"
				job.Files[i].Error = err.Error()
				job.Failed++
			} else {
				job.Files[i].Status = "done"
				job.Files[i].Progress = 100
				job.Downloaded++
			}
		}
		job.mu.Unlock()
	}

	job.mu.Lock()
	defer job.mu.Unlock()
	switch {
	case ctx.Err() != nil:
		job.Status = "cancelled"
	case job.Failed > 0 && job.Downloaded == 0:
		job.Status = "failed"
	default:
		job.Status = "done"
	}
}

// YouTubeJobs lists download jobs.
// GET /api/youtube/jobs
func YouTubeJobs(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"jobs": listYouTubeJobs()})
}

// YouTubeJob returns one download job.
// GET /api/youtube/jobs/{id}
func YouTubeJob(w http.ResponseWriter, r *http.Request) {
	job, ok := findYouTubeJob(r.PathValue("id"))
	if !ok {
		writeJSONError(w, http.StatusNotFound, "job not found")
		return
	}
	writeJSON(w, http.StatusOK, job.snapshot())
}

// YouTubeCancelJob cancels a running download job.
// POST /api/youtube/jobs/{id}/cancel
func YouTubeCancelJob(w http.ResponseWriter, r *http.Request) {
	job, ok := findYouTubeJob(r.PathValue("id"))
	if !ok {
		writeJSONError(w, http.StatusNotFound, "job not found")
		return
	}
	job.mu.Lock()
	if job.cancel != nil {
		job.cancel()
	}
	if job.Status == "running" {
		job.Status = "cancelled"
	}
	job.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}
