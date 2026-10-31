package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// fakeBridge writes a stub "node" executable that implements the bridge's
// serve mode: it answers NDJSON commands read from stdin.
func fakeBridge(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	py := filepath.Join(dir, "fake_bridge.py")
	script := `import json, sys
for line in sys.stdin:
    line = line.strip()
    if not line:
        continue
    req = json.loads(line)
    rid = req.get("id", "")
    cmd = req.get("cmd")
    def out(obj):
        obj["id"] = rid
        sys.stdout.write(json.dumps(obj) + "\n")
        sys.stdout.flush()
    if cmd == "status":
        out({"type": "status", "loggedIn": True, "account": "tester"})
    elif cmd == "login":
        out({"type": "ok", "account": "tester"})
    elif cmd == "search":
        out({"type": "result", "resultIndex": 0, "user": "u1", "folder": "Album",
             "files": [{"fileIndex": 0, "name": "a.mp3", "size": 100, "bitrate": 320}],
             "avgBitrate": 320, "sizeMb": 1, "speedKbs": 100, "slots": True,
             "raw": [{"user": "u1", "file": "Music\\Album\\a.mp3", "size": 100,
                      "slots": True, "bitrate": 320, "speed": 1024}]})
        out({"type": "result", "resultIndex": 1, "user": "u2", "folder": "Other",
             "files": [{"fileIndex": 0, "name": "b.mp3", "size": 200, "bitrate": 192}],
             "avgBitrate": 192, "sizeMb": 1, "speedKbs": 50, "slots": True,
             "raw": [{"user": "u2", "file": "Music\\Other\\b.mp3", "size": 200,
                      "slots": True, "bitrate": 192, "speed": 512}]})
        out({"type": "done", "count": 2})
    elif cmd == "download":
        out({"type": "starting", "count": 1})
        out({"type": "file-start", "name": "a.mp3", "path": "/tmp/a.mp3"})
        out({"type": "file-done", "name": "a.mp3", "path": "/tmp/a.mp3"})
        out({"type": "done", "downloaded": 1, "failed": 0})
    else:
        out({"type": "error", "message": "unknown command"})
`
	if err := os.WriteFile(py, []byte(script), 0o644); err != nil {
		t.Fatal(err)
	}
	node := filepath.Join(dir, "fake-node")
	if err := os.WriteFile(node, []byte("#!/bin/bash\nexec python3 "+py+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SOULSEEK_NODE", node)
	t.Setenv("SOULSEEK_BRIDGE", "unused.mjs")
}

func TestSoulseekStatusReportsLogin(t *testing.T) {
	fakeBridge(t)

	rec := httptest.NewRecorder()
	SoulseekStatus(rec, httptest.NewRequest("GET", "/api/soulseek/status", nil))

	if rec.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["loggedIn"] != true || body["account"] != "tester" {
		t.Fatalf("unexpected status body: %v", body)
	}
}

func TestSoulseekSearchReturnsResultsWithoutRawPayload(t *testing.T) {
	fakeBridge(t)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/soulseek/search", bytes.NewReader([]byte(`{"query":"pink floyd"}`)))
	SoulseekSearch(rec, req)

	if rec.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var body struct {
		SearchID string `json:"searchId"`
		Results  []struct {
			User   string `json:"user"`
			Folder string `json:"folder"`
			Files  []struct {
				Name string `json:"name"`
			} `json:"files"`
			Raw json.RawMessage `json:"raw"`
		} `json:"results"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.SearchID == "" {
		t.Fatal("expected a searchId")
	}
	if len(body.Results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(body.Results))
	}
	if body.Results[0].User != "u1" || body.Results[0].Files[0].Name != "a.mp3" {
		t.Fatalf("unexpected first result: %+v", body.Results[0])
	}
	if len(body.Results[0].Raw) != 0 {
		t.Fatalf("raw slsk payloads must not leak to the API: %s", body.Results[0].Raw)
	}
}

func TestSoulseekDownloadRunsJob(t *testing.T) {
	_ = withLibrary(t)
	fakeBridge(t)

	searchRec := httptest.NewRecorder()
	SoulseekSearch(searchRec, httptest.NewRequest("POST", "/api/soulseek/search", bytes.NewReader([]byte(`{"query":"q"}`))))
	var searchBody struct {
		SearchID string `json:"searchId"`
	}
	if err := json.Unmarshal(searchRec.Body.Bytes(), &searchBody); err != nil {
		t.Fatal(err)
	}

	dlRec := httptest.NewRecorder()
	payload := `{"searchId":"` + searchBody.SearchID + `","items":[{"resultIndex":0}]}`
	SoulseekDownload(dlRec, httptest.NewRequest("POST", "/api/soulseek/download", bytes.NewReader([]byte(payload))))
	if dlRec.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", dlRec.Code, dlRec.Body.String())
	}
	var dlBody struct {
		JobID string `json:"jobId"`
		Files int    `json:"files"`
	}
	if err := json.Unmarshal(dlRec.Body.Bytes(), &dlBody); err != nil {
		t.Fatal(err)
	}
	if dlBody.JobID == "" || dlBody.Files != 1 {
		t.Fatalf("unexpected download body: %+v", dlBody)
	}

	// Poll until the fake bridge finishes (PathValue needs a real mux).
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/soulseek/jobs/{id}", SoulseekJob)
	deadline := time.Now().Add(5 * time.Second)
	for {
		jobRec := httptest.NewRecorder()
		mux.ServeHTTP(jobRec, httptest.NewRequest("GET", "/api/soulseek/jobs/"+dlBody.JobID, nil))
		var job downloadJob
		if err := json.Unmarshal(jobRec.Body.Bytes(), &job); err != nil {
			t.Fatal(err)
		}
		if job.Status != "running" {
			if job.Status != "done" {
				t.Fatalf("expected done, got %q", job.Status)
			}
			if job.Downloaded != 1 || len(job.Files) != 1 || job.Files[0].Status != "done" {
				var fileStates []string
				for _, f := range job.Files {
					fileStates = append(fileStates, f.Name+":"+f.Status)
				}
				t.Fatalf("unexpected job state: status=%q downloaded=%d files=%v", job.Status, job.Downloaded, fileStates)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("job did not finish in time")
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func TestSoulseekDownloadRejectsExpiredSearch(t *testing.T) {
	fakeBridge(t)

	rec := httptest.NewRecorder()
	payload := `{"searchId":"nope","items":[{"resultIndex":0}]}`
	SoulseekDownload(rec, httptest.NewRequest("POST", "/api/soulseek/download", bytes.NewReader([]byte(payload))))
	if rec.Code != 404 {
		t.Fatalf("expected 404 for unknown search, got %d", rec.Code)
	}
}
