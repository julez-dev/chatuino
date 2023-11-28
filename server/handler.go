package server

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

var scopes = [...]string{
	"chat:read", "chat:edit", "channel:moderate", "moderator:read:chat_settings", "moderation:read", "user:read:chat",
}

type tokenPair struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

func (a *API) handleAuthStart() http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logger := a.getLoggerFrom(r.Context())

		state, err := randomString(10)
		if err != nil {
			logger.Err(err).Msg("could not generate random state string")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		val := url.Values{}
		val.Set("client_id", a.conf.ClientID)
		val.Set("force_verify", "false")
		val.Set("redirect_uri", a.conf.RedirectURL)
		val.Set("response_type", "code")
		val.Set("scope", strings.Join(scopes[:], " "))
		val.Set("state", state)

		u := url.URL{
			Scheme:   "https",
			Host:     "id.twitch.tv",
			Path:     "oauth2/authorize",
			RawQuery: val.Encode(),
		}

		http.SetCookie(w, &http.Cookie{
			Name:     "chatuino_state",
			Value:    state,
			MaxAge:   int((time.Minute * 5).Seconds()),
			HttpOnly: true,
			Path:     "/auth/redirect",
		})

		w.Header().Set("Location", u.String())
		w.WriteHeader(http.StatusFound)
	})
}

func (a *API) handleAuthRedirect() http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logger := a.getLoggerFrom(r.Context())

		values := r.URL.Query()

		if qErr := values.Get("error"); qErr != "" {
			logger.Err(errors.New(qErr)).Msg("got error from twitch redirect")
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("App was not authorized"))
			return
		}

		if !slices.Equal(strings.Split(values.Get("scope"), " "), scopes[:]) {
			logger.Error().Strs("want", scopes[:]).Str("got", values.Get("scope")).Msg("returned scopes don't match expected scopes")
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("returned scopes don't match expected scopes"))
			return
		}

		qState := values.Get("state")

		if qState == "" {
			logger.Error().Msg("state is missing from twitch redirect")
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("State query parameter missing"))
			return
		}

		cookie, err := r.Cookie("chatuino_state")
		if err != nil {
			logger.Error().Msg("state cookie is missing")
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("State cookie is missing"))
			return
		}

		if qState != cookie.Value {
			logger.Error().Msg("state cookie does not match query from redirect")
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("State cookie does not match query parameter for state"))
			return
		}

		// delete cookie
		http.SetCookie(w, &http.Cookie{
			Name:    "chatuino_state",
			Expires: time.Now().Add(-24 * time.Hour),
		})

		// request token + refresh token
		formVal := url.Values{}
		formVal.Set("client_id", a.conf.ClientID)
		formVal.Set("client_secret", a.conf.ClientSecret)
		formVal.Set("code", values.Get("code"))
		formVal.Set("grant_type", "authorization_code")
		formVal.Set("redirect_uri", a.conf.RedirectURL)

		req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, "https://id.twitch.tv/oauth2/token", strings.NewReader(formVal.Encode()))
		if err != nil {
			logger.Err(err).Msg("could not create new http.Request")
			w.WriteHeader(http.StatusInternalServerError)
		}

		req.Header.Add("Content-Type", "application/x-www-form-urlencoded")

		resp, err := a.client.Do(req)
		if err != nil {
			logger.Err(err).Msg("could not do http request")
			w.WriteHeader(http.StatusInternalServerError)
		}

		defer resp.Body.Close()

		bodyBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			logger.Err(err).Msg("could not read response body")
			w.WriteHeader(http.StatusInternalServerError)
		}

		var tokenData tokenPair
		if err := json.Unmarshal(bodyBytes, &tokenData); err != nil {
			logger.Err(err).Msg("could not json unmarshal response body")
			w.WriteHeader(http.StatusInternalServerError)
		}

		w.WriteHeader(resp.StatusCode)
		fmt.Fprintf(w, "Paste this in Chatuinos account prompt: %s%%%s", tokenData.AccessToken, tokenData.RefreshToken)
	})
}

func (a *API) handleAuthRevoke() http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logger := a.getLoggerFrom(r.Context())
		splits := strings.SplitN(r.Header.Get("Authorization"), " ", 2)

		if len(splits) != 2 {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("Authorization header with token missing or malformed"))
			return
		}

		token := splits[1]
		formVal := url.Values{}
		formVal.Set("client_id", a.conf.ClientID)
		formVal.Set("token", token)

		req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, "https://id.twitch.tv/oauth2/revoke", strings.NewReader(formVal.Encode()))
		if err != nil {
			logger.Err(err).Msg("could not create new http.Request")
			w.WriteHeader(http.StatusInternalServerError)
		}

		req.Header.Add("Content-Type", "application/x-www-form-urlencoded")

		resp, err := a.client.Do(req)
		if err != nil {
			logger.Err(err).Msg("could not do http request")
			w.WriteHeader(http.StatusInternalServerError)
		}

		defer resp.Body.Close()

		if resp.StatusCode == http.StatusOK {
			return
		}

		w.WriteHeader(resp.StatusCode)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("ttv resp: "))
		io.Copy(w, resp.Body)
	})
}

