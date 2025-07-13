package twitch

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
	"golang.org/x/sync/errgroup"
	"resenje.org/singleflight"

	"github.com/julez-dev/chatuino/save"
)

var (
	ErrNoAuthProvided     = errors.New("one of app secret or user access token needs to be provided")
	ErrNoUserAccess       = errors.New("user endpoint called when no token was provided")
	ErrUserRefreshToken   = errors.New("the provided user refresh token is empty")
	ErrNoRefresher        = errors.New("refresher was not provided")
	ErrNoClientSecret     = errors.New("no app access token was provided")
	ErrAppTokenStatusCode = errors.New("invalid status code response while creating app access token")
)

const baseURL = "https://api.twitch.tv/helix"

type TokenRefresher interface {
	RefreshToken(ctx context.Context, refreshToken string) (string, string, error)
}

type AccountProvider interface {
	GetAccountBy(id string) (save.Account, error)
	UpdateTokensFor(id, accessToken, refreshToken string) error
}

type APIOptionFunc func(api *API) error

func WithHTTPClient(client *http.Client) APIOptionFunc {
	return func(api *API) error {
		api.client = client
		return nil
	}
}

func WithClientSecret(secret string) APIOptionFunc {
	return func(api *API) error {
		api.clientSecret = secret
		return nil
	}
}

func WithUserAuthentication(provider AccountProvider, refresher TokenRefresher, accountID string) APIOptionFunc {
	return func(api *API) error {
		api.refresher = refresher
		api.provider = provider
		api.accountID = accountID
		return nil
	}
}

type API struct {
	client *http.Client

	provider  AccountProvider
	refresher TokenRefresher
	accountID string

	m                   *sync.Mutex
	singleRefresh       *singleflight.Group[string, string]
	singleUserChatColor *singleflight.Group[string, []UserChatColor]
	singleUserBadge     *singleflight.Group[string, []ChannelChatBadges]

	appAccessToken string

	clientID     string
	clientSecret string
}

func NewAPI(clientID string, opts ...APIOptionFunc) (*API, error) {
	api := &API{
		clientID:            clientID,
		m:                   &sync.Mutex{},
		singleRefresh:       &singleflight.Group[string, string]{},
		singleUserBadge:     &singleflight.Group[string, []ChannelChatBadges]{},
		singleUserChatColor: &singleflight.Group[string, []UserChatColor]{},
	}

	for _, f := range opts {
		if err := f(api); err != nil {
			return nil, err
		}
	}

	if api.client == nil {
		api.client = http.DefaultClient
	}

	return api, nil
}

func (a *API) GetChannelChatBadges(ctx context.Context, broadcasterID string) ([]ChannelChatBadges, error) {
	if a.provider == nil {
		return nil, ErrNoUserAccess
	}

	values := url.Values{}
	values.Add("broadcaster_id", broadcasterID)

	url := fmt.Sprintf("/chat/badges?%s", values.Encode())

	data, _, err := a.singleUserBadge.Do(ctx, url, func(ctx context.Context) ([]ChannelChatBadges, error) {
		resp, err := doAuthenticatedUserRequest[GetChannelChatBadgesResp](ctx, a, http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}

		return resp.Data, nil
	})

	if err != nil {
		return nil, err
	}

	return data, nil
}

func (a *API) GetUserChatColor(ctx context.Context, userIDs []string) ([]UserChatColor, error) {
	if a.provider == nil {
		return nil, ErrNoUserAccess
	}

	values := url.Values{}
	for _, id := range userIDs {
		values.Add("user_id", id)
	}

	url := fmt.Sprintf("/chat/color?%s", values.Encode())

	data, _, err := a.singleUserChatColor.Do(ctx, url, func(ctx context.Context) ([]UserChatColor, error) {
		resp, err := doAuthenticatedUserRequest[GetUserChatColorResponse](ctx, a, http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}

		return resp.Data, nil
	})

	if err != nil {
		return nil, err
	}

	return data, nil
}

func (a *API) FetchAllUserEmotes(ctx context.Context, userID string, broadcasterID string) ([]UserEmoteImage, string, error) {
	if a.provider == nil {
		return nil, "", ErrNoUserAccess
	}

	emotes := []UserEmoteImage{}
	var (
		after    string
		template string
	)

	for {
		values := url.Values{}
		if broadcasterID != "" {
			values.Add("broadcaster_id", broadcasterID)
		}
		values.Add("user_id", userID)
		if after != "" {
			values.Add("after", after)
		}

		url := fmt.Sprintf("/chat/emotes/user?%s", values.Encode())

		resp, err := doAuthenticatedUserRequest[GetUserEmotesResponse](ctx, a, http.MethodGet, url, nil)
		if err != nil {
			return nil, "", err
		}

		emotes = append(emotes, resp.Data...)

		if resp.Pagination.Cursor == "" {
			break
		}

		template = resp.Template
		after = resp.Pagination.Cursor
	}

	return emotes, template, nil
}

