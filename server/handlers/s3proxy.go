package handlers

import (
	"io"
	"log"
	"net/http"
)

// S3Proxy forwards a request to an S3 presigned URL, adding CORS headers.
// This avoids CORS issues when the S3 server doesn't properly handle browser preflight.
// Usage: GET /api/s3proxy?url=<presigned-url>
//
// The target is validated and fetched through an SSRF-safe client. By default
// only public hosts are reachable; add local/private S3 servers (MinIO) to
// S3_ALLOWED_HOSTS. Link-local and cloud metadata addresses are always blocked.
func S3Proxy(w http.ResponseWriter, r *http.Request) {
	targetURL := r.URL.Query().Get("url")
	if targetURL == "" {
		http.Error(w, `{"error":"missing url param"}`, http.StatusBadRequest)
		return
	}

	target, err := validateProxyTarget(targetURL)
	if err != nil {
		http.Error(w, `{"error":"invalid url"}`, http.StatusBadRequest)
		return
	}

	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, target.String(), nil)
	if err != nil {
		http.Error(w, `{"error":"invalid url"}`, http.StatusBadRequest)
		return
	}

	// Forward byte-range headers (seeking, partial metadata reads).
	if value := r.Header.Get("Range"); value != "" {
		req.Header.Set("Range", value)
	}
	if value := r.Header.Get("If-Range"); value != "" {
		req.Header.Set("If-Range", value)
	}

	resp, err := getS3HTTPClient().Do(req)
	if err != nil {
		log.Printf("s3proxy: upstream request failed: %v", err)
		http.Error(w, `{"error":"upstream request failed"}`, http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// Never proxy redirects: that would let an allowed host bounce us elsewhere.
	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		log.Printf("s3proxy: refusing upstream redirect to %q", resp.Header.Get("Location"))
		http.Error(w, `{"error":"upstream redirect refused"}`, http.StatusBadGateway)
		return
	}

	for _, header := range []string{
		"Content-Type",
		"Content-Length",
		"Content-Range",
		"Accept-Ranges",
		"ETag",
		"Last-Modified",
		"Cache-Control",
	} {
		if value := resp.Header.Get(header); value != "" {
			w.Header().Set(header, value)
		}
	}

	w.WriteHeader(resp.StatusCode)
	if _, err := io.Copy(w, resp.Body); err != nil {
		log.Printf("s3proxy: copy response: %v", err)
	}
}
