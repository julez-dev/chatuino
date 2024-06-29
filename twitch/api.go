package twitch

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/rs/zerolog/log"
	"io"
	"net/http"
	"net/url"
	"resenje.org/singleflight"
	"strconv"
	"strings"
	"sync"
	"time"

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

	m             *sync.Mutex
	singleRefresh *singleflight.Group[string, string]

	appAccessToken string

	clientID     string
	clientSecret string
}

func NewAPI(clientID string, opts ...APIOptionFunc) (*API, error) {
	api := &API{
		clientID:      clientID,
		m:             &sync.Mutex{},
		singleRefresh: &singleflight.Group[string, string]{},
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

func (a *API) BanUser(ctx context.Context, broadcasterID string, data BanUserData) error {

	if a.provider == nil {
		return ErrNoUserAccess
	}

	loggedInUser, err := a.GetUsers(ctx, nil, nil)

	if err != nil {
		return err
	}

	if len(loggedInUser.Data) == 0 {
		return errors.New("could not fetch logged in user for current token")
	}

	values := url.Values{}
	values.Add("broadcaster_id", broadcasterID)
	values.Add("moderator_id", loggedInUser.Data[0].ID)

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
		if errors.As(err, &apiErr) && apiErr.Status == http.StatusUnauthorized {

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

func doAuthenticatedRequest[T any](ctx context.Context, api *API, token, method, url string, body []byte) (T, error) {
	var data T

	url = fmt.Sprintf("%s%s", baseURL, url)
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

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return data, err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// Is rate-limit reached?
		// Then wait
		if resp.StatusCode == http.StatusTooManyRequests && resp.Header.Get("Ratelimit-Reset") != "" {

			waitUntil, err := strconv.Atoi(resp.Header.Get("Ratelimit-Reset"))
			if err != nil {
				return data, err
			}

			diff := time.Until(time.Unix(int64(waitUntil), 0)) + time.Second*1
			timer := time.NewTimer(diff)

			select {
			case <-timer.C: // reset time is reached, try again
				return doAuthenticatedRequest[T](ctx, api, token, method, url, body)
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
