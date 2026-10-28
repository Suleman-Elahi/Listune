package handlers

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func withTempTokenDir(t *testing.T) {
	t.Helper()
	old := tokenDir
	// Not pre-created: saveSession must create it with restrictive permissions.
	tokenDir = filepath.Join(t.TempDir(), "gtokens")
	t.Cleanup(func() { tokenDir = old })
}

func withGoogleOAuthConfigured(t *testing.T) {
	t.Helper()
	t.Setenv("GOOGLE_CLIENT_ID", "client-id")
	t.Setenv("GOOGLE_CLIENT_SECRET", "client-secret")
}

func stubTokenEndpoint(t *testing.T, body string) {
	t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	old := googleTokenURL
	googleTokenURL = ts.URL
	t.Cleanup(func() {
		googleTokenURL = old
		ts.Close()
	})
}

func handleKey(t *testing.T, handle string) string {
	t.Helper()
	key, _, found := strings.Cut(handle, ".")
	if !found {
		t.Fatalf("malformed handle %q", handle)
	}
	return key
}

func TestSessionResolveReturnsStoredAccessToken(t *testing.T) {
	withTempTokenDir(t)

	handle, err := createGoogleSession("user-1", "a@b.c", "access-1", "refresh-1", 3600)
	if err != nil {
		t.Fatalf("createGoogleSession: %v", err)
	}
	if handle == "access-1" {
		t.Fatal("frontend must not receive the raw access token")
	}

	got, err := resolveAccessToken(context.Background(), handle)
	if err != nil {
		t.Fatalf("resolveAccessToken: %v", err)
	}
	if got != "access-1" {
		t.Fatalf("got %q, want access-1", got)
	}
}

func TestExpiredSessionIsRefreshedAndPersisted(t *testing.T) {
	withTempTokenDir(t)
	withGoogleOAuthConfigured(t)
	stubTokenEndpoint(t, `{"access_token":"access-2","expires_in":3600}`)

	handle, err := createGoogleSession("user-1", "a@b.c", "access-1", "refresh-1", 3600)
	if err != nil {
		t.Fatalf("createGoogleSession: %v", err)
	}

	key := handleKey(t, handle)
	rec, err := loadSession(key)
	if err != nil {
		t.Fatalf("loadSession: %v", err)
	}
	rec.Expiry = time.Now().Add(-time.Minute)
	if err := saveSession(key, rec); err != nil {
		t.Fatalf("saveSession: %v", err)
	}

	got, err := resolveAccessToken(context.Background(), handle)
	if err != nil {
		t.Fatalf("resolveAccessToken: %v", err)
	}
	if got != "access-2" {
		t.Fatalf("got %q, want refreshed access-2", got)
	}

	stored, err := loadSession(key)
	if err != nil {
		t.Fatalf("loadSession: %v", err)
	}
	if stored.AccessToken != "access-2" {
		t.Fatalf("refreshed token not persisted: %q", stored.AccessToken)
	}
	if stored.RefreshToken != "refresh-1" {
		t.Fatalf("refresh token must survive a refresh: %q", stored.RefreshToken)
	}
}

func TestRefreshTokenIsKeptWhenGoogleOmitsIt(t *testing.T) {
	withTempTokenDir(t)

	if _, err := createGoogleSession("user-1", "a@b.c", "access-1", "refresh-1", 3600); err != nil {
		t.Fatalf("first login: %v", err)
	}
	// Second consent: Google returns no refresh_token.
	handle, err := createGoogleSession("user-1", "a@b.c", "access-9", "", 3600)
	if err != nil {
		t.Fatalf("second login: %v", err)
	}

	rec, err := loadSession(handleKey(t, handle))
	if err != nil {
		t.Fatalf("loadSession: %v", err)
	}
	if rec.RefreshToken != "refresh-1" {
		t.Fatalf("refresh token lost: %q", rec.RefreshToken)
	}
}

func TestInvalidRefreshTokenMeansSigninAgain(t *testing.T) {
	withTempTokenDir(t)
	withGoogleOAuthConfigured(t)
	stubTokenEndpoint(t, `{"error":"invalid_grant","error_description":"Token has been revoked."}`)

	handle, err := createGoogleSession("user-1", "a@b.c", "access-1", "refresh-1", 3600)
	if err != nil {
		t.Fatalf("createGoogleSession: %v", err)
	}
	key := handleKey(t, handle)
	rec, _ := loadSession(key)
	rec.Expiry = time.Now().Add(-time.Minute)
	_ = saveSession(key, rec)

	if _, err := resolveAccessToken(context.Background(), handle); !errors.Is(err, ErrNoGoogleSession) {
		t.Fatalf("got %v, want ErrNoGoogleSession", err)
	}
}

func TestUnknownOrWrongSecretIsRejected(t *testing.T) {
	withTempTokenDir(t)

	handle, err := createGoogleSession("user-1", "a@b.c", "access-1", "refresh-1", 3600)
	if err != nil {
		t.Fatalf("createGoogleSession: %v", err)
	}

	if _, err := resolveAccessToken(context.Background(), "deadbeefdeadbeefdeadbeef."+strings.Repeat("0", 48)); !errors.Is(err, ErrNoGoogleSession) {
		t.Fatalf("unknown key: got %v, want ErrNoGoogleSession", err)
	}
	key := handleKey(t, handle)
	if _, err := resolveAccessToken(context.Background(), key+"."+strings.Repeat("0", 48)); !errors.Is(err, ErrNoGoogleSession) {
		t.Fatalf("wrong secret: got %v, want ErrNoGoogleSession", err)
	}
	if _, err := resolveAccessToken(context.Background(), ""); !errors.Is(err, ErrNoGoogleSession) {
		t.Fatalf("empty token: got %v, want ErrNoGoogleSession", err)
	}
}

func TestLegacyRawAccessTokenPassesThrough(t *testing.T) {
	withTempTokenDir(t)

	const legacy = "ya29.a0AfNotARealTokenButLooksLikeOne"
	got, err := resolveAccessToken(context.Background(), legacy)
	if err != nil {
		t.Fatalf("resolveAccessToken: %v", err)
	}
	if got != legacy {
		t.Fatalf("got %q, want the legacy token unchanged", got)
	}
}

func TestSessionFilePathRejectsTraversal(t *testing.T) {
	withTempTokenDir(t)

	for _, key := range []string{"../evil", "../../etc/passwd", "short", "ZZZZZZZZZZZZZZZZZZZZZZZZ"} {
		if _, err := sessionFilePath(key); err == nil {
			t.Fatalf("sessionFilePath(%q) accepted a non-hex key", key)
		}
	}
}

func TestSaveSessionCreatesPrivateDirectory(t *testing.T) {
	withTempTokenDir(t)

	if _, err := createGoogleSession("user-1", "a@b.c", "access-1", "refresh-1", 3600); err != nil {
		t.Fatalf("createGoogleSession: %v", err)
	}
	info, err := os.Stat(tokenDir)
	if err != nil {
		t.Fatalf("stat tokenDir: %v", err)
	}
	if perm := info.Mode().Perm(); perm&0o077 != 0 {
		t.Fatalf("token dir permissions too open: %o", perm)
	}
}
