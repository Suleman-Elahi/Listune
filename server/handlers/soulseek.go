package handlers

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

const searchTTL = 15 * time.Minute

// --- bridge process (long-lived `serve` session) ----------------------------

// A single slsk connection is shared by search and download: peers are only
// downloadable in the process that discovered them via search. The Go side
// therefore keeps one `node bridge.mjs serve` daemon and talks NDJSON over
// its stdin/stdout, routing events by request id.

func soulseekNode() string {
	if n := os.Getenv("SOULSEEK_NODE"); n != "" {
		return n
	}
	return "node"
}

func soulseekBridgePath() string {
	if p := os.Getenv("SOULSEEK_BRIDGE"); p != "" {
		return p
	}
	return filepath.Join("soulseek-bridge", "bridge.mjs")
}

// bridgeEvent is one NDJSON event line emitted by the bridge.
type bridgeEvent struct {
	Type       string          `json:"type"`
	ID         string          `json:"id"`
	Message    string          `json:"message,omitempty"`
	Account    string          `json:"account,omitempty"`
	LoggedIn   *bool           `json:"loggedIn,omitempty"`
	Count      int             `json:"count,omitempty"`
	Downloaded int             `json:"downloaded,omitempty"`
	Failed     int             `json:"failed,omitempty"`
	Name       string          `json:"name,omitempty"`
	Path       string          `json:"path,omitempty"`

	// Line is the complete original event, for callers that need fields
	// beyond the typed subset above.
	Line json.RawMessage `json:"-"`
}

// terminalEvent reports whether an event completes a request.
func (e bridgeEvent) terminal() bool {
	switch e.Type {
	case "done", "ok", "error", "status":
		return true
	}
	return false
}

// bridgeProc owns the daemon process and routes request/response events.
type bridgeProc struct {
	mu      sync.Mutex
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	pending map[string]chan bridgeEvent
	nextID  int
}

var defaultBridge = &bridgeProc{pending: map[string]chan bridgeEvent{}}

// ensureLocked starts the daemon when it is not running. Caller holds mu.
func (b *bridgeProc) ensureLocked() error {
	if b.cmd != nil && b.cmd.Process != nil {
		return nil
	}

	cmd := exec.Command(soulseekNode(), soulseekBridgePath(), "serve")
	cmd.Env = os.Environ()
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return err
	}

	b.cmd = cmd
	b.stdin = stdin
	go b.readLoop(stdout)
	go func() {
		_ = cmd.Wait()
		b.dropPending("bridge process exited")
	}()
	return nil
}

// readLoop dispatches stdout events to the waiting request by id.
func (b *bridgeProc) readLoop(stdout io.Reader) {
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 256*1024), 2*1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		var ev bridgeEvent
		if err := json.Unmarshal(line, &ev); err != nil {
			continue
		}
		ev.Line = json.RawMessage(append([]byte(nil), line...))

		b.mu.Lock()
		ch := b.pending[ev.ID]
		b.mu.Unlock()
		if ch != nil {
			ch <- ev
			if ev.terminal() {
				close(ch)
				b.mu.Lock()
				delete(b.pending, ev.ID)
				b.mu.Unlock()
			}
		}
	}
}

func (b *bridgeProc) dropPending(message string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for id, ch := range b.pending {
		ch <- bridgeEvent{Type: "error", Message: message}
		close(ch)
		delete(b.pending, id)
	}
	b.cmd = nil
	b.stdin = nil
}

// call sends one command and collects its events until the terminal event.
// onEvent, when non-nil, observes every non-terminal event as it arrives.
func (b *bridgeProc) call(ctx context.Context, req map[string]any, onEvent func(bridgeEvent)) ([]bridgeEvent, error) {
	b.mu.Lock()
	if err := b.ensureLocked(); err != nil {
		b.mu.Unlock()
		return nil, err
	}
	b.nextID++
	id := "r" + strconv.Itoa(b.nextID)
	req["id"] = id
	ch := make(chan bridgeEvent, 64)
	b.pending[id] = ch
	payload, _ := json.Marshal(req)
	_, err := b.stdin.Write(append(payload, '\n'))
	b.mu.Unlock()
	if err != nil {
		b.dropPending("bridge stdin closed")
		return nil, fmt.Errorf("bridge write failed: %w", err)
	}

	var events []bridgeEvent
	for {
		select {
		case <-ctx.Done():
			b.mu.Lock()
			delete(b.pending, id)
			b.mu.Unlock()
			// Drain whatever is already buffered without blocking.
			for {
				select {
				case ev, ok := <-ch:
					if !ok {
						return events, ctx.Err()
					}
					events = append(events, ev)
				default:
					return events, ctx.Err()
				}
			}
		case ev, ok := <-ch:
			if !ok {
				return events, nil
			}
			if ev.Type == "error" && ev.terminal() {
				events = append(events, ev)
				return events, fmt.Errorf("%s", ev.Message)
			}
			if ev.terminal() {
				events = append(events, ev)
				return events, nil
			}
			events = append(events, ev)
			if onEvent != nil {
				onEvent(ev)
			}
		}
	}
}

