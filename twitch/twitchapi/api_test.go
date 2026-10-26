package twitchapi

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/julez-dev/chatuino/save"
	"github.com/stretchr/testify/require"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

type fakeAccountProvider struct{}

func (fakeAccountProvider) GetAccountBy(id string) (save.Account, error) {
	return save.Account{ID: id, AccessToken: "token"}, nil
}

func (fakeAccountProvider) UpdateTokensFor(string, string, string) error { return nil }

// TestAPI_ModeratorCommandContracts pins the method, path, query and body of the
// Helix calls behind the new moderator commands. The command tests fake this layer,
// so this is the test that catches a wrong verb, parameter or wrapper before Twitch
// does.
func TestAPI_ModeratorCommandContracts(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name     string
		method   string
		path     string
		query    string
		wantBody string
		response string
		status   int
		call     func(context.Context, *API) error
	}{
		{
			name: "update chat settings", method: http.MethodPatch, path: "/helix/chat/settings",
			query:    "broadcaster_id=channel-id&moderator_id=account-id",
			wantBody: `{"slow_mode":true,"slow_mode_wait_time":30}`,
			response: `{"data":[{"broadcaster_id":"channel-id"}]}`, status: http.StatusOK,
			call: func(ctx context.Context, api *API) error {
				_, err := api.UpdateChatSettings(ctx, "channel-id", "account-id", ChatSettingsUpdateData{
					SlowMode:         boolPointer(true),
					SlowModeWaitTime: intPointer(30),
				})
				return err
			},
		},
		{
			name: "get moderators", method: http.MethodGet, path: "/helix/moderation/moderators",
			query:    "broadcaster_id=channel-id&first=100&user_id=alice-id",
			response: `{"data":[]}`, status: http.StatusOK,
			call: func(ctx context.Context, api *API) error {
				_, err := api.GetModerators(ctx, "channel-id", []string{"alice-id"})
				return err
			},
		},
		{
			name: "add moderator", method: http.MethodPost, path: "/helix/moderation/moderators",
			query: "broadcaster_id=channel-id&user_id=alice-id", status: http.StatusNoContent,
			call: func(ctx context.Context, api *API) error {
				return api.AddChannelModerator(ctx, "channel-id", "alice-id")
			},
		},
		{
			name: "remove moderator", method: http.MethodDelete, path: "/helix/moderation/moderators",
			query: "broadcaster_id=channel-id&user_id=alice-id", status: http.StatusNoContent,
			call: func(ctx context.Context, api *API) error {
				return api.RemoveChannelModerator(ctx, "channel-id", "alice-id")
			},
		},
		{
			name: "get vips", method: http.MethodGet, path: "/helix/channels/vips",
			query: "broadcaster_id=channel-id&first=100", response: `{"data":[]}`, status: http.StatusOK,
			call: func(ctx context.Context, api *API) error {
				_, err := api.GetVIPs(ctx, "channel-id", nil)
				return err
			},
		},
		{
			name: "add vip", method: http.MethodPost, path: "/helix/channels/vips",
			query: "broadcaster_id=channel-id&user_id=alice-id", status: http.StatusNoContent,
			call: func(ctx context.Context, api *API) error {
				return api.AddChannelVIP(ctx, "channel-id", "alice-id")
			},
		},
		{
			name: "remove vip", method: http.MethodDelete, path: "/helix/channels/vips",
			query: "broadcaster_id=channel-id&user_id=alice-id", status: http.StatusNoContent,
			call: func(ctx context.Context, api *API) error {
				return api.RemoveChannelVIP(ctx, "channel-id", "alice-id")
			},
		},
		{
			name: "warn user", method: http.MethodPost, path: "/helix/moderation/warnings",
			query:    "broadcaster_id=channel-id&moderator_id=account-id",
			wantBody: `{"data":{"user_id":"alice-id","reason":"spam"}}`,
			response: `{"data":[{"user_id":"alice-id","reason":"spam"}]}`, status: http.StatusOK,
			call: func(ctx context.Context, api *API) error {
				return api.WarnChatUser(ctx, "channel-id", "account-id", "alice-id", "spam")
			},
		},
		{
			name: "start raid", method: http.MethodPost, path: "/helix/raids",
			query:    "from_broadcaster_id=channel-id&to_broadcaster_id=bob-id",
			response: `{"data":[{"created_at":"2026-01-02T03:04:05Z"}]}`, status: http.StatusOK,
			call: func(ctx context.Context, api *API) error {
				_, err := api.StartRaid(ctx, "channel-id", "bob-id")
				return err
			},
		},
		{
			name: "cancel raid", method: http.MethodDelete, path: "/helix/raids",
			query: "broadcaster_id=channel-id", status: http.StatusNoContent,
			call: func(ctx context.Context, api *API) error {
				return api.CancelRaid(ctx, "channel-id")
			},
		},
		{
			name: "send shoutout", method: http.MethodPost, path: "/helix/chat/shoutouts",
			query:  "from_broadcaster_id=channel-id&moderator_id=account-id&to_broadcaster_id=bob-id",
			status: http.StatusNoContent,
			call: func(ctx context.Context, api *API) error {
				return api.SendShoutout(ctx, "channel-id", "bob-id", "account-id")
			},
		},
		{
			name: "manage automod message", method: http.MethodPost, path: "/helix/moderation/automod/message",
			wantBody: `{"user_id":"account-id","msg_id":"msg-1","action":"ALLOW"}`,
			status:   http.StatusNoContent,
			call: func(ctx context.Context, api *API) error {
				return api.ManageAutoModMessage(ctx, "account-id", "msg-1", "ALLOW")
			},
		},
		{
			name: "snooze next ad", method: http.MethodPost, path: "/helix/channels/ads/schedule/snooze",
			query:    "broadcaster_id=channel-id",
			response: `{"data":[{"next_ad_at":"2026-01-02T03:04:05Z"}]}`, status: http.StatusOK,
			call: func(ctx context.Context, api *API) error {
				_, err := api.SnoozeNextAd(ctx, "channel-id")
				return err
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var (
				gotMethod string
				gotPath   string
				gotQuery  string
				gotBody   []byte
			)

			client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				gotMethod = req.Method
				gotPath = req.URL.Path
				gotQuery = req.URL.RawQuery
				if req.Body != nil {
					gotBody, _ = io.ReadAll(req.Body)
				}

				require.Equal(t, "Bearer token", req.Header.Get("Authorization"))
				require.Equal(t, "client-id", req.Header.Get("Client-Id"))

				body := tt.response
				if tt.status == http.StatusNoContent {
					body = ""
				}

				return &http.Response{
					StatusCode: tt.status,
					Header:     http.Header{"Content-Type": []string{"application/json"}},
					Body:       io.NopCloser(strings.NewReader(body)),
				}, nil
			})}

			api, err := NewAPI("client-id", WithHTTPClient(client), WithUserAuthentication(fakeAccountProvider{}, nil, "account-id"))
			require.NoError(t, err)

			require.NoError(t, tt.call(t.Context(), api))
			require.Equal(t, tt.method, gotMethod)
			require.Equal(t, tt.path, gotPath)
			require.Equal(t, tt.query, gotQuery)
			if tt.wantBody != "" {
				require.JSONEq(t, tt.wantBody, string(gotBody))
			} else {
				require.Empty(t, gotBody)
			}
		})
	}
}

