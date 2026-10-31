package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/bogem/id3v2/v2"
)

// WriteTagsRequest contains the metadata to write to a file.
type WriteTagsRequest struct {
	// For server-side files
	FilePath string `json:"filePath,omitempty"`
	// For S3-hosted files
	S3Key          string `json:"s3Key,omitempty"`
	S3Endpoint     string `json:"s3Endpoint,omitempty"`
	S3Bucket       string `json:"s3Bucket,omitempty"`
	S3Region       string `json:"s3Region,omitempty"`
	S3Access       string `json:"s3Access,omitempty"`
	S3Secret       string `json:"s3Secret,omitempty"`
	S3SessionToken string `json:"s3SessionToken,omitempty"`
	// Metadata to write
	Title       string `json:"title,omitempty"`
	Artist      string `json:"artist,omitempty"`
	Album       string `json:"album,omitempty"`
	Year        string `json:"year,omitempty"`
	Genre       string `json:"genre,omitempty"`
	CoverArtURL string `json:"coverArtUrl,omitempty"`
}

// WriteTagsResponse is the result of the tag write operation.
type WriteTagsResponse struct {
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
}

// WriteTags writes ID3v2 metadata to an audio file (local or S3).
// POST /api/write-tags
func WriteTags(w http.ResponseWriter, r *http.Request) {
	var req WriteTagsRequest
	body, _ := io.ReadAll(r.Body)
	if err := json.Unmarshal(body, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, WriteTagsResponse{Error: "invalid request body"})
		return
	}

	if req.FilePath == "" && req.S3Key == "" {
		writeJSON(w, http.StatusBadRequest, WriteTagsResponse{Error: "filePath or s3Key is required"})
		return
	}

	// Route to appropriate handler
	if req.S3Key != "" {
		writeTagsS3(w, r, &req)
	} else {
		writeTagsLocal(w, &req)
	}
}