func (a *API) BanUser(ctx context.Context, broadcasterID string, moderatorID string, data BanUserData) error {
	if a.provider == nil {
		return ErrNoUserAccess
	}

	values := url.Values{}
	values.Add("broadcaster_id", broadcasterID)
	values.Add("moderator_id", moderatorID)

	url := fmt.Sprintf("/moderation/bans?%s", values.Encode())

	body, err := json.Marshal(BanUserRequest{Data: data})
	if err != nil {
		return err
	}

	_, err = doAuthenticatedUserRequest[any](ctx, a, http.MethodPost, url, body)
	if err != nil {
		return err
	}

	return nil
}

func (a *API) UnbanUser(ctx context.Context, broadcasterID string, moderatorID string, userID string) error {
	if a.provider == nil {
		return ErrNoUserAccess
	}

	values := url.Values{}
	values.Add("broadcaster_id", broadcasterID)
	values.Add("moderator_id", moderatorID)
	values.Add("user_id", userID)

	url := fmt.Sprintf("/moderation/bans?%s", values.Encode())

	_, err := doAuthenticatedUserRequest[any](ctx, a, http.MethodDelete, url, nil)
	if err != nil {
		return err
	}

	return nil
}

func (a *API) CreateClip(ctx context.Context, broadcastID string, hasDelay bool) (CreatedClip, error) {
	if a.provider == nil {
		return CreatedClip{}, ErrNoUserAccess
	}

	values := url.Values{}
	values.Add("broadcaster_id", broadcastID)
	if hasDelay {
		values.Add("has_delay", "true")
	}

	url := fmt.Sprintf("/clips?%s", values.Encode())

	resp, err := doAuthenticatedUserRequest[CreateClipResponse](ctx, a, http.MethodPost, url, nil)
	if err != nil {
		return CreatedClip{}, err
	}

	return resp.Data[0], nil
}

func (a *API) FetchUserFollowedChannels(ctx context.Context, userID string, broadcasterID string) ([]FollowedChannel, error) {
	if a.provider == nil {
		return nil, ErrNoUserAccess
	}

	channels := []FollowedChannel{}
	var after string

	for {
		values := url.Values{}
		if broadcasterID != "" {
			values.Add("broadcaster_id", broadcasterID)
		}
		values.Add("user_id", userID)
		values.Add("first", "100")
		if after != "" {
			values.Add("after", after)
		}

		url := fmt.Sprintf("/channels/followed?%s", values.Encode())

		resp, err := doAuthenticatedUserRequest[GetFollowedChannelsResponse](ctx, a, http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}

		channels = append(channels, resp.Data...)

		if resp.Pagination.Cursor == "" {
			break
		}

		after = resp.Pagination.Cursor
	}

	return channels, nil
}

func (a *API) FetchUnbanRequests(ctx context.Context, broadcasterID, moderatorID string) ([]UnbanRequest, error) {
	if a.provider == nil {
		return nil, ErrNoUserAccess
	}

	// Fetch all unban requests for the broadcaster
	// For all statuses, handle each status in a separate goroutine
	// One status may have multiple pages, so we need to fetch all pages for each status
	allStatus := [...]string{"pending", "approved", "denied", "acknowledged", "canceled"}
	respChannel := make(chan UnbanRequest)
	wg, ctx := errgroup.WithContext(ctx)

	for _, status := range allStatus {
		// In theory, we don't need to make a local copy of status since loop variable behavior was changed in go 1.22
		// and the go.mod file requires at least 1.22, so let's find out :)
		wg.Go(func() error {
			var after string

			for {
				values := url.Values{}
				values.Add("broadcaster_id", broadcasterID)
				values.Add("moderator_id", moderatorID)
				values.Add("status", status)
				values.Add("first", "100")
				if after != "" {
					values.Add("after", after)
				}

				url := fmt.Sprintf("/moderation/unban_requests?%s", values.Encode())

				resp, err := doAuthenticatedUserRequest[GetUnbanRequestsResponse](ctx, a, http.MethodGet, url, nil)
				if err != nil {
					return err
				}

				for _, r := range resp.Data {
					respChannel <- r
				}

				if resp.Pagination.Cursor == "" {
					break
				}

				after = resp.Pagination.Cursor
			}

			return nil
		})
	}

	// This goroutine will close the channel once all requests are done
	// When an error occurs, it will close the channel immediately
	// which will unblock the main routine which then will call .Wait again to get the error
	// that canceled the goroutines, this way we don't need to pass the error around in the channel
	go func() {
		_ = wg.Wait()
		close(respChannel)
	}()

	var requests []UnbanRequest
	for r := range respChannel {
		requests = append(requests, r)
	}

	// If the goroutines returned an error, return it now
	if err := wg.Wait(); err != nil {
		return nil, err
	}

	return requests, nil
}

