package handlers

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

const dataDir = "data/userstate"

func init() {
	os.MkdirAll(dataDir, 0755)
}

// sanitizeKey ensures the key is safe for filesystem use
func sanitizeKey(key string) string {
	key = strings.ReplaceAll(key, "..", "")
	key = strings.ReplaceAll(key, "/", "_")
	key = strings.ReplaceAll(key, "\\", "_")
	if key == "" {
		key = "default"
	}
	return key
}

// GetUserState handles GET /api/state?key=<key>
func GetUserState(w http.ResponseWriter, r *http.Request) {
	key := sanitizeKey(r.URL.Query().Get("key"))
	if key == "" {
		http.Error(w, `{"error":"missing key"}`, http.StatusBadRequest)
		return
	}

	filePath := filepath.Join(dataDir, key+".json")
	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte("null"))
			return
		}
		http.Error(w, `{"error":"read failed"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(data)
}

// PutUserState handles PUT /api/state?key=<key>
func PutUserState(w http.ResponseWriter, r *http.Request) {
	key := sanitizeKey(r.URL.Query().Get("key"))
	if key == "" {
		http.Error(w, `{"error":"missing key"}`, http.StatusBadRequest)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20)) // 1MB max
	if err != nil {
		http.Error(w, `{"error":"read body failed"}`, http.StatusBadRequest)
		return
	}

	// Validate it's valid JSON
	if !json.Valid(body) {
		http.Error(w, `{"error":"invalid JSON"}`, http.StatusBadRequest)
		return
	}

	filePath := filepath.Join(dataDir, key+".json")
	if err := os.WriteFile(filePath, body, 0644); err != nil {
		http.Error(w, `{"error":"write failed"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"success":true}`))
}
