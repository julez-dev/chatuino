package bttv

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

const baseURL = "https://api.betterttv.net/3"

type API struct {
	client *http.Client
}

func NewAPI(client *http.Client) *API {
	if client == nil {
		client = http.DefaultClient
	}

	return &API{
		client: client,
	}
}

// https://api.betterttv.net/3/cached/users/twitch/22484632
func (a API) GetChannelEmotes(ctx context.Context, channelID string) (UserResponse, error) {
	resp, err := doRequest[UserResponse](ctx, a, http.MethodGet, "/cached/users/twitch/"+channelID, nil)
	if err != nil {
		return UserResponse{}, err
	}

	return resp, nil
}

func (a API) GetGlobalEmotes(ctx context.Context) (GlobalEmoteResponse, error) {
	resp, err := doRequest[GlobalEmoteResponse](ctx, a, http.MethodGet, "/cached/emotes/global", nil)
	if err != nil {
		return GlobalEmoteResponse{}, err
	}

	return resp, nil
}

func doRequest[T any](ctx context.Context, api API, method, url string, body io.Reader) (T, error) {
	var data T

	url = fmt.Sprintf("%s%s", baseURL, url)

	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return data, err
	}

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

		errResp.StatusCode = resp.StatusCode
		errResp.Status = resp.Status

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