// writeTagsLocal writes tags to a local file on the server.
func writeTagsLocal(w http.ResponseWriter, req *WriteTagsRequest) {
	resolved, err := resolveLibraryPath(req.FilePath)
	if err != nil {
		writeJSON(w, http.StatusForbidden, WriteTagsResponse{Error: "path outside library"})
		return
	}

	ext := strings.ToLower(filepath.Ext(resolved))
	if ext != ".mp3" {
		writeJSON(w, http.StatusBadRequest, WriteTagsResponse{
			Error: "tag writing currently supported for .mp3 files only",
		})
		return
	}

	if _, err := os.Stat(resolved); err != nil {
		writeJSON(w, http.StatusNotFound, WriteTagsResponse{Error: "file not found"})
		return
	}

	if err := applyTags(resolved, req); err != nil {
		writeJSON(w, http.StatusInternalServerError, WriteTagsResponse{Error: err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, WriteTagsResponse{Success: true})
}

// writeTagsS3 downloads from S3, writes tags, re-uploads.
func writeTagsS3(w http.ResponseWriter, r *http.Request, req *WriteTagsRequest) {
	ext := strings.ToLower(filepath.Ext(req.S3Key))
	if ext != ".mp3" {
		writeJSON(w, http.StatusBadRequest, WriteTagsResponse{
			Error: "tag writing currently supported for .mp3 files only",
		})
		return
	}

	if req.S3Endpoint == "" || req.S3Bucket == "" || req.S3Region == "" || req.S3Access == "" || req.S3Secret == "" {
		writeJSON(w, http.StatusBadRequest, WriteTagsResponse{Error: "S3 credentials required for S3 tag writing"})
		return
	}

	// 1. Download from S3
	downloadURL, err := generatePresignedURL(req.S3Endpoint, req.S3Bucket, req.S3Region, req.S3Access, req.S3Secret, req.S3SessionToken, req.S3Key, "GET")
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, WriteTagsResponse{Error: "failed to generate download URL: " + err.Error()})
		return
	}

	reqDownload, err := http.NewRequestWithContext(r.Context(), http.MethodGet, downloadURL, nil)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, WriteTagsResponse{Error: "failed to build download request"})
		return
	}
	resp, err := getS3HTTPClient().Do(reqDownload)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, WriteTagsResponse{Error: "failed to download from S3: " + err.Error()})
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		writeJSON(w, http.StatusBadGateway, WriteTagsResponse{Error: fmt.Sprintf("S3 download returned %d", resp.StatusCode)})
		return
	}

	// Write to temp file
	tmpFile, err := os.CreateTemp("", "listune-s3-*"+ext)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, WriteTagsResponse{Error: "failed to create temp file"})
		return
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	if _, err := io.Copy(tmpFile, resp.Body); err != nil {
		tmpFile.Close()
		writeJSON(w, http.StatusInternalServerError, WriteTagsResponse{Error: "failed to save downloaded file"})
		return
	}
	tmpFile.Close()

	// 2. Write tags
	if err := applyTags(tmpPath, req); err != nil {
		writeJSON(w, http.StatusInternalServerError, WriteTagsResponse{Error: err.Error()})
		return
	}

	// 3. Re-upload to S3
	if err := uploadToS3(r.Context(), req.S3Endpoint, req.S3Bucket, req.S3Region, req.S3Access, req.S3Secret, req.S3SessionToken, req.S3Key, tmpPath); err != nil {
		writeJSON(w, http.StatusBadGateway, WriteTagsResponse{Error: "failed to upload to S3: " + err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, WriteTagsResponse{Success: true})
}

// applyTags opens an MP3 file and writes ID3v2 tags to it.
func applyTags(filePath string, req *WriteTagsRequest) error {
	tag, err := id3v2.Open(filePath, id3v2.Options{Parse: true})
	if err != nil {
		return fmt.Errorf("failed to open file for tagging: %w", err)
	}
	defer tag.Close()

	if req.Title != "" {
		tag.SetTitle(req.Title)
	}
	if req.Artist != "" {
		tag.SetArtist(req.Artist)
	}
	if req.Album != "" {
		tag.SetAlbum(req.Album)
	}
	if req.Year != "" {
		tag.SetYear(req.Year)
	}
	if req.Genre != "" {
		tag.SetGenre(req.Genre)
	}

	// Download and embed cover art if URL provided
	if req.CoverArtURL != "" {
		artData, mimeType, err := downloadImage(req.CoverArtURL)
		if err == nil && len(artData) > 0 {
			pic := id3v2.PictureFrame{
				Encoding:    id3v2.EncodingUTF8,
				MimeType:    mimeType,
				PictureType: id3v2.PTFrontCover,
				Description: "Front Cover",
				Picture:     artData,
			}
			tag.AddAttachedPicture(pic)
		}
	}

	if err := tag.Save(); err != nil {
		return fmt.Errorf("failed to save tags: %w", err)
	}
	return nil
}

// uploadToS3 uploads a local file to S3 using a PUT request with SigV4.
func uploadToS3(ctx context.Context, endpoint, bucket, region, accessKey, secretKey, sessionToken, key, localPath string) error {
	fileData, err := os.ReadFile(localPath)
	if err != nil {
		return err
	}

	parsed, err := parseS3Endpoint(endpoint)
	if err != nil {
		return err
	}

	now := time.Now().UTC()
	dateStamp := now.Format("20060102")
	amzDate := now.Format("20060102T150405Z")
	canonicalURI := canonicalS3URI(parsed, bucket, key)
	payloadHash := sha256Hex(fileData)
	contentType := contentTypeForFile(localPath)

	canonicalHeaders := fmt.Sprintf(
		"host:%s\nx-amz-content-sha256:%s\nx-amz-date:%s\n",
		parsed.host, payloadHash, amzDate,
	)
	signedHeaders := "host;x-amz-content-sha256;x-amz-date"
	if sessionToken != "" {
		canonicalHeaders += "x-amz-security-token:" + sessionToken + "\n"
		signedHeaders += ";x-amz-security-token"
	}

	credentialScope := fmt.Sprintf("%s/%s/s3/aws4_request", dateStamp, region)
	canonicalRequest := strings.Join([]string{
		http.MethodPut,
		canonicalURI,
		"",
		canonicalHeaders,
		signedHeaders,
		payloadHash,
	}, "\n")

	stringToSign := strings.Join([]string{
		"AWS4-HMAC-SHA256",
		amzDate,
		credentialScope,
		sha256Hex([]byte(canonicalRequest)),
	}, "\n")

	signature := fmt.Sprintf("%x", hmacSHA256(signingKey(secretKey, dateStamp, region), stringToSign))
	authorization := fmt.Sprintf(
		"AWS4-HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		accessKey, credentialScope, signedHeaders, signature,
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, parsed.origin+canonicalURI, bytes.NewReader(fileData))
	if err != nil {
		return err
	}
	req.Header.Set("X-Amz-Content-Sha256", payloadHash)
	req.Header.Set("X-Amz-Date", amzDate)
	req.Header.Set("Authorization", authorization)
	req.Header.Set("Content-Type", contentType)
	if sessionToken != "" {
		req.Header.Set("X-Amz-Security-Token", sessionToken)
	}

	resp, err := getS3HTTPClient().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("S3 PUT returned %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

func contentTypeForFile(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".mp3":
		return "audio/mpeg"
	case ".flac":
		return "audio/flac"
	case ".ogg", ".opus":
		return "audio/ogg"
	case ".m4a", ".aac":
		return "audio/mp4"
	case ".wav":
		return "audio/wav"
	default:
		return "application/octet-stream"
	}
}

func downloadImage(rawURL string) ([]byte, string, error) {
	target, err := validateProxyTarget(rawURL)
	if err != nil {
		return nil, "", err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return nil, "", err
	}

	resp, err := getImageHTTPClient().Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return nil, "", err
	}

	mimeType := resp.Header.Get("Content-Type")
	if mimeType == "" {
		mimeType = "image/jpeg"
	}

	return data, mimeType, nil
}
