package ivr

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

const baseURL = "https://api.ivr.fi/v2"

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

// https://api.ivr.fi/v2/twitch/subage/{user}/{channel}
func (a API) GetSubAge(ctx context.Context, user, channel string) (SubAgeResponse, error) {
	resp, err := doRequest[SubAgeResponse](ctx, a, http.MethodGet, "/twitch/subage/"+user+"/"+channel, nil)
	if err != nil {
		return SubAgeResponse{}, err
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
		return data, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	if err := json.Unmarshal(respBody, &data); err != nil {
		return data, err
	}

	return data, nil
}
