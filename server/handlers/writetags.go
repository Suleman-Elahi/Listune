package handlers

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
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
	S3Key      string `json:"s3Key,omitempty"`
	S3Endpoint string `json:"s3Endpoint,omitempty"`
	S3Bucket   string `json:"s3Bucket,omitempty"`
	S3Region   string `json:"s3Region,omitempty"`
	S3Access   string `json:"s3Access,omitempty"`
	S3Secret   string `json:"s3Secret,omitempty"`
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
		writeTagsS3(w, &req)
	} else {
		writeTagsLocal(w, &req)
	}
}

// writeTagsLocal writes tags to a local file on the server.
func writeTagsLocal(w http.ResponseWriter, req *WriteTagsRequest) {
	ext := strings.ToLower(filepath.Ext(req.FilePath))
	if ext != ".mp3" {
		writeJSON(w, http.StatusBadRequest, WriteTagsResponse{
			Error: "tag writing currently supported for .mp3 files only",
		})
		return
	}

	if _, err := os.Stat(req.FilePath); err != nil {
		writeJSON(w, http.StatusNotFound, WriteTagsResponse{Error: "file not found"})
		return
	}

	if err := applyTags(req.FilePath, req); err != nil {
		writeJSON(w, http.StatusInternalServerError, WriteTagsResponse{Error: err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, WriteTagsResponse{Success: true})
}

// writeTagsS3 downloads from S3, writes tags, re-uploads.
func writeTagsS3(w http.ResponseWriter, req *WriteTagsRequest) {
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
	downloadURL, err := generatePresignedURL(req.S3Endpoint, req.S3Bucket, req.S3Region, req.S3Access, req.S3Secret, req.S3Key, "GET")
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, WriteTagsResponse{Error: "failed to generate download URL: " + err.Error()})
		return
	}

	resp, err := http.Get(downloadURL)
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
	if err := uploadToS3(req.S3Endpoint, req.S3Bucket, req.S3Region, req.S3Access, req.S3Secret, req.S3Key, tmpPath); err != nil {
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
func uploadToS3(endpoint, bucket, region, accessKey, secretKey, key, localPath string) error {
	fileData, err := os.ReadFile(localPath)
	if err != nil {
		return err
	}

	now := time.Now().UTC()
	dateStamp := now.Format("20060102")
	amzDate := now.Format("20060102T150405Z")

	endpointURL := strings.TrimRight(endpoint, "/")
	host := strings.TrimPrefix(strings.TrimPrefix(endpointURL, "https://"), "http://")
	canonicalURI := "/" + bucket + "/" + key

	// SHA256 of the body
	payloadHash := sha256Hex(fileData)

	// Canonical headers
	canonicalHeaders := fmt.Sprintf("host:%s\nx-amz-content-sha256:%s\nx-amz-date:%s\n", host, payloadHash, amzDate)
	signedHeaders := "host;x-amz-content-sha256;x-amz-date"

	// Canonical request
	canonicalRequest := fmt.Sprintf("PUT\n%s\n\n%s\n%s\n%s", canonicalURI, canonicalHeaders, signedHeaders, payloadHash)

	// String to sign
	credentialScope := fmt.Sprintf("%s/%s/s3/aws4_request", dateStamp, region)
	stringToSign := fmt.Sprintf("AWS4-HMAC-SHA256\n%s\n%s\n%s", amzDate, credentialScope, sha256Hex([]byte(canonicalRequest)))

	// Signing key
	kDate := hmacSHA256([]byte("AWS4"+secretKey), dateStamp)
	kRegion := hmacSHA256(kDate, region)
	kService := hmacSHA256(kRegion, "s3")
	kSigning := hmacSHA256(kService, "aws4_request")

	signature := hex.EncodeToString(hmacSHA256(kSigning, stringToSign))

	authorization := fmt.Sprintf("AWS4-HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		accessKey, credentialScope, signedHeaders, signature)

	// Make the PUT request
	reqURL := endpointURL + canonicalURI
	req, err := http.NewRequest("PUT", reqURL, bytes.NewReader(fileData))
	if err != nil {
		return err
	}
	req.Header.Set("Host", host)
	req.Header.Set("X-Amz-Content-Sha256", payloadHash)
	req.Header.Set("X-Amz-Date", amzDate)
	req.Header.Set("Authorization", authorization)
	req.Header.Set("Content-Type", "audio/mpeg")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("S3 PUT returned %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// generatePresignedURL creates a presigned GET URL for S3.
func generatePresignedURL(endpoint, bucket, region, accessKey, secretKey, key, method string) (string, error) {
	now := time.Now().UTC()
	dateStamp := now.Format("20060102")
	amzDate := now.Format("20060102T150405Z")
	credentialScope := fmt.Sprintf("%s/%s/s3/aws4_request", dateStamp, region)
	credential := fmt.Sprintf("%s/%s", accessKey, credentialScope)

	endpointURL := strings.TrimRight(endpoint, "/")
	host := strings.TrimPrefix(strings.TrimPrefix(endpointURL, "https://"), "http://")
	canonicalURI := "/" + bucket + "/" + key

	queryParams := fmt.Sprintf("X-Amz-Algorithm=AWS4-HMAC-SHA256&X-Amz-Credential=%s&X-Amz-Date=%s&X-Amz-Expires=3600&X-Amz-SignedHeaders=host",
		strings.ReplaceAll(credential, "/", "%2F"), amzDate)

	canonicalHeaders := fmt.Sprintf("host:%s\n", host)
	signedHeaders := "host"

	canonicalRequest := fmt.Sprintf("%s\n%s\n%s\n%s\n%s\nUNSIGNED-PAYLOAD", method, canonicalURI, queryParams, canonicalHeaders, signedHeaders)

	stringToSign := fmt.Sprintf("AWS4-HMAC-SHA256\n%s\n%s\n%s", amzDate, credentialScope, sha256Hex([]byte(canonicalRequest)))

	kDate := hmacSHA256([]byte("AWS4"+secretKey), dateStamp)
	kRegion := hmacSHA256(kDate, region)
	kService := hmacSHA256(kRegion, "s3")
	kSigning := hmacSHA256(kService, "aws4_request")

	signature := hex.EncodeToString(hmacSHA256(kSigning, stringToSign))

	return fmt.Sprintf("%s%s?%s&X-Amz-Signature=%s", endpointURL, canonicalURI, queryParams, signature), nil
}

func sha256Hex(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

func hmacSHA256(key []byte, data string) []byte {
	h := hmac.New(sha256.New, key)
	h.Write([]byte(data))
	return h.Sum(nil)
}

func downloadImage(url string) ([]byte, string, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", err
	}

	mimeType := resp.Header.Get("Content-Type")
	if mimeType == "" {
		mimeType = "image/jpeg"
	}

	return data, mimeType, nil
}
