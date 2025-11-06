package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"

	"github.com/julez-dev/chatuino/twitch"
)

type Client struct {
	httpClient *http.Client
	baseURL    string
	userInfos  *sync.Map
}

func NewClient(baseURL string, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	return &Client{
		baseURL:    baseURL,
		httpClient: httpClient,
		userInfos:  &sync.Map{},
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

func (c *Client) GetGlobalEmotes(ctx context.Context) (twitch.EmoteResponse, error) {
	return do[twitch.EmoteResponse](ctx, c, c.baseURL+"/ttv/emotes/global")
}

func (c *Client) GetChannelEmotes(ctx context.Context, broadcaster string) (twitch.EmoteResponse, error) {
	return do[twitch.EmoteResponse](ctx, c, c.baseURL+"/ttv/channel/"+broadcaster+"/emotes")
}

func (c *Client) GetStreamInfo(ctx context.Context, broadcastID []string) (twitch.GetStreamsResponse, error) {
	if len(broadcastID) == 0 {
		return twitch.GetStreamsResponse{}, fmt.Errorf("expected at least one broadcast id")
	}

	if len(broadcastID) == 1 {
		if v, ok := c.userInfos.Load(broadcastID[0]); ok {
			return v.(twitch.GetStreamsResponse), nil
		}

		resp, err := do[twitch.GetStreamsResponse](ctx, c, c.baseURL+"/ttv/channel/"+broadcastID[0]+"/info")
		if err != nil {
			c.userInfos.Delete(broadcastID[0])
			return twitch.GetStreamsResponse{}, err
		}

		c.userInfos.Swap(broadcastID[0], resp)
		return resp, nil
	}

	userValues := url.Values{}
	for _, login := range broadcastID {
		userValues.Add("user_id", login)
	}

	return do[twitch.GetStreamsResponse](ctx, c, c.baseURL+"/ttv/channels/info?"+userValues.Encode())
}

func (c *Client) GetUsers(ctx context.Context, logins []string, ids []string) (twitch.UserResponse, error) {
	if len(logins) == 0 {
		return twitch.UserResponse{}, fmt.Errorf("expected at least one login")
	}

	if len(logins) == 1 {
		return do[twitch.UserResponse](ctx, c, c.baseURL+"/ttv/channel/"+logins[0]+"/user")
	}

	userValues := url.Values{}
	for _, login := range logins {
		userValues.Add("logins", login)
	}

	return do[twitch.UserResponse](ctx, c, c.baseURL+"/ttv/channels?"+userValues.Encode())
}

func (c *Client) GetChatSettings(ctx context.Context, broadcasterID string, moderatorID string) (twitch.GetChatSettingsResponse, error) {
	return do[twitch.GetChatSettingsResponse](ctx, c, c.baseURL+"/ttv/channel/"+broadcasterID+"/chat/settings")
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
