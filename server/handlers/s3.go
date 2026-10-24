package handlers

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

// ── SigV4 helpers ────────────────────────────────────────────────────────────

func sha256Hex(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

func hmacSHA256(key []byte, data string) []byte {
	h := hmac.New(sha256.New, key)
	h.Write([]byte(data))
	return h.Sum(nil)
}

// awsEncode implements RFC 3986 percent-encoding as required by SigV4.
func awsEncode(value string) string {
	var b strings.Builder
	b.Grow(len(value))
	for i := 0; i < len(value); i++ {
		c := value[i]
		switch {
		case c >= 'A' && c <= 'Z', c >= 'a' && c <= 'z', c >= '0' && c <= '9',
			c == '-', c == '_', c == '.', c == '~':
			b.WriteByte(c)
		default:
			fmt.Fprintf(&b, "%%%02X", c)
		}
	}
	return b.String()
}

func encodeS3Key(key string) string {
	segments := strings.Split(key, "/")
	for i, segment := range segments {
		segments[i] = awsEncode(segment)
	}
	return strings.Join(segments, "/")
}

// s3Endpoint holds a normalized endpoint, including any path prefix it is mounted under.
type s3Endpoint struct {
	origin string // scheme://host
	prefix string // escaped path prefix, no trailing slash
	host   string // host[:port]
}

func parseS3Endpoint(endpoint string) (*s3Endpoint, error) {
	u, err := url.Parse(strings.TrimSpace(endpoint))
	if err != nil || u.Host == "" {
		return nil, fmt.Errorf("invalid S3 endpoint")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("S3 endpoint must use http or https")
	}
	if u.User != nil {
		return nil, fmt.Errorf("S3 endpoint must not contain credentials")
	}

	prefix := strings.TrimRight(u.EscapedPath(), "/")
	if prefix == "/" {
		prefix = ""
	}

	return &s3Endpoint{
		origin: u.Scheme + "://" + u.Host,
		prefix: prefix,
		host:   u.Host,
	}, nil
}

func canonicalS3URI(endpoint *s3Endpoint, bucket, key string) string {
	uri := endpoint.prefix + "/" + awsEncode(bucket)
	if key != "" {
		uri += "/" + encodeS3Key(key)
	}
	return uri
}

func signingKey(secretKey, dateStamp, region string) []byte {
	kDate := hmacSHA256([]byte("AWS4"+secretKey), dateStamp)
	kRegion := hmacSHA256(kDate, region)
	kService := hmacSHA256(kRegion, "s3")
	return hmacSHA256(kService, "aws4_request")
}

// generatePresignedURL creates a SigV4 query-authenticated URL.
func generatePresignedURL(endpoint, bucket, region, accessKey, secretKey, sessionToken, key, method string) (string, error) {
	parsed, err := parseS3Endpoint(endpoint)
	if err != nil {
		return "", err
	}

	now := time.Now().UTC()
	dateStamp := now.Format("20060102")
	amzDate := now.Format("20060102T150405Z")
	credentialScope := fmt.Sprintf("%s/%s/s3/aws4_request", dateStamp, region)
	credential := accessKey + "/" + credentialScope
	canonicalURI := canonicalS3URI(parsed, bucket, key)

	params := map[string]string{
		"X-Amz-Algorithm":     "AWS4-HMAC-SHA256",
		"X-Amz-Credential":    credential,
		"X-Amz-Date":          amzDate,
		"X-Amz-Expires":       "3600",
		"X-Amz-SignedHeaders": "host",
	}
	if sessionToken != "" {
		params["X-Amz-Security-Token"] = sessionToken
	}

	names := make([]string, 0, len(params))
	for name := range params {
		names = append(names, name)
	}
	sort.Strings(names)

	pairs := make([]string, 0, len(names))
	for _, name := range names {
		pairs = append(pairs, awsEncode(name)+"="+awsEncode(params[name]))
	}
	canonicalQuery := strings.Join(pairs, "&")

	canonicalHeaders := "host:" + parsed.host + "\n"
	signedHeaders := "host"
	canonicalRequest := strings.Join([]string{
		method,
		canonicalURI,
		canonicalQuery,
		canonicalHeaders,
		signedHeaders,
		"UNSIGNED-PAYLOAD",
	}, "\n")

	stringToSign := strings.Join([]string{
		"AWS4-HMAC-SHA256",
		amzDate,
		credentialScope,
		sha256Hex([]byte(canonicalRequest)),
	}, "\n")

	signature := hex.EncodeToString(hmacSHA256(signingKey(secretKey, dateStamp, region), stringToSign))

	return parsed.origin + canonicalURI + "?" + canonicalQuery + "&X-Amz-Signature=" + signature, nil
}

// ── SSRF-safe HTTP clients ───────────────────────────────────────────────────

func hostAllowlist() []string {
	raw := strings.TrimSpace(os.Getenv("S3_ALLOWED_HOSTS"))
	if raw == "" {
		return nil
	}
	return strings.Split(raw, ",")
}

func metadataAddress(ip net.IP) bool {
	return ip.Equal(net.ParseIP("169.254.169.254")) ||
		ip.Equal(net.ParseIP("fd00:ec2::254")) ||
		ip.Equal(net.ParseIP("100.100.100.200"))
}

func blockedIP(ip net.IP, blockPrivate bool) error {
	if ip.IsUnspecified() || ip.IsMulticast() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return fmt.Errorf("blocked address %s", ip)
	}
	if metadataAddress(ip) {
		return fmt.Errorf("blocked metadata address %s", ip)
	}
	if blockPrivate && (ip.IsPrivate() || ip.IsLoopback()) {
		return fmt.Errorf("blocked private address %s", ip)
	}
	return nil
}

