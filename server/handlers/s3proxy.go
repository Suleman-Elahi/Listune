package handlers

import (
	"io"
	"net/http"
)

// S3Proxy forwards a request to an S3 presigned URL, adding CORS headers.
// This avoids CORS issues when the S3 server doesn't properly handle browser preflight.
// Usage: GET /api/s3proxy?url=<presigned-url>
func S3Proxy(w http.ResponseWriter, r *http.Request) {
	targetURL := r.URL.Query().Get("url")
	if targetURL == "" {
		http.Error(w, `{"error":"missing url param"}`, http.StatusBadRequest)
		return
	}

	// Create upstream request
	req, err := http.NewRequestWithContext(r.Context(), "GET", targetURL, nil)
	if err != nil {
		http.Error(w, `{"error":"invalid url"}`, http.StatusBadRequest)
		return
	}

	// Forward Range header if present (for partial reads)
	if rangeHeader := r.Header.Get("Range"); rangeHeader != "" {
		req.Header.Set("Range", rangeHeader)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		http.Error(w, `{"error":"upstream request failed"}`, http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// Copy relevant response headers
	if ct := resp.Header.Get("Content-Type"); ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	if cl := resp.Header.Get("Content-Length"); cl != "" {
		w.Header().Set("Content-Length", cl)
	}
	if cr := resp.Header.Get("Content-Range"); cr != "" {
		w.Header().Set("Content-Range", cr)
	}
	if ar := resp.Header.Get("Accept-Ranges"); ar != "" {
		w.Header().Set("Accept-Ranges", ar)
	}

	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}