// --- search results cache ---------------------------------------------------

// slskFile is the raw slsk-client search hit, echoed to the bridge for download.
type slskFile struct {
	User    string `json:"user"`
	File    string `json:"file"`
	Size    int64  `json:"size"`
	Slots   bool   `json:"slots"`
	Bitrate int    `json:"bitrate"`
	Speed   int64  `json:"speed"`
}

// slskResultFile is one downloadable file inside a result folder.
type slskResultFile struct {
	FileIndex int    `json:"fileIndex"`
	Name      string `json:"name"`
	Size      int64  `json:"size"`
	Bitrate   *int   `json:"bitrate"`
}

// slskResult is one peer folder offered by a search.
type slskResult struct {
	ResultIndex int              `json:"resultIndex"`
	User        string           `json:"user"`
	Folder      string           `json:"folder"`
	Files       []slskResultFile `json:"files"`
	AvgBitrate  int              `json:"avgBitrate"`
	SizeMb      int              `json:"sizeMb"`
	SpeedKbs    int              `json:"speedKbs"`
	Slots       bool             `json:"slots"`
	Raw         []slskFile       `json:"-"`
}

type searchEntry struct {
	query   string
	results []slskResult
	created time.Time
}

var (
	searchMu sync.Mutex
	searches = map[string]searchEntry{}
)

func storeSearch(query string, results []slskResult) string {
	searchMu.Lock()
	defer searchMu.Unlock()
	for id, e := range searches {
		if time.Since(e.created) > searchTTL {
			delete(searches, id)
		}
	}
	id := strconv.FormatInt(time.Now().UnixNano(), 36)
	searches[id] = searchEntry{query: query, results: results, created: time.Now()}
	return id
}

func getSearch(id string) (searchEntry, bool) {
	searchMu.Lock()
	defer searchMu.Unlock()
	e, ok := searches[id]
	if ok && time.Since(e.created) > searchTTL {
		delete(searches, id)
		return searchEntry{}, false
	}
	return e, ok
}

// --- download jobs ----------------------------------------------------------

// jobFile tracks one file inside a download job.
type jobFile struct {
	Name   string `json:"name"`
	Path   string `json:"path,omitempty"`
	Status string `json:"status"` // pending | downloading | done | skipped | failed
	Error  string `json:"error,omitempty"`
}

// downloadJob is one bridge download run.
type downloadJob struct {
	ID          string     `json:"id"`
	Query       string     `json:"query,omitempty"`
	Status      string     `json:"status"` // running | done | failed | cancelled
	Files       []*jobFile `json:"files"`
	Downloaded  int        `json:"downloaded"`
	Failed      int        `json:"failed"`
	Destination string     `json:"destination"`
	Created     time.Time  `json:"created"`

	mu     sync.Mutex
	cancel context.CancelFunc
}

