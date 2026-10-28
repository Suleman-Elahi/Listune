package handlers

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWriteJSONListNeverNull(t *testing.T) {
	cases := map[string]any{
		"nil any":     nil,
		"nil slice":   []driveFile(nil),
		"empty slice": []driveFile{},
		"populated":   []driveFile{{ID: "1", Name: "Music", MimeType: "application/vnd.google-apps.folder"}},
	}

	for name, v := range cases {
		t.Run(name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			writeJSONList(rec, v)

			body := strings.TrimSpace(rec.Body.String())
			if body == "null" {
				t.Fatal("encoded null; frontend templates crash on `.length`")
			}
			if !json.Valid([]byte(body)) {
				t.Fatalf("invalid JSON: %q", body)
			}
			if !strings.HasPrefix(body, "[") {
				t.Fatalf("expected a JSON array, got %q", body)
			}
		})
	}
}

func TestGDriveFoldersHandlerRequiresToken(t *testing.T) {
	rec := httptest.NewRecorder()
	GDriveFoldersHandler(rec, httptest.NewRequest("GET", "/api/gdrive/folders", nil))

	if rec.Code != 401 {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("expected JSON error body: %v", err)
	}
	if body["error"] == "" {
		t.Fatal("expected an error message")
	}
}

func TestGDriveFilesHandlerRequiresToken(t *testing.T) {
	rec := httptest.NewRecorder()
	GDriveFilesHandler(rec, httptest.NewRequest("GET", "/api/gdrive/files", nil))

	if rec.Code != 401 {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}
