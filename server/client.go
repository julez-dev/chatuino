package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/julez-dev/chatuino/twitch"
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

	return do[twitch.GetStreamsResponse](ctx, c, c.baseURL+"/ttv/channel/"+broadcastID[0]+"/info")
}

func (c *Client) GetUsers(ctx context.Context, logins []string, ids []string) (twitch.UserResponse, error) {
	if len(logins) == 0 {
		return twitch.UserResponse{}, fmt.Errorf("expected at least one login")
	}

	return do[twitch.UserResponse](ctx, c, c.baseURL+"/ttv/channel/"+logins[0]+"/user")
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
		return respData, fmt.Errorf("non 200 response code (%d):  %s", resp.StatusCode, bodyBytes)
	}

	if err := json.Unmarshal(bodyBytes, &respData); err != nil {
		return respData, err
	}

	return respData, nil
}
