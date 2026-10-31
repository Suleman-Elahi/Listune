package handlers

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func withLibrary(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	t.Setenv("LIBRARY_PATH", root)
	return root
}

func TestScanFolderReadsLibraryRootRecursively(t *testing.T) {
	root := withLibrary(t)

	sub := filepath.Join(root, "Album")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	files := []string{
		filepath.Join(root, "one.mp3"),
		filepath.Join(sub, "two.flac"),
		filepath.Join(root, "notes.txt"),
	}
	for _, f := range files {
		if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	rec := httptest.NewRecorder()
	ScanFolder(rec, httptest.NewRequest("POST", "/api/library/scan", nil))

	if rec.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp ScanResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if resp.Total != 2 {
		t.Fatalf("expected 2 tracks (subfolders included, non-audio skipped), got %d: %+v", resp.Total, resp.Tracks)
	}

	paths := map[string]bool{}
	for _, tr := range resp.Tracks {
		paths[tr.ServerPath] = true
		if filepath.IsAbs(tr.ServerPath) {
			t.Fatalf("ServerPath must be library-relative, got %q", tr.ServerPath)
		}
	}
	if !paths["one.mp3"] || !paths["Album/two.flac"] {
		t.Fatalf("unexpected relative paths: %v", paths)
	}
}

func TestResolveLibraryPathRejectsEscape(t *testing.T) {
	root := withLibrary(t)

	if _, err := resolveLibraryPath("../outside.mp3"); err == nil {
		t.Fatal("expected traversal path to be rejected")
	}
	if _, err := resolveLibraryPath("/etc/passwd"); err == nil {
		t.Fatal("expected absolute path outside library to be rejected")
	}

	inside, err := resolveLibraryPath("sub/song.mp3")
	if err != nil {
		t.Fatalf("relative path inside library must resolve: %v", err)
	}
	if inside != filepath.Join(root, "sub", "song.mp3") {
		t.Fatalf("unexpected resolution: %s", inside)
	}
}

func TestStreamFileRejectsOutsideLibrary(t *testing.T) {
	_ = withLibrary(t)

	rec := httptest.NewRecorder()
	StreamFile(rec, httptest.NewRequest("GET", "/api/stream?path=../../secret.mp3", nil))
	if rec.Code != 403 {
		t.Fatalf("expected 403 for traversal, got %d", rec.Code)
	}
}