func (j *downloadJob) snapshot() downloadJob {
	j.mu.Lock()
	defer j.mu.Unlock()
	files := make([]*jobFile, len(j.Files))
	copy(files, j.Files)
	return downloadJob{
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
	jobsMu sync.Mutex
	jobs   = map[string]*downloadJob{}
)

func addJob(j *downloadJob) {
	jobsMu.Lock()
	defer jobsMu.Unlock()
	jobs[j.ID] = j
}

func findJob(id string) (*downloadJob, bool) {
	jobsMu.Lock()
	defer jobsMu.Unlock()
	j, ok := jobs[id]
	return j, ok
}

func listJobs() []downloadJob {
	jobsMu.Lock()
	defer jobsMu.Unlock()
	out := make([]downloadJob, 0, len(jobs))
	for _, j := range jobs {
		out = append(out, j.snapshot())
	}
	return out
}

// --- handlers ---------------------------------------------------------------

// SoulseekStatus reports whether Soulseek credentials are configured.
// GET /api/soulseek/status
func SoulseekStatus(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()

	events, err := defaultBridge.call(ctx, map[string]any{"cmd": "status"}, nil)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"loggedIn": false,
			"account":  "",
			"error":    "soulseek bridge unavailable: " + err.Error(),
		})
		return
	}

	status := map[string]any{"loggedIn": false, "account": ""}
	for _, ev := range events {
		if ev.Type == "status" {
			status["loggedIn"] = ev.LoggedIn != nil && *ev.LoggedIn
			status["account"] = ev.Account
		}
	}
	jobsMu.Lock()
	active := 0
	for _, j := range jobs {
		if j.Status == "running" {
			active++
		}
	}
	jobsMu.Unlock()
	status["activeJobs"] = active
	writeJSON(w, http.StatusOK, status)
}

// SoulseekLogin stores Soulseek credentials in the system keychain
// (same keychain entry as the soulseek-cli).
// POST /api/soulseek/login
func SoulseekLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Account  string `json:"account"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Account == "" || req.Password == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "account and password are required"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	_, err := defaultBridge.call(ctx, map[string]any{
		"cmd":      "login",
		"account":  req.Account,
		"password": req.Password,
	}, nil)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "login failed: " + err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "account": req.Account})
}

// SoulseekSearch runs a Soulseek search and caches the raw results server-side
// so downloads can reference them by index.
// POST /api/soulseek/search
func SoulseekSearch(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Query     string `json:"query"`
		Mode      string `json:"mode"`
		Quality   string `json:"quality"`
		TimeoutMs int    `json:"timeoutMs"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Query) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "query is required"})
		return
	}
	if req.Mode != "flac" {
		req.Mode = "mp3"
	}
	timeoutMs := req.TimeoutMs
	if timeoutMs <= 0 || timeoutMs > 60000 {
		timeoutMs = 8000
	}

	ctx, cancel := context.WithTimeout(r.Context(), 120*time.Second)
	defer cancel()

	events, err := defaultBridge.call(ctx, map[string]any{
		"cmd":       "search",
		"query":     req.Query,
		"mode":      req.Mode,
		"quality":   req.Quality,
		"timeoutMs": timeoutMs,
	}, nil)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "search failed: " + err.Error()})
		return
	}

	results := make([]slskResult, 0, 32)
	for _, ev := range events {
		if ev.Type != "result" {
			continue
		}
		var full struct {
			ResultIndex int              `json:"resultIndex"`
			User        string           `json:"user"`
			Folder      string           `json:"folder"`
			Files       []slskResultFile `json:"files"`
			AvgBitrate  int              `json:"avgBitrate"`
			SizeMb      int              `json:"sizeMb"`
			SpeedKbs    int              `json:"speedKbs"`
			Slots       bool             `json:"slots"`
			Raw         []slskFile       `json:"raw"`
		}
		if err := json.Unmarshal(ev.Line, &full); err != nil {
			continue
		}
		results = append(results, slskResult{
			ResultIndex: full.ResultIndex,
			User:        full.User,
			Folder:      full.Folder,
			Files:       full.Files,
			AvgBitrate:  full.AvgBitrate,
			SizeMb:      full.SizeMb,
			SpeedKbs:    full.SpeedKbs,
			Slots:       full.Slots,
			Raw:         full.Raw,
		})
	}

	searchID := storeSearch(req.Query, results)
	writeJSON(w, http.StatusOK, map[string]any{
		"searchId": searchID,
		"query":    req.Query,
		"mode":     req.Mode,
		"results":  results,
	})
}