// newSafeHTTPClient returns a client that resolves and dials IPs itself so that
// DNS rebinding cannot bypass the address checks. When allowedHosts is
// non-empty, only those hosts may be contacted.
func newSafeHTTPClient(allowedHosts []string, blockPrivate bool) *http.Client {
	allowed := make(map[string]bool, len(allowedHosts))
	for _, host := range allowedHosts {
		host = strings.ToLower(strings.TrimSpace(host))
		if host != "" {
			allowed[host] = true
		}
	}

	dialer := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}

	transport := &http.Transport{
		// Do not honour ambient proxy settings: they would bypass the host
		// allowlist because the dialer would only ever see the proxy address.
		Proxy: nil,
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, err
			}
			normalizedHost := strings.ToLower(strings.Trim(host, "[]"))

			if len(allowed) > 0 && !allowed[normalizedHost] && !allowed[strings.ToLower(addr)] {
				return nil, fmt.Errorf("host %q is not allowed (set S3_ALLOWED_HOSTS)", normalizedHost)
			}

			ips, err := net.DefaultResolver.LookupIP(ctx, "ip", normalizedHost)
			if err != nil {
				return nil, err
			}

			var lastErr error
			for _, ip := range ips {
				if err := blockedIP(ip, blockPrivate); err != nil {
					lastErr = err
					continue
				}
				conn, err := dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
				if err == nil {
					return conn, nil
				}
				lastErr = err
			}
			if lastErr == nil {
				lastErr = fmt.Errorf("no usable address for %s", normalizedHost)
			}
			return nil, lastErr
		},
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
		IdleConnTimeout:       90 * time.Second,
		MaxIdleConns:          100,
	}

	return &http.Client{
		Transport: transport,
		// No overall timeout: audio responses can be large and long-lived.
		// Dial/TLS/header timeouts above still prevent hung requests.
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

var (
	s3ClientOnce    sync.Once
	s3Client        *http.Client
	imageClientOnce sync.Once
	imageClient     *http.Client
)

// Clients are created lazily so that .env values loaded in main() are visible.
func getS3HTTPClient() *http.Client {
	s3ClientOnce.Do(func() {
		// Without an explicit allowlist, only public hosts may be reached.
		// Local/private S3 servers (MinIO) must be listed in S3_ALLOWED_HOSTS.
		allowlist := hostAllowlist()
		s3Client = newSafeHTTPClient(allowlist, len(allowlist) == 0)
	})
	return s3Client
}

func getImageHTTPClient() *http.Client {
	imageClientOnce.Do(func() {
		// Cover art should only ever come from the public internet.
		imageClient = newSafeHTTPClient(nil, true)
	})
	return imageClient
}

func validateProxyTarget(raw string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" {
		return nil, fmt.Errorf("invalid url")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("only http and https URLs are allowed")
	}
	if u.User != nil {
		return nil, fmt.Errorf("URLs with embedded credentials are not allowed")
	}
	return u, nil
}
