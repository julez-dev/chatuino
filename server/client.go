package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/julez-dev/chatuino/twitch/twitchapi"
)

type Client struct {
	httpClient *http.Client
	baseURL    string
}

func NewClient(baseURL string, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	return &Client{
		baseURL:    baseURL,
		httpClient: httpClient,
	}
}

func (c *Client) RefreshToken(ctx context.Context, refreshToken string) (string, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/auth/refresh", nil)
	if err != nil {
		return "", "", err
	}

	req.Header.Add("Authorization", "Bearer "+refreshToken)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", "", err
	}

	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", err
	}

	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("non 200 response code (%d): %s", resp.StatusCode, bodyBytes)
	}

	var tokenPair tokenPair
	if err := json.Unmarshal(bodyBytes, &tokenPair); err != nil {
		return "", "", err
	}

	return tokenPair.AccessToken, tokenPair.RefreshToken, nil
}

func (c *Client) RevokeToken(ctx context.Context, token string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/auth/revoke", nil)
	if err != nil {
		return err
	}

	req.Header.Add("Authorization", "Bearer "+token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}

	defer resp.Body.Close()

	// for revoke endpoint, BadRequest just means that the token was invalid (most likely expired, we can ignore this)
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusBadRequest {
		return fmt.Errorf("non 200 response code (%d)", resp.StatusCode)
	}

	return nil
}

func (c *Client) GetGlobalEmotes(ctx context.Context) (twitchapi.EmoteResponse, error) {
	return do[twitchapi.EmoteResponse](ctx, c, c.baseURL+"/ttv/emotes/global")
}

func (c *Client) GetChannelEmotes(ctx context.Context, broadcaster string) (twitchapi.EmoteResponse, error) {
	return do[twitchapi.EmoteResponse](ctx, c, c.baseURL+"/ttv/channel/"+broadcaster+"/emotes")
}

func (c *Client) GetStreamInfo(ctx context.Context, broadcastID []string) (twitchapi.GetStreamsResponse, error) {
	if len(broadcastID) == 0 {
		return twitchapi.GetStreamsResponse{}, fmt.Errorf("expected at least one broadcast id")
	}

	if len(broadcastID) == 1 {
		resp, err := do[twitchapi.GetStreamsResponse](ctx, c, c.baseURL+"/ttv/channel/"+broadcastID[0]+"/info")
		if err != nil {
			return twitchapi.GetStreamsResponse{}, err
		}

		return resp, nil
	}

	userValues := url.Values{}
	for _, login := range broadcastID {
		userValues.Add("user_id", login)
	}

	return do[twitchapi.GetStreamsResponse](ctx, c, c.baseURL+"/ttv/channels/info?"+userValues.Encode())
}

func (c *Client) GetUsers(ctx context.Context, logins []string, ids []string) (twitchapi.UserResponse, error) {
	if len(logins) == 0 && len(ids) == 0 {
		return twitchapi.UserResponse{}, fmt.Errorf("expected at least one login or id")
	}

	if len(logins) == 1 && len(ids) == 0 {
		return do[twitchapi.UserResponse](ctx, c, c.baseURL+"/ttv/channel/"+logins[0]+"/user")
	}

	userValues := url.Values{}
	for _, login := range logins {
		userValues.Add("logins", login)
	}
	for _, id := range ids {
		userValues.Add("ids", id)
	}

	return do[twitchapi.UserResponse](ctx, c, c.baseURL+"/ttv/channels?"+userValues.Encode())
}

func (c *Client) GetChatSettings(ctx context.Context, broadcasterID string, moderatorID string) (twitchapi.GetChatSettingsResponse, error) {
	return do[twitchapi.GetChatSettingsResponse](ctx, c, c.baseURL+"/ttv/channel/"+broadcasterID+"/chat/settings")
}

func (c *Client) GetGlobalChatBadges(ctx context.Context) ([]twitchapi.BadgeSet, error) {
	return do[[]twitchapi.BadgeSet](ctx, c, c.baseURL+"/ttv/chat/badges/global")
}

func (c *Client) GetChannelChatBadges(ctx context.Context, broadcasterID string) ([]twitchapi.BadgeSet, error) {
	return do[[]twitchapi.BadgeSet](ctx, c, c.baseURL+"/ttv/channel/"+broadcasterID+"/chat/badges")
}

type CheckLinkResponse struct {
	RemoteStatusCode  int
	RemoteContentType string
	VisitedURLs       []string
}

func (c *Client) CheckLink(ctx context.Context, targetURL string) (CheckLinkResponse, error) {
	u := fmt.Sprintf("%s/proxy/link_check?target=%s", c.baseURL, url.QueryEscape(targetURL))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return CheckLinkResponse{}, fmt.Errorf("failed to create req for: %s: %w", u, err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return CheckLinkResponse{}, fmt.Errorf("failed to send request: %w", err)
	}

	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		return CheckLinkResponse{}, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	code := resp.Header.Get("X-Remote-Status-Code")
	parsedStatusCode, err := strconv.Atoi(code)
	if err != nil {
		return CheckLinkResponse{}, fmt.Errorf("failed to parse remote status code (%s): %w", code, err)
	}

	data := CheckLinkResponse{
		RemoteStatusCode:  parsedStatusCode,
		RemoteContentType: resp.Header.Get("X-Remote-Content-Type"),
	}

	for u := range strings.SplitSeq(resp.Header.Get("X-Visited-Urls"), ",") {
		if u == "" {
			continue
		}

		data.VisitedURLs = append(data.VisitedURLs, u)
	}

	return data, nil
}

func do[T any](ctx context.Context, client *Client, url string) (T, error) {
	var respData T

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return respData, err
	}

	resp, err := client.httpClient.Do(req)
	if err != nil {
		return respData, err
	}

	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return respData, err
	}

	if resp.StatusCode != http.StatusOK {
		return respData, fmt.Errorf("non 200 response code (%d): %s", resp.StatusCode, string(bytes.Trim(bodyBytes, "\n")))
	}

	if err := json.Unmarshal(bodyBytes, &respData); err != nil {
		return respData, err
	}

	return respData, nil
}