func (a *API) ResolveBanRequest(ctx context.Context, broadcasterID, moderatorID, requestID, status string) (UnbanRequest, error) {
	if a.provider == nil {
		return UnbanRequest{}, ErrNoUserAccess
	}

	values := url.Values{}
	values.Add("broadcaster_id", broadcasterID)
	values.Add("moderator_id", moderatorID)
	values.Add("unban_request_id", requestID)
	values.Add("status", status)

	url := fmt.Sprintf("/moderation/unban_requests?%s", values.Encode())

	resp, err := doAuthenticatedUserRequest[GetUnbanRequestsResponse](ctx, a, http.MethodPatch, url, nil)
	if err != nil {
		return UnbanRequest{}, err
	}

	return resp.Data[0], nil
}

func (a *API) GetUsers(ctx context.Context, logins []string, ids []string) (UserResponse, error) {
	values := url.Values{}
	for _, login := range logins {
		values.Add("login", login)
	}

	for _, id := range ids {
		values.Add("id", id)
	}

	var (
		resp UserResponse
		err  error
	)

	url := fmt.Sprintf("/users?%s", values.Encode())

	if a.provider != nil {
		resp, err = doAuthenticatedUserRequest[UserResponse](ctx, a, http.MethodGet, url, nil)
	} else {
		resp, err = doAuthenticatedAppRequest[UserResponse](ctx, a, http.MethodGet, url, nil)
	}

	if err != nil {
		return UserResponse{}, err
	}

	return resp, nil
}

func (a *API) GetStreamInfo(ctx context.Context, broadcastID []string) (GetStreamsResponse, error) {
	values := url.Values{}
	for _, id := range broadcastID {
		values.Add("user_id", id)
	}

	values.Add("type", "all")

	url := fmt.Sprintf("/streams?%s", values.Encode())

	var (
		resp GetStreamsResponse
		err  error
	)

	if a.provider != nil {
		resp, err = doAuthenticatedUserRequest[GetStreamsResponse](ctx, a, http.MethodGet, url, nil)
	} else {
		resp, err = doAuthenticatedAppRequest[GetStreamsResponse](ctx, a, http.MethodGet, url, nil)
	}
	if err != nil {
		return GetStreamsResponse{}, err
	}

	return resp, nil
}

func (a *API) CreateEventSubSubscription(ctx context.Context, reqData CreateEventSubSubscriptionRequest) (CreateEventSubSubscriptionResponse, error) {
	if a.provider == nil {
		return CreateEventSubSubscriptionResponse{}, ErrNoUserAccess
	}

	reqBytes, err := json.Marshal(reqData)
	if err != nil {
		return CreateEventSubSubscriptionResponse{}, err
	}

	resp, err := doAuthenticatedUserRequest[CreateEventSubSubscriptionResponse](ctx, a, http.MethodPost, "/eventsub/subscriptions", reqBytes)
	if err != nil {
		return CreateEventSubSubscriptionResponse{}, err
	}

	return resp, nil
}

// https://dev.twitch.tv/docs/api/reference/#get-eventsub-subscriptions
func (a *API) FetchEventSubSubscriptions(ctx context.Context, status, subType, userID string) ([]EventSubData, error) {
	if a.provider == nil {
		return nil, ErrNoUserAccess
	}

	subs := []EventSubData{}
	var after string

	for {
		values := url.Values{}
		values.Add("status", status)
		if subType != "" {
			values.Add("type", subType)
		}
		if userID != "" {
			values.Add("user_id", userID)
		}
		if after != "" {
			values.Add("after", after)
		}

		url := fmt.Sprintf("/eventsub/subscriptions?%s", values.Encode())

		resp, err := doAuthenticatedUserRequest[GetEventSubSubscriptionsResponse](ctx, a, http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}

		subs = append(subs, resp.Data...)

		if resp.Pagination.Cursor == "" {
			break
		}

		after = resp.Pagination.Cursor
	}

	return subs, nil
}

