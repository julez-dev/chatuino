package twitch

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

const baseURL = "https://api.twitch.tv/helix"

type API struct {
	client *http.Client

	userAccessToken string
	clientID        string
}

func NewAPI(client *http.Client, userAccessToken, clientID string) *API {
	if client == nil {
		client = http.DefaultClient
	}

	return &API{
		client:          client,
		userAccessToken: userAccessToken,
		clientID:        clientID,
	}
}

func (a API) GetUsers(ctx context.Context, logins []string, ids []string) (UserResponse, error) {
	values := url.Values{}
	for _, login := range logins {
		values.Add("login", login)
	}

	for _, id := range ids {
		values.Add("id", id)
	}

	url := fmt.Sprintf("/users?%s", values.Encode())
	resp, err := doAuthenticatedRequest[UserResponse](ctx, a, http.MethodGet, url, nil)

	if err != nil {
		return UserResponse{}, err
	}

	return resp, nil
}

func (a API) GetStreamInfo(ctx context.Context, broadcastID []string) (GetStreamsResponse, error) {
	values := url.Values{}
	for _, id := range broadcastID {
		values.Add("user_id", id)
	}

	values.Add("type", "all")

	url := fmt.Sprintf("/streams?%s", values.Encode())

	resp, err := doAuthenticatedRequest[GetStreamsResponse](ctx, a, http.MethodGet, url, nil)
	if err != nil {
		return GetStreamsResponse{}, err
	}

	return resp, nil
}

func (a API) GetGlobalEmotes(ctx context.Context) (EmoteResponse, error) {
	resp, err := doAuthenticatedRequest[EmoteResponse](ctx, a, http.MethodGet, "/chat/emotes/global", nil)
	if err != nil {
		return EmoteResponse{}, err
	}

	return resp, nil
}

func (a API) GetChannelEmotes(ctx context.Context, broadcaster string) (EmoteResponse, error) {
	// /chat/emotes?broadcaster_id=141981764
	resp, err := doAuthenticatedRequest[EmoteResponse](ctx, a, http.MethodGet, "/chat/emotes?broadcaster_id="+broadcaster, nil)
	if err != nil {
		return EmoteResponse{}, err
	}

	return resp, nil
}

func doAuthenticatedRequest[T any](ctx context.Context, api API, method, url string, body io.Reader) (T, error) {
	var data T

	url = fmt.Sprintf("%s%s", baseURL, url)
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return data, err
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", api.userAccessToken))
	req.Header.Set("Client-Id", api.clientID)

	resp, err := api.client.Do(req)
	if err != nil {
		return data, err
	}

	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return data, err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var errResp APIError
		if err := json.Unmarshal(respBody, &errResp); err != nil {
			return data, err
		}

		return data, errResp
	}

	if err := json.Unmarshal(respBody, &data); err != nil {
		return data, err
	}

	return data, nil
}
