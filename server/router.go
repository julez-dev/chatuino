package server

import (
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/rs/zerolog"
)

func router(logger zerolog.Logger, api *API) *chi.Mux {
	c := chi.NewMux()

	c.Use(
		middleware.RequestID,
		requestLogger(logger),
		middleware.RequestSize(5*1024),
		middleware.Recoverer,
	)

	c.Route("/internal", func(r chi.Router) {
		r.Get("/health", api.handleGetHealth())
		r.Get("/ready", api.handleGetHealth())
	})

	c.Route("/auth", func(r chi.Router) {
		r.Get("/start", api.handleAuthStart())
		r.Get("/redirect", api.handleAuthRedirect())
		r.Post("/revoke", api.handleAuthRevoke())
		r.Post("/refresh", api.handleAuthRefresh())
	})

	c.Route("/ttv", func(r chi.Router) {
		r.Get("/emotes/global", api.handleGetGlobalEmotes())

		// batched endoints
		r.Get("/channels", api.handleGetStreamUser())
		r.Get("/channels/info", api.handleGetStreamInfo())

		r.Get("/channel/{channelID}/emotes", api.handleGetChannelEmotes())
		r.Get("/channel/{channelID}/info", api.handleGetStreamInfo())
		r.Get("/channel/{channelID}/chat/settings", api.handleGetChatSettings())
		r.Get("/channel/{login}/user", api.handleGetStreamUser())
	})

	return c
}