// SoulseekDownload starts a download job for selected search results.
// POST /api/soulseek/download
func SoulseekDownload(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SearchID string `json:"searchId"`
		Items    []struct {
			ResultIndex int  `json:"resultIndex"`
			FileIndex   *int `json:"fileIndex"`
		} `json:"items"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.SearchID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "searchId is required"})
		return
	}

	entry, ok := getSearch(req.SearchID)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "search expired, search again"})
		return
	}

	// Expand the selection into concrete bridge download items.
	type dlItem struct {
		Name   string   `json:"name"`
		Folder string   `json:"folder"`
		Raw    slskFile `json:"raw"`
	}
	items := make([]dlItem, 0, 16)
	files := make([]*jobFile, 0, 16)
	seen := map[string]bool{}

	add := func(res slskResult, f slskResultFile, raw slskFile) {
		key := fmt.Sprintf("%d.%d", res.ResultIndex, f.FileIndex)
		if seen[key] {
			return
		}
		seen[key] = true
		items = append(items, dlItem{Name: f.Name, Folder: res.Folder, Raw: raw})
		files = append(files, &jobFile{Name: f.Name, Status: "pending"})
	}

	for _, sel := range req.Items {
		if sel.ResultIndex < 0 || sel.ResultIndex >= len(entry.results) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid result index"})
			return
		}
		res := entry.results[sel.ResultIndex]
		if sel.FileIndex == nil {
			for i, f := range res.Files {
				if i < len(res.Raw) {
					add(res, f, res.Raw[i])
				}
			}
			continue
		}
		fi := *sel.FileIndex
		if fi < 0 || fi >= len(res.Files) || fi >= len(res.Raw) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid file index"})
			return
		}
		add(res, res.Files[fi], res.Raw[fi])
	}

	if len(items) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "nothing selected"})
		return
	}

	dest := LibraryRoot()
	job := &downloadJob{
		ID:          strconv.FormatInt(time.Now().UnixNano(), 36),
		Query:       entry.query,
		Status:      "running",
		Files:       files,
		Destination: dest,
		Created:     time.Now(),
	}
	addJob(job)

	payload := make([]map[string]any, 0, len(items))
	for _, it := range items {
		payload = append(payload, map[string]any{
			"name":   it.Name,
			"folder": it.Folder,
			"raw":    it.Raw,
		})
	}

	go runDownloadJob(job, dest, payload)

	writeJSON(w, http.StatusOK, map[string]any{"jobId": job.ID, "files": len(items)})
}

func runDownloadJob(job *downloadJob, dest string, payload []map[string]any) {
	ctx, cancel := context.WithCancel(context.Background())
	job.mu.Lock()
	job.cancel = cancel
	job.mu.Unlock()
	defer cancel()

	onEvent := func(ev bridgeEvent) {
		job.mu.Lock()
		defer job.mu.Unlock()
		switch ev.Type {
		case "file-start":
			for _, f := range job.Files {
				if f.Name == ev.Name && f.Status == "pending" {
					f.Status = "downloading"
					f.Path = ev.Path
					break
				}
			}
		case "file-done":
			for _, f := range job.Files {
				if f.Name == ev.Name && f.Status != "done" {
					f.Status = "done"
					f.Path = ev.Path
					break
				}
			}
			job.Downloaded++
		case "file-skip":
			for _, f := range job.Files {
				if f.Name == ev.Name {
					f.Status = "skipped"
					f.Path = ev.Path
					break
				}
			}
			job.Downloaded++
		case "file-error":
			for _, f := range job.Files {
				if f.Name == ev.Name {
					f.Status = "failed"
					f.Error = ev.Message
					break
				}
			}
			job.Failed++
		}
	}

	_, err := defaultBridge.call(ctx, map[string]any{
		"cmd":   "download",
		"dest":  dest,
		"items": payload,
	}, onEvent)

	job.mu.Lock()
	defer job.mu.Unlock()
	if job.Status == "running" {
		switch {
		case ctx.Err() != nil:
			job.Status = "cancelled"
		case err != nil:
			job.Status = "failed"
			for _, f := range job.Files {
				if f.Status == "pending" || f.Status == "downloading" {
					f.Status = "failed"
					f.Error = err.Error()
				}
			}
		default:
			job.Status = "done"
		}
	}
}

// SoulseekJobs lists download jobs.
// GET /api/soulseek/jobs
func SoulseekJobs(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"jobs": listJobs()})
}

// SoulseekJob returns one download job.
// GET /api/soulseek/jobs/{id}
func SoulseekJob(w http.ResponseWriter, r *http.Request) {
	job, ok := findJob(r.PathValue("id"))
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "job not found"})
		return
	}
	writeJSON(w, http.StatusOK, job.snapshot())
}

// SoulseekCancelJob cancels a running download job.
// POST /api/soulseek/jobs/{id}/cancel
func SoulseekCancelJob(w http.ResponseWriter, r *http.Request) {
	job, ok := findJob(r.PathValue("id"))
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "job not found"})
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
