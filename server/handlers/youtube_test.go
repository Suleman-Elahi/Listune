package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func resetYouTubeSearches(t *testing.T) {
	t.Helper()
	youtubeSearchMu.Lock()
	youtubeSearches = map[string]youtubeSearchEntry{}
	youtubeSearchMu.Unlock()
}

func stubYouTubeAPI(t *testing.T, body string, status int) {
	t.Helper()
	resetYouTubeSearches(t)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	old := youtubeAPIBase
	youtubeAPIBase = ts.URL
	t.Cleanup(func() {
		youtubeAPIBase = old
		ts.Close()
	})
}

func youtubeCall(t *testing.T, handler http.HandlerFunc, method, target, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, target, bytes.NewReader([]byte(body)))
	if id := strings.TrimPrefix(target, "/api/youtube/jobs/"); id != target {
		id, _, _ = strings.Cut(id, "/")
		req.SetPathValue("id", id)
	}
	rec := httptest.NewRecorder()
	handler(rec, req)
	return rec
}

const youtubeSearchBody = `{
  "items": [
    {
      "id": {"kind": "youtube#video", "videoId": "abcdefghijk"},
      "snippet": {
        "title": "Test Song",
        "description": "desc",
        "channelTitle": "Test Channel",
        "publishedAt": "2026-01-01T00:00:00Z",
        "liveBroadcastContent": "none",
        "thumbnails": {"medium": {"url": "https://img.example/thumb.jpg"}}
      }
    },
    {"id": {"kind": "youtube#channel", "channelId": "channel1"}, "snippet": {"title": "A channel"}},
    {
      "id": {"kind": "youtube#video", "videoId": "livevideo00"},
      "snippet": {"title": "Live", "liveBroadcastContent": "live"}
    }
  ],
  "nextPageToken": "next-page"
}`

func TestYouTubeSearchMapsOnlyVideos(t *testing.T) {
	stubYouTubeAPI(t, youtubeSearchBody, http.StatusOK)

	rec := youtubeCall(t, YouTubeSearch, http.MethodPost, "/api/youtube/search?token=test-token", `{"query":"test song"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var got struct {
		SearchID      string         `json:"searchId"`
		NextPageToken string         `json:"nextPageToken"`
		Results       []youtubeVideo `json:"results"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.SearchID == "" {
		t.Fatal("missing searchId")
	}
	if got.NextPageToken != "next-page" {
		t.Fatalf("nextPageToken = %q", got.NextPageToken)
	}
	if len(got.Results) != 1 {
		t.Fatalf("results = %d, want 1 (only non-live videos)", len(got.Results))
	}
	v := got.Results[0]
	if v.VideoID != "abcdefghijk" || v.Title != "Test Song" || v.ChannelTitle != "Test Channel" {
		t.Fatalf("unexpected video: %+v", v)
	}
	if v.ThumbnailURL != "https://img.example/thumb.jpg" {
		t.Fatalf("thumbnail = %q", v.ThumbnailURL)
	}
}

func TestYouTubeSearchDeduplicatesRepeatedQueries(t *testing.T) {
	resetYouTubeSearches(t)
	calls := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		_, _ = w.Write([]byte(`{"items":[]}`))
	}))
	old := youtubeAPIBase
	youtubeAPIBase = ts.URL
	t.Cleanup(func() {
		youtubeAPIBase = old
		ts.Close()
	})

	ids := make([]string, 0, 2)
	for range 2 {
		rec := youtubeCall(t, YouTubeSearch, http.MethodPost, "/api/youtube/search?token=test-token", `{"query":"dedupe"}`)
		var got struct {
			SearchID string `json:"searchId"`
		}
		_ = json.Unmarshal(rec.Body.Bytes(), &got)
		ids = append(ids, got.SearchID)
	}
	if ids[0] == "" || ids[0] != ids[1] {
		t.Fatalf("search IDs = %v, want the same cached ID", ids)
	}
	if calls != 1 {
		t.Fatalf("youtube api calls = %d, want 1", calls)
	}
}

func TestYouTubeSearchErrors(t *testing.T) {
	tests := []struct {
		name       string
		target     string
		body       string
		apiBody    string
		apiStatus  int
		wantStatus int
		wantSub    string
	}{
		{name: "missing token", target: "/api/youtube/search", body: `{"query":"test"}`, apiBody: `{}`, apiStatus: http.StatusOK, wantStatus: http.StatusUnauthorized},
		{name: "empty query", target: "/api/youtube/search?token=test-token", body: `{"query":"  "}`, apiBody: `{}`, apiStatus: http.StatusOK, wantStatus: http.StatusBadRequest},
		{name: "api failure", target: "/api/youtube/search?token=test-token", body: `{"query":"test"}`, apiBody: `{"error":{"message":"YouTube Data API v3 is disabled"}}`, apiStatus: http.StatusForbidden, wantStatus: http.StatusBadGateway, wantSub: "YouTube Data API v3 is disabled"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stubYouTubeAPI(t, tt.apiBody, tt.apiStatus)
			rec := youtubeCall(t, YouTubeSearch, http.MethodPost, tt.target, tt.body)
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			if tt.wantSub != "" && !strings.Contains(rec.Body.String(), tt.wantSub) {
				t.Fatalf("body = %q, want substring %q", rec.Body.String(), tt.wantSub)
			}
		})
	}
}