func (a *API) DeleteSubSubscriptions(ctx context.Context, id string) error {
	if a.provider == nil {
		return ErrNoUserAccess
	}

	values := url.Values{}
	values.Add("id", id)

	if _, err := doAuthenticatedUserRequest[any](ctx, a, http.MethodDelete, "/eventsub/subscriptions", nil); err != nil {
		return err
	}

	return nil
}

func (a *API) GetGlobalEmotes(ctx context.Context) (EmoteResponse, error) {
	var (
		resp EmoteResponse
		err  error
	)

	url := "/chat/emotes/global"

	if a.provider != nil {
		resp, err = doAuthenticatedUserRequest[EmoteResponse](ctx, a, http.MethodGet, url, nil)
	} else {
		resp, err = doAuthenticatedAppRequest[EmoteResponse](ctx, a, http.MethodGet, url, nil)
	}
	if err != nil {
		return EmoteResponse{}, err
	}

	return resp, nil
}

func (a *API) GetChannelEmotes(ctx context.Context, broadcaster string) (EmoteResponse, error) {
	var (
		resp EmoteResponse
		err  error
	)

	// /chat/emotes?broadcaster_id=141981764
	if a.provider != nil {
		resp, err = doAuthenticatedUserRequest[EmoteResponse](ctx, a, http.MethodGet, "/chat/emotes?broadcaster_id="+broadcaster, nil)
	} else {
		resp, err = doAuthenticatedAppRequest[EmoteResponse](ctx, a, http.MethodGet, "/chat/emotes?broadcaster_id="+broadcaster, nil)
	}

	if err != nil {
		return EmoteResponse{}, err
	}

	return resp, nil
}

// moderatorID needs to match ID of the user the token was generated for
func (a *API) GetChatSettings(ctx context.Context, broadcasterID string, moderatorID string) (GetChatSettingsResponse, error) {
	var (
		resp GetChatSettingsResponse
		err  error
	)

	values := url.Values{}
	values.Add("broadcaster_id", broadcasterID)
	if moderatorID != "" {
		values.Add("moderator_id", moderatorID)
	}

	url := fmt.Sprintf("/chat/settings?%s", values.Encode())

	if a.provider != nil {
		resp, err = doAuthenticatedUserRequest[GetChatSettingsResponse](ctx, a, http.MethodGet, url, nil)
	} else {
		resp, err = doAuthenticatedAppRequest[GetChatSettingsResponse](ctx, a, http.MethodGet, url, nil)
	}

	if err != nil {
		return GetChatSettingsResponse{}, err
	}

	return resp, nil
}

func (a *API) SendChatAnnouncement(ctx context.Context, broadcasterID string, moderatorID string, req CreateChatAnnouncementRequest) error {
	if a.provider == nil {
		return ErrNoUserAccess
	}

	values := url.Values{}
	values.Add("broadcaster_id", broadcasterID)
	values.Add("moderator_id", moderatorID)

	url := fmt.Sprintf("/chat/announcements?%s", values.Encode())

	reqBytes, err := json.Marshal(req)
	if err != nil {
		return err
	}

	_, err = doAuthenticatedUserRequest[struct{}](ctx, a, http.MethodPost, url, reqBytes)
	if err != nil {
		return err
	}

	return nil
}

func (a *API) CreateStreamMarker(ctx context.Context, req CreateStreamMarkerRequest) (StreamMarker, error) {
	if a.provider == nil {
		return StreamMarker{}, ErrNoUserAccess
	}

	reqBytes, err := json.Marshal(req)
	if err != nil {
		return StreamMarker{}, err
	}

	resp, err := doAuthenticatedUserRequest[CreateStreamMarkerResponse](ctx, a, http.MethodPost, "/streams/markers", reqBytes)
	if err != nil {
		return StreamMarker{}, err
	}

	return resp.Data[0], nil
}

