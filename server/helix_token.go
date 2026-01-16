package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
)

const defaultTokenURL = "https://id.twitch.tv/oauth2/token"

// HelixTokenProvider manages app access tokens for the Twitch Helix API.
// It handles token creation and caching using the client credentials flow.
type HelixTokenProvider struct {
	client       *http.Client
	clientID     string
	clientSecret string
	tokenURL     string // configurable for testing

	mu    sync.Mutex
	token string
}

// NewHelixTokenProvider creates a new Helix token provider.
func NewHelixTokenProvider(client *http.Client, clientID, clientSecret string) *HelixTokenProvider {
	return &HelixTokenProvider{
		client:       client,
		clientID:     clientID,
		clientSecret: clientSecret,
		tokenURL:     defaultTokenURL,
	}
}

// EnsureToken returns a valid app access token, creating one if necessary.
// This method may perform HTTP requests to fetch a new token.
func (p *HelixTokenProvider) EnsureToken(ctx context.Context) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.token != "" {
		return p.token, nil
	}

	token, err := p.createToken(ctx)
	if err != nil {
		return "", err
	}

	p.token = token
	return token, nil
}

// InvalidateToken clears the cached token, forcing a refresh on next GetToken call.
func (p *HelixTokenProvider) InvalidateToken() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.token = ""
}

// createToken requests a new app access token from Twitch using client credentials flow.
func (p *HelixTokenProvider) createToken(ctx context.Context) (string, error) {
	formVal := url.Values{}
	formVal.Set("client_id", p.clientID)
	formVal.Set("client_secret", p.clientSecret)
	formVal.Set("grant_type", "client_credentials")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.tokenURL, strings.NewReader(formVal.Encode()))
	if err != nil {
		return "", fmt.Errorf("create token request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := p.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("token request failed: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read token response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("token request returned status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
	}

	if err := json.Unmarshal(bodyBytes, &tokenResp); err != nil {
		return "", fmt.Errorf("unmarshal token response: %w", err)
	}

	return tokenResp.AccessToken, nil
}
