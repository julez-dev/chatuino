package seventv

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

const baseURL = "https://7tv.io/v3"

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

// https://7tv.io/v3/users/twitch/71092938
func (a API) GetChannelEmotes(ctx context.Context, channelID string) (ChannelEmoteResponse, error) {
	resp, err := doRequest[ChannelEmoteResponse](ctx, a, http.MethodGet, "/users/twitch/"+channelID, nil)
	if err != nil {
		return ChannelEmoteResponse{}, err
	}

	return resp, nil
}

func (a API) GetGlobalEmotes(ctx context.Context) (EmoteResponse, error) {
	resp, err := doRequest[EmoteResponse](ctx, a, http.MethodGet, "/emote-sets/global", nil)
	if err != nil {
		return EmoteResponse{}, err
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
		errResp.Status = resp.Status
		errResp.StatusCode = resp.StatusCode
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
