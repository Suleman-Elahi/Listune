package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"reflect"
	"strings"
)

const driveAPIBase = "https://www.googleapis.com/drive/v3"

type driveFile struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	MimeType string `json:"mimeType"`
	Size     string `json:"size,omitempty"`
}

type driveListResponse struct {
	Files         []driveFile `json:"files"`
	NextPageToken string      `json:"nextPageToken"`
}

// driveToken resolves the handle supplied by the frontend into a valid Google
// access token (refreshing it server-side when it expired).
func driveToken(r *http.Request) (string, error) {
	raw := r.URL.Query().Get("token")
	if raw == "" {
		raw = extractBearer(r.Header.Get("Authorization"))
	}
	return resolveAccessToken(r.Context(), raw)
}

func driveList(ctx context.Context, parentID, token, pageToken string) ([]driveFile, string, error) {
	q := fmt.Sprintf("'%s' in parents and trashed = false", parentID)
	u := driveAPIBase + "/files?q=" + url.QueryEscape(q) +
		"&fields=" + url.QueryEscape("nextPageToken,files(id,name,mimeType,size)") +
		"&pageSize=1000"
	if pageToken != "" {
		u += "&pageToken=" + url.QueryEscape(pageToken)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusUnauthorized {
		return nil, "", errGoogleUnauthorized
	}
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("drive api %d: %s", resp.StatusCode, string(body))
	}
	var lr driveListResponse
	if err := json.Unmarshal(body, &lr); err != nil {
		return nil, "", err
	}
	return lr.Files, lr.NextPageToken, nil
}

func isAudioMime(mime, name string) bool {
	mime = strings.ToLower(mime)
	if strings.HasPrefix(mime, "audio/") {
		return true
	}
	ext := ""
	if i := strings.LastIndex(name, "."); i >= 0 {
		ext = strings.ToLower(name[i:])
	}
	switch ext {
	case ".mp3", ".flac", ".ogg", ".oga", ".opus", ".m4a", ".m4b", ".wav", ".aac", ".wma", ".aiff", ".aif", ".ape", ".webm":
		return true
	}
	return false
}

func isFolder(mime string) bool {
	return mime == "application/vnd.google-apps.folder"
}

func writeJSONError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// writeJSONList encodes a slice as a JSON array, never as `null`.
// A typed nil slice would otherwise be encoded as `null`, which crashes the
// frontend templates that do `folders.length`.
func writeJSONList(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	if v == nil {
		w.Write([]byte("[]"))
		return
	}
	if rv := reflect.ValueOf(v); (rv.Kind() == reflect.Slice || rv.Kind() == reflect.Map) && rv.IsNil() {
		w.Write([]byte("[]"))
		return
	}
	json.NewEncoder(w).Encode(v)
}

// List immediate child folders for browsing.
func GDriveFoldersHandler(w http.ResponseWriter, r *http.Request) {
	token, err := driveToken(r)
	if err != nil {
		writeDriveError(w, err)
		return
	}
	parentID := r.URL.Query().Get("parentId")
	if parentID == "" {
		parentID = "root"
	}
	folders := make([]driveFile, 0, 32)
	pageToken := ""
	for {
		files, next, err := driveList(r.Context(), parentID, token, pageToken)
		if err != nil {
			writeDriveError(w, err)
			return
		}
		for _, f := range files {
			if isFolder(f.MimeType) {
				folders = append(folders, f)
			}
		}
		if next == "" {
			break
		}
		pageToken = next
	}
	writeJSONList(w, folders)
}

// Recursively collect audio files under a folder.
func collectDriveAudio(ctx context.Context, folderID, token string) ([]driveFile, error) {
	result := make([]driveFile, 0, 64)
	pageToken := ""
	for {
		files, next, err := driveList(ctx, folderID, token, pageToken)
		if err != nil {
			return nil, err
		}
		for _, f := range files {
			if isFolder(f.MimeType) {
				sub, err := collectDriveAudio(ctx, f.ID, token)
				if err != nil {
					return nil, err
				}
				result = append(result, sub...)
			} else if isAudioMime(f.MimeType, f.Name) {
				result = append(result, f)
			}
		}
		if next == "" {
			break
		}
		pageToken = next
	}
	return result, nil
}

func GDriveFilesHandler(w http.ResponseWriter, r *http.Request) {
	token, err := driveToken(r)
	if err != nil {
		writeDriveError(w, err)
		return
	}
	folderID := r.URL.Query().Get("folderId")
	if folderID == "" {
		folderID = "root"
	}
	files, err := collectDriveAudio(r.Context(), folderID, token)
	if err != nil {
		writeDriveError(w, err)
		return
	}
	writeJSONList(w, files)
}

func GDriveStreamHandler(w http.ResponseWriter, r *http.Request) {
	fileID := r.URL.Query().Get("fileId")
	token, err := driveToken(r)
	if err != nil {
		writeDriveError(w, err)
		return
	}
	if fileID == "" {
		http.Error(w, `{"error":"missing fileId"}`, http.StatusBadRequest)
		return
	}

	// metadata
	metaURL := fmt.Sprintf("%s/files/%s?fields=size,mimeType,name", driveAPIBase, url.PathEscape(fileID))
	mreq, _ := http.NewRequestWithContext(r.Context(), http.MethodGet, metaURL, nil)
	mreq.Header.Set("Authorization", "Bearer "+token)
	mresp, err := http.DefaultClient.Do(mreq)
	if err != nil {
		http.Error(w, "metadata lookup failed", http.StatusBadGateway)
		return
	}
	defer mresp.Body.Close()
	if mresp.StatusCode == http.StatusUnauthorized {
		writeDriveError(w, errGoogleUnauthorized)
		return
	}
	var meta struct {
		Size     string `json:"size"`
		MimeType string `json:"mimeType"`
		Name     string `json:"name"`
	}
	mbody, _ := io.ReadAll(mresp.Body)
	json.Unmarshal(mbody, &meta)

	downloadURL := fmt.Sprintf("%s/files/%s?alt=media", driveAPIBase, url.PathEscape(fileID))
	dreq, _ := http.NewRequestWithContext(r.Context(), http.MethodGet, downloadURL, nil)
	dreq.Header.Set("Authorization", "Bearer "+token)
	if rng := r.Header.Get("Range"); rng != "" {
		dreq.Header.Set("Range", rng)
	}
	dresp, err := http.DefaultClient.Do(dreq)
	if err != nil {
		http.Error(w, "stream fetch failed", http.StatusBadGateway)
		return
	}
	defer dresp.Body.Close()
	if dresp.StatusCode == http.StatusUnauthorized {
		writeDriveError(w, errGoogleUnauthorized)
		return
	}

	if meta.MimeType != "" {
		w.Header().Set("Content-Type", meta.MimeType)
	}
	if dresp.Header.Get("Content-Range") != "" {
		w.Header().Set("Content-Range", dresp.Header.Get("Content-Range"))
		w.WriteHeader(http.StatusPartialContent)
	} else if meta.Size != "" {
		w.Header().Set("Content-Length", meta.Size)
	}
	w.Header().Set("Accept-Ranges", "bytes")

	io.Copy(w, dresp.Body)
}