func (a *API) handleAuthRefresh() http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logger := a.getLoggerFrom(r.Context())
		splits := strings.SplitN(r.Header.Get("Authorization"), " ", 2)

		if len(splits) != 2 {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("Authorization header with token missing or malformed"))
			return
		}

		refreshToken := splits[1]
		formVal := url.Values{}
		formVal.Set("client_id", a.conf.ClientID)
		formVal.Set("client_secret", a.conf.ClientSecret)
		formVal.Set("grant_type", "refresh_token")
		formVal.Set("refresh_token", refreshToken)

		req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, "https://id.twitch.tv/oauth2/token", strings.NewReader(formVal.Encode()))
		if err != nil {
			logger.Err(err).Msg("could not create new http.Request")
			w.WriteHeader(http.StatusInternalServerError)
		}

		req.Header.Add("Content-Type", "application/x-www-form-urlencoded")

		resp, err := a.client.Do(req)
		if err != nil {
			logger.Err(err).Msg("could not do http request")
			w.WriteHeader(http.StatusInternalServerError)
		}

		defer resp.Body.Close()

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(resp.StatusCode)
		io.Copy(w, resp.Body)
	})
}

func (a *API) handleGetGlobalEmotes() http.HandlerFunc {
	// client := &http.Client{}

	// proxy := &httputil.ReverseProxy{
	// 	Rewrite: func(r *httputil.ProxyRequest) {
	// 		url := &url.URL{
	// 			Scheme: "https",
	// 			Host:   "api.twitch.tv",
	// 			Path:   "/helix/chat/emotes/global",
	// 		}

	// 		r.SetURL(url)
	// 		r.SetXForwarded()
	// 	},
	// 	Transport: client.Transport,
	// }

	// return func(w http.ResponseWriter, r *http.Request) {
	// 	r.URL.Path = "" // remove path so that RP does not add it to the outgoing request
	// 	proxy.ServeHTTP(w, r)
	// }

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logger := a.getLoggerFrom(r.Context())

		emotes, err := a.ttvAPI.GetGlobalEmotes(r.Context())
		if err != nil {
			logger.Err(err).Msg("could not get global emotes")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		jsonData, err := json.Marshal(emotes)
		if err != nil {
			logger.Err(err).Msg("could not marshal global emotes")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(jsonData)
	})
}

func (a *API) handleGetChannelEmotes() http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logger := a.getLoggerFrom(r.Context())

		emotes, err := a.ttvAPI.GetChannelEmotes(r.Context(), chi.URLParam(r, "channelID"))
		if err != nil {
			logger.Err(err).Msg("could not get global emotes")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		jsonData, err := json.Marshal(emotes)
		if err != nil {
			logger.Err(err).Msg("could not marshal global emotes")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(jsonData)
	})
}

func (a *API) handleGetStreamInfo() http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logger := a.getLoggerFrom(r.Context())

		info, err := a.ttvAPI.GetStreamInfo(r.Context(), []string{chi.URLParam(r, "channelID")})
		if err != nil {
			logger.Err(err).Str("channel", chi.URLParam(r, "channelID")).Msg("could not get stream info")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		jsonData, err := json.Marshal(info)
		if err != nil {
			logger.Err(err).Msg("could not marshal info")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(jsonData)
	})
}

func (a *API) handleGetChatSettings() http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logger := a.getLoggerFrom(r.Context())

		settings, err := a.ttvAPI.GetChatSettings(r.Context(), chi.URLParam(r, "channelID"), "")
		if err != nil {
			logger.Err(err).Str("channel", chi.URLParam(r, "channelID")).Msg("could not get chat settings")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		jsonData, err := json.Marshal(settings)
		if err != nil {
			logger.Err(err).Msg("could not marshal settings")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(jsonData)
	})
}

func (a *API) handleGetStreamUser() http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logger := a.getLoggerFrom(r.Context())

		users, err := a.ttvAPI.GetUsers(r.Context(), []string{chi.URLParam(r, "login")}, nil)
		if err != nil {
			logger.Err(err).Str("login", chi.URLParam(r, "login")).Msg("could not get stream user")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		jsonData, err := json.Marshal(users)
		if err != nil {
			logger.Err(err).Msg("could not marshal users")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(jsonData)
	})
}

func randomString(n int) (string, error) {
	b := make([]byte, n)

	if _, err := rand.Read(b); err != nil {
		return "", err
	}

	return hex.EncodeToString(b), nil
}