func (a *API) createAppAccessToken(ctx context.Context) (string, error) {
	if a.clientSecret == "" {
		return "", ErrNoClientSecret
	}

	formVal := url.Values{}
	formVal.Set("client_id", a.clientID)
	formVal.Set("client_secret", a.clientSecret)
	formVal.Set("grant_type", "client_credentials")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://id.twitch.tv/oauth2/token", strings.NewReader(formVal.Encode()))
	if err != nil {
		return "", err
	}

	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")

	resp, err := a.client.Do(req)
	if err != nil {
		return "", err
	}

	defer resp.Body.Close()

	type tokenResp struct {
		AccessToken string `json:"access_token"`
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var token tokenResp
	if err := json.Unmarshal(bodyBytes, &token); err != nil {
		return "", err
	}

	if resp.StatusCode != http.StatusOK {
		return "", ErrAppTokenStatusCode
	}

	return token.AccessToken, nil
}

func doAuthenticatedAppRequest[T any](ctx context.Context, api *API, method, url string, body []byte) (T, error) {
	api.m.Lock()
	defer api.m.Unlock()

	if api.clientSecret == "" {
		var d T
		return d, ErrNoClientSecret
	}

	resp, err := doAuthenticatedRequest[T](ctx, api, api.appAccessToken, method, url, body)
	if err != nil {
		apiErr := APIError{}
		// Unauthorized - the access token may be expired
		if errors.As(err, &apiErr) && apiErr.Status == http.StatusUnauthorized {
			token, err := api.createAppAccessToken(ctx)
			if err != nil {
				return resp, err
			}

			api.appAccessToken = token

			// retry request
			return doAuthenticatedRequest[T](ctx, api, api.appAccessToken, method, url, body)
		}

		return resp, err
	}

	return resp, nil
}

func doAuthenticatedUserRequest[T any](ctx context.Context, api *API, method, url string, body []byte) (T, error) {
	user, err := api.provider.GetAccountBy(api.accountID)
	if err != nil {
		var d T
		return d, err
	}

	resp, err := doAuthenticatedRequest[T](ctx, api, user.AccessToken, method, url, body)
	if err != nil {
		apiErr := APIError{}
		// Unauthorized - the access token may be expired
		if errors.As(err, &apiErr) &&
			apiErr.Status == http.StatusUnauthorized &&
			(apiErr.Message == "Invalid OAuth token" || apiErr.Message == "OAuth token is missing") {

			// Single flight to prevent multiple refreshes at the same time
			// If multiple requests are made at the same time, only one will refresh the token
			// The others will wait for the first to finish then use the new token
			key := "user-refresh" + user.ID + user.RefreshToken
			accessToken, shared, err := api.singleRefresh.Do(ctx, key, func(ctx context.Context) (string, error) {
				log.Logger.Info().Str("user-id", user.ID).Msg("running singleflight for token refresh")
				// refresh tokens
				accessToken, refreshToken, err := api.refresher.RefreshToken(ctx, user.RefreshToken)
				if err != nil {
					return "", err
				}

				// persists new tokens
				if err := api.provider.UpdateTokensFor(user.ID, accessToken, refreshToken); err != nil {
					return "", err
				}

				return accessToken, nil
			})
			if err != nil {
				log.Logger.Err(err).Str("user-id", user.ID).Bool("shared", shared).Msg("could not refresh token")

				api.singleRefresh.Forget(key)
				return resp, err
			}

			log.Logger.Info().Str("user-id", user.ID).Bool("shared", shared).Msg("refreshed token")

			// retry request
			return doAuthenticatedRequest[T](ctx, api, accessToken, method, url, body)
		}

		return resp, err
	}

	return resp, nil
}

func doAuthenticatedRequest[T any](ctx context.Context, api *API, token, method, endpoint string, body []byte) (T, error) {
	var data T

	url := fmt.Sprintf("%s%s", baseURL, endpoint)
	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(body))
	if err != nil {
		return data, err
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	req.Header.Set("Client-Id", api.clientID)

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := api.client.Do(req)
	if err != nil {
		return data, err
	}

	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNoContent {
		return data, nil
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return data, err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// Is rate-limit reached?
		// Then wait
		if resp.StatusCode == http.StatusTooManyRequests && resp.Header.Get("Ratelimit-Reset") != "" {
			if endpoint == "/eventsub/subscriptions" {
				return data, fmt.Errorf("reached event sub cost limit")
			}

			waitUntil, err := strconv.Atoi(resp.Header.Get("Ratelimit-Reset"))
			if err != nil {
				return data, err
			}

			diff := time.Until(time.Unix(int64(waitUntil), 0)) + time.Second*1
			timer := time.NewTimer(diff)

			defer func() {
				timer.Stop()
				select {
				case <-timer.C:
				default:
				}
			}()

			select {
			case <-timer.C: // reset time is reached, try again
				return doAuthenticatedRequest[T](ctx, api, token, method, endpoint, body)
			case <-ctx.Done():
				timer.Stop()
				return data, ctx.Err()
			}
		}

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