func boolPointer(value bool) *bool { return &value }
func intPointer(value int) *int    { return &value }

func TestAPI_UnbanRequestContracts(t *testing.T) {
	t.Parallel()

	t.Run("fetch pending and resolved pages", func(t *testing.T) {
		t.Parallel()

		var (
			seenMu     sync.Mutex
			seenStatus = map[string]int{}
		)
		client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			require.Equal(t, http.MethodGet, req.Method)
			require.Equal(t, "/helix/moderation/unban_requests", req.URL.Path)
			require.Equal(t, "channel-id", req.URL.Query().Get("broadcaster_id"))
			require.Equal(t, "account-id", req.URL.Query().Get("moderator_id"))
			require.Equal(t, "100", req.URL.Query().Get("first"))
			seenMu.Lock()
			seenStatus[req.URL.Query().Get("status")]++
			seenMu.Unlock()

			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(`{"data":[],"pagination":{}}`)),
			}, nil
		})}

		api, err := NewAPI("client-id", WithHTTPClient(client), WithUserAuthentication(fakeAccountProvider{}, nil, "account-id"))
		require.NoError(t, err)

		requests, err := api.FetchUnbanRequests(t.Context(), "channel-id", "account-id")
		require.NoError(t, err)
		require.Empty(t, requests)
		require.Equal(t, map[string]int{
			"pending": 1, "approved": 1, "denied": 1, "acknowledged": 1, "canceled": 1,
		}, seenStatus)
	})

	t.Run("resolve with optional text", func(t *testing.T) {
		t.Parallel()

		client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			require.Equal(t, http.MethodPatch, req.Method)
			require.Equal(t, "/helix/moderation/unban_requests", req.URL.Path)
			require.Equal(t, "channel-id", req.URL.Query().Get("broadcaster_id"))
			require.Equal(t, "account-id", req.URL.Query().Get("moderator_id"))
			require.Equal(t, "request-1", req.URL.Query().Get("unban_request_id"))
			require.Equal(t, "approved", req.URL.Query().Get("status"))
			require.Equal(t, "apology accepted", req.URL.Query().Get("resolution_text"))

			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(`{"data":[{"id":"request-1","status":"approved","resolution_text":"apology accepted"}]}`)),
			}, nil
		})}

		api, err := NewAPI("client-id", WithHTTPClient(client), WithUserAuthentication(fakeAccountProvider{}, nil, "account-id"))
		require.NoError(t, err)

		request, err := api.ResolveBanRequest(t.Context(), "channel-id", "account-id", "request-1", "approved", "apology accepted")
		require.NoError(t, err)
		require.Equal(t, "request-1", request.ID)
		require.Equal(t, "apology accepted", request.ResolutionText)
	})
}
