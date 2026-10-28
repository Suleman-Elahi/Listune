package handlers

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// tokenDir holds one JSON record per signed-in Google user. Records contain
// refresh tokens, so the directory must never be committed.
var tokenDir = "data/gtokens"

// ErrNoGoogleSession is returned when the caller has to sign in again.
var ErrNoGoogleSession = errors.New("Google session expired. Sign in again")

// errGoogleUnauthorized is returned when Drive rejects an access token we
// cannot refresh (a raw token stored before server-side sessions existed).
var errGoogleUnauthorized = errors.New("Google rejected the access token. Sign in again")

// tokenMu serialises read-modify-write of session records so concurrent
// requests cannot lose a refreshed access token.
var tokenMu sync.Mutex

// googleSession is the persisted OAuth state for one Google user.
type googleSession struct {
	UserID       string    `json:"userId"`
	Email        string    `json:"email"`
	Secret       string    `json:"secret"`
	AccessToken  string    `json:"accessToken"`
	RefreshToken string    `json:"refreshToken"`
	Expiry       time.Time `json:"expiry"`
}

// sessionKey derives a filesystem-safe, non-enumerable record name.
func sessionKey(userID string) string {
	sum := sha256.Sum256([]byte(userID))
	return hex.EncodeToString(sum[:])[:24]
}

func newSecret() (string, error) {
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func isHex(s string, length int) bool {
	if len(s) != length {
		return false
	}
	for _, r := range s {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return false
		}
	}
	return true
}

func sessionFilePath(key string) (string, error) {
	// key comes from the request; refuse anything that is not exactly the
	// digest prefix we generate ourselves (blocks path traversal).
	if !isHex(key, 24) {
		return "", errors.New("malformed session key")
	}
	return filepath.Join(tokenDir, key+".json"), nil
}

func loadSession(key string) (*googleSession, error) {
	path, err := sessionFilePath(key)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var s googleSession
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

func saveSession(key string, s *googleSession) error {
	path, err := sessionFilePath(key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.Marshal(s)
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// createGoogleSession stores the OAuth tokens server-side and returns the
// opaque handle the frontend keeps instead of a short-lived access token.
//
// Google only sends a refresh_token on the first consent, so an existing one
// is carried over when a later login omits it.
func createGoogleSession(userID, email, accessToken, refreshToken string, expiresIn int) (string, error) {
	if userID == "" || accessToken == "" {
		return "", errors.New("incomplete google token response")
	}

	tokenMu.Lock()
	defer tokenMu.Unlock()

	key := sessionKey(userID)
	existing, err := loadSession(key)
	if err == nil && existing.RefreshToken != "" && refreshToken == "" {
		refreshToken = existing.RefreshToken
	}
	if refreshToken == "" {
		return "", errors.New("no refresh token returned by google")
	}

	secret, err := newSecret()
	if err != nil {
		return "", err
	}
	if expiresIn <= 0 {
		expiresIn = 3600
	}
	s := &googleSession{
		UserID:       userID,
		Email:        email,
		Secret:       secret,
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		Expiry:       time.Now().Add(time.Duration(expiresIn) * time.Second),
	}
	if err := saveSession(key, s); err != nil {
		return "", err
	}
	return key + "." + secret, nil
}

// resolveAccessToken turns the handle supplied by the frontend into a
// currently-valid Google access token, refreshing it when needed. Raw access
// tokens stored before sessions existed are passed through untouched.
func resolveAccessToken(ctx context.Context, raw string) (string, error) {
	if raw == "" {
		return "", ErrNoGoogleSession
	}

	key, secret, found := strings.Cut(raw, ".")
	if !found || !isHex(key, 24) || !isHex(secret, 48) {
		// Legacy frontend payload: a raw Google access token.
		return raw, nil
	}

	tokenMu.Lock()
	defer tokenMu.Unlock()

	s, err := loadSession(key)
	if err != nil || s.Secret == "" || subtle.ConstantTimeCompare([]byte(s.Secret), []byte(secret)) != 1 {
		return "", ErrNoGoogleSession
	}
	if s.AccessToken != "" && time.Until(s.Expiry) > 90*time.Second {
		return s.AccessToken, nil
	}
	if s.RefreshToken == "" {
		return "", ErrNoGoogleSession
	}

	access, expiresIn, err := refreshGoogleToken(ctx, s.RefreshToken)
	if err != nil {
		return "", err
	}
	s.AccessToken = access
	s.Expiry = time.Now().Add(time.Duration(expiresIn) * time.Second)
	if err := saveSession(key, s); err != nil {
		return "", err
	}
	return access, nil
}

// refreshGoogleToken exchanges a refresh token for a new access token.
func refreshGoogleToken(ctx context.Context, refreshToken string) (string, int, error) {
	clientID := os.Getenv("GOOGLE_CLIENT_ID")
	clientSecret := os.Getenv("GOOGLE_CLIENT_SECRET")
	if clientID == "" || clientSecret == "" {
		return "", 0, errors.New("google OAuth not configured")
	}

	form := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
		"client_id":     {clientID},
		"client_secret": {clientSecret},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, googleTokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", 0, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", 0, fmt.Errorf("token refresh failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var out struct {
		AccessToken      string `json:"access_token"`
		ExpiresIn        int    `json:"expires_in"`
		Error            string `json:"error"`
		ErrorDescription string `json:"error_description"`
	}
	_ = json.Unmarshal(body, &out)

	switch out.Error {
	case "":
	case "invalid_grant":
		return "", 0, ErrNoGoogleSession
	default:
		return "", 0, fmt.Errorf("token refresh failed: %s %s", out.Error, out.ErrorDescription)
	}
	if out.AccessToken == "" {
		return "", 0, errors.New("token refresh returned no access token")
	}
	if out.ExpiresIn <= 0 {
		out.ExpiresIn = 3600
	}
	return out.AccessToken, out.ExpiresIn, nil
}

// writeDriveError maps authentication failures to 401 so the frontend can
// prompt for a new sign-in instead of showing a generic 502.
func writeDriveError(w http.ResponseWriter, err error) {
	if errors.Is(err, ErrNoGoogleSession) || errors.Is(err, errGoogleUnauthorized) {
		writeJSONError(w, http.StatusUnauthorized, err.Error())
		return
	}
	writeJSONError(w, http.StatusBadGateway, err.Error())
}
