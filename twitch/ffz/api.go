package ffz

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
)

const baseURL = "https://api.frankerfacez.com/v1"

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

// GetChannelEmotes fetches FFZ emotes for a channel by Twitch user ID.
// Returns a flat slice of all emotes across all sets for the channel.
func (a API) GetChannelEmotes(ctx context.Context, channelID string) ([]Emote, error) {
	resp, err := doRequest[channelResponse](ctx, a, http.MethodGet, "/room/id/"+channelID, nil)
	if err != nil {
		return nil, err
	}

	return collectEmotes(resp.Sets), nil
}

// GetGlobalEmotes fetches FFZ global emotes.
// Returns a flat slice of emotes from all default sets.
func (a API) GetGlobalEmotes(ctx context.Context) ([]Emote, error) {
	resp, err := doRequest[globalResponse](ctx, a, http.MethodGet, "/set/global", nil)
	if err != nil {
		return nil, err
	}

	defaultSets := make(map[int]struct{}, len(resp.DefaultSets))
	for _, id := range resp.DefaultSets {
		defaultSets[id] = struct{}{}
	}

	var emotes []Emote
	for idStr, set := range resp.Sets {
		id, err := strconv.Atoi(idStr)
		if err != nil {
			continue
		}
		if _, ok := defaultSets[id]; !ok {
			continue
		}
		emotes = append(emotes, set.Emoticons...)
	}

	return emotes, nil
}

// collectEmotes flattens all emote sets into a single slice.
func collectEmotes(sets map[string]emoteSet) []Emote {
	var emotes []Emote
	for _, set := range sets {
		emotes = append(emotes, set.Emoticons...)
	}
	return emotes
}

func doRequest[T any](ctx context.Context, api API, method, url string, body io.Reader) (T, error) {
	var data T

	url = baseURL + url

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