func TestYouTubeStatus(t *testing.T) {
	rec := youtubeCall(t, YouTubeStatus, http.MethodGet, "/api/youtube/status", "")
	if !strings.Contains(rec.Body.String(), `"available":false`) {
		t.Fatalf("body = %s", rec.Body.String())
	}

	rec = youtubeCall(t, YouTubeStatus, http.MethodGet, "/api/youtube/status?token=test-token", "")
	if !strings.Contains(rec.Body.String(), `"available":true`) {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

func TestYouTubeDownloadUsesCachedVideosOnly(t *testing.T) {
	t.Setenv("LIBRARY_PATH", t.TempDir())
	stubYouTubeAPI(t, youtubeSearchBody, http.StatusOK)

	called := false
	old := runYouTubeDownload
	runYouTubeDownload = func(_ context.Context, _ string, _ youtubeVideo, _ string, _ func(int)) error {
		called = true
		return nil
	}
	t.Cleanup(func() { runYouTubeDownload = old })

	search := youtubeCall(t, YouTubeSearch, http.MethodPost, "/api/youtube/search?token=test-token", `{"query":"test"}`)
	var searchResp struct {
		SearchID string `json:"searchId"`
	}
	if err := json.Unmarshal(search.Body.Bytes(), &searchResp); err != nil {
		t.Fatalf("decode search: %v", err)
	}

	tests := []struct {
		name       string
		body       string
		wantStatus int
	}{
		{
			name:       "unknown video is rejected",
			body:       `{"searchId":"` + searchResp.SearchID + `","videos":[{"videoId":"notincache"}]}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "unknown search is rejected",
			body:       `{"searchId":"missing","videos":[{"videoId":"abcdefghijk"}]}`,
			wantStatus: http.StatusNotFound,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := youtubeCall(t, YouTubeDownload, http.MethodPost, "/api/youtube/download?token=test-token", tt.body)
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
		})
	}
	if called {
		t.Fatal("downloader must not run for rejected selections")
	}
}

func waitForYouTubeJob(t *testing.T, id string) youtubeJob {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		rec := youtubeCall(t, YouTubeJob, http.MethodGet, "/api/youtube/jobs/"+id, "")
		if rec.Code == http.StatusOK {
			var job youtubeJob
			if err := json.Unmarshal(rec.Body.Bytes(), &job); err != nil {
				t.Fatalf("decode job: %v", err)
			}
			if job.Status != "running" {
				return job
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("job did not finish")
	return youtubeJob{}
}

func TestYouTubeDownloadJobProgressAndCancel(t *testing.T) {
	t.Setenv("LIBRARY_PATH", t.TempDir())
	stubYouTubeAPI(t, youtubeSearchBody, http.StatusOK)

	old := runYouTubeDownload
	runYouTubeDownload = func(ctx context.Context, _ string, _ youtubeVideo, _ string, onProgress func(int)) error {
		onProgress(50)
		<-ctx.Done()
		return ctx.Err()
	}
	t.Cleanup(func() { runYouTubeDownload = old })

	search := youtubeCall(t, YouTubeSearch, http.MethodPost, "/api/youtube/search?token=test-token", `{"query":"test"}`)
	var searchResp struct {
		SearchID string `json:"searchId"`
	}
	_ = json.Unmarshal(search.Body.Bytes(), &searchResp)

	rec := youtubeCall(t, YouTubeDownload, http.MethodPost, "/api/youtube/download?token=test-token",
		`{"searchId":"`+searchResp.SearchID+`","videos":[{"videoId":"abcdefghijk"}],"quality":"invalid"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var start struct {
		JobID string `json:"jobId"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &start); err != nil {
		t.Fatalf("decode start: %v", err)
	}
	if start.JobID == "" {
		t.Fatal("missing jobId")
	}

	// The stub blocks until cancellation; make sure progress was reported first.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		jobRec := youtubeCall(t, YouTubeJob, http.MethodGet, "/api/youtube/jobs/"+start.JobID, "")
		var inProgress youtubeJob
		_ = json.Unmarshal(jobRec.Body.Bytes(), &inProgress)
		if len(inProgress.Files) == 1 && inProgress.Files[0].Progress == 50 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	cancel := youtubeCall(t, YouTubeCancelJob, http.MethodPost, "/api/youtube/jobs/"+start.JobID+"/cancel", "")
	if cancel.Code != http.StatusOK {
		t.Fatalf("cancel status = %d", cancel.Code)
	}
	var job youtubeJob
	deadline = time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		job = waitForYouTubeJob(t, start.JobID)
		if len(job.Files) == 1 && job.Files[0].Status != "downloading" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if job.Status != "cancelled" {
		t.Fatalf("status after cancel = %q", job.Status)
	}
	if len(job.Files) != 1 {
		t.Fatalf("files after cancel = %d", len(job.Files))
	}
	if job.Files[0].Progress != 50 || job.Files[0].Status != "failed" {
		t.Fatalf("file after cancel = %+v (progress=%d status=%q)", *job.Files[0], job.Files[0].Progress, job.Files[0].Status)
	}
}
