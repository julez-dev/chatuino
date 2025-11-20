package recentmessage

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/julez-dev/chatuino/twitch/twitchirc"
	"github.com/mailru/easyjson"
	"github.com/rs/zerolog/log"
)

const (
	baseURL    = "https://recent-messages.robotty.de/api/v2/recent-messages"
	messageCap = 100
)

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

func (a API) GetRecentMessagesFor(ctx context.Context, channelLogin string) ([]twitchirc.IRCer, error) {
	reqURL, err := url.JoinPath(baseURL, channelLogin)
	if err != nil {
		return nil, err
	}

	values := url.Values{}
	values.Set("hide_moderation_messages", "true")
	values.Set("hide_moderated_messages", "true")
	values.Set("limit", fmt.Sprintf("%d", messageCap))

	reqURL = fmt.Sprintf("%s?%s", reqURL, values.Encode())

	data, err := a.doRequest(ctx, a, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("could not fetch recent messages for %s: %w", channelLogin, err)
	}

	messages := make([]twitchirc.IRCer, 0, len(data.Messages))

	for _, message := range data.Messages {
		parsed, err := twitchirc.ParseIRC(message)
		if err != nil {
			log.Logger.Error().Err(err).Str("message", message).Msg("failed to parse message")
			continue
		}

		messages = append(messages, parsed)
	}

	return messages, nil
}

func (a API) doRequest(ctx context.Context, api API, method, url string, body io.Reader) (responseData, error) {
	var data responseData
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
		if err := json.Unmarshal(respBody, &errResp); err != nil {
			return data, err
		}

		return data, errResp
	}

	if err := easyjson.Unmarshal(respBody, &data); err != nil {
		return data, err
	}

	return data, nil
}
