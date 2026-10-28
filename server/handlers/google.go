package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
)

const (
	googleAuthURL = "https://accounts.google.com/o/oauth2/v2/auth"
	googleUserInfo = "https://www.googleapis.com/oauth2/v2/userinfo"
)

// googleTokenURL is a var so tests can point it at a stub server.
var googleTokenURL = "https://oauth2.googleapis.com/token"

// GoogleLogin redirects user to Google's OAuth consent page.
func GoogleLogin(w http.ResponseWriter, r *http.Request) {
	clientID := os.Getenv("GOOGLE_CLIENT_ID")
	redirectURI := os.Getenv("GOOGLE_REDIRECT_URI")

	if clientID == "" || redirectURI == "" {
		http.Error(w, `{"error":"Google SSO not configured"}`, http.StatusServiceUnavailable)
		return
	}

	scopes := "openid email profile https://www.googleapis.com/auth/drive.readonly"

	params := url.Values{
		"client_id":     {clientID},
		"response_type": {"code"},
		"redirect_uri":  {redirectURI},
		"scope":         {scopes},
		// offline is required to receive a refresh_token; consent makes sure
		// Google actually returns one instead of omitting it on later logins.
		"access_type": {"offline"},
		"prompt":      {"select_account consent"},
	}

	http.Redirect(w, r, googleAuthURL+"?"+params.Encode(), http.StatusTemporaryRedirect)
}

// GoogleCallback exchanges the auth code and returns user info to the frontend.
func GoogleCallback(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	if code == "" {
		http.Error(w, `{"error":"missing code"}`, http.StatusBadRequest)
		return
	}

	clientID := os.Getenv("GOOGLE_CLIENT_ID")
	clientSecret := os.Getenv("GOOGLE_CLIENT_SECRET")
	redirectURI := os.Getenv("GOOGLE_REDIRECT_URI")
	frontendOrigin := os.Getenv("FRONTEND_ORIGIN")
	if frontendOrigin == "" {
		frontendOrigin = "http://localhost:9000"
	}

	// Exchange code for token
	data := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {redirectURI},
		"client_id":     {clientID},
		"client_secret": {clientSecret},
	}

	resp, err := http.DefaultClient.PostForm(googleTokenURL, data)
	if err != nil {
		http.Error(w, `{"error":"token exchange failed"}`, http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		http.Error(w, fmt.Sprintf(`{"error":"google returned %d"}`, resp.StatusCode), http.StatusBadGateway)
		return
	}

	var tokenResp struct {
		AccessToken  string `json:"access_token"`
		IDToken      string `json:"id_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
	}
	json.Unmarshal(body, &tokenResp)

	// Fetch user profile
	req, _ := http.NewRequestWithContext(r.Context(), "GET", googleUserInfo, nil)
	req.Header.Set("Authorization", "Bearer "+tokenResp.AccessToken)

	userResp, err := http.DefaultClient.Do(req)
	if err != nil {
		http.Error(w, `{"error":"failed to fetch user info"}`, http.StatusBadGateway)
		return
	}
	defer userResp.Body.Close()

	var userInfo struct {
		ID      string `json:"id"`
		Email   string `json:"email"`
		Name    string `json:"name"`
		Picture string `json:"picture"`
	}
	userBody, _ := io.ReadAll(userResp.Body)
	json.Unmarshal(userBody, &userInfo)
	if userInfo.ID == "" {
		http.Error(w, `{"error":"failed to fetch user info"}`, http.StatusBadGateway)
		return
	}

	// Keep the refresh token server-side and hand the frontend an opaque,
	// long-lived handle instead of the 1-hour access token.
	token, err := createGoogleSession(userInfo.ID, userInfo.Email, tokenResp.AccessToken, tokenResp.RefreshToken, tokenResp.ExpiresIn)
	if err != nil {
		http.Error(w, `{"error":"could not store google session"}`, http.StatusBadGateway)
		return
	}

	// Redirect back to frontend with user info
	params := url.Values{
		"email":   {userInfo.Email},
		"name":    {userInfo.Name},
		"picture": {userInfo.Picture},
		"token":   {token},
	}

	redirectURL := fmt.Sprintf("%s/#/google-callback?%s", frontendOrigin, params.Encode())
	http.Redirect(w, r, redirectURL, http.StatusTemporaryRedirect)
}

// Helper to extract bearer token from header
func extractBearer(header string) string {
	if strings.HasPrefix(header, "Bearer ") {
		return header[7:]
	}
	return header
}
