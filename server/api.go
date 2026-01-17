package server

import (
	"context"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/rs/zerolog"
	"golang.org/x/sync/errgroup"
)

type RatelimitStore interface {
	io.Closer
	// todo define api later
}

type Config struct {
	ClientID             string
	ClientSecret         string
	HostAndPort          string
	RedirectURL          string
	Redis                RedisConfig
	EnableProxyRateLimit bool
}

type API struct {
	logger             zerolog.Logger
	conf               Config
	client             *http.Client
	helixTokenProvider *HelixTokenProvider
	ratelimitStore     RatelimitStore
}

func New(logger zerolog.Logger, config Config, client *http.Client) *API {
	return &API{
		logger:             logger,
		conf:               config,
		client:             client,
		helixTokenProvider: NewHelixTokenProvider(client, config.ClientID, config.ClientSecret),
	}
}

func (a *API) Launch(ctx context.Context) error {
	if a.conf.EnableProxyRateLimit {
		redisClient, err := NewRedisClient(ctx, a.conf.Redis)
		if err != nil {
			a.logger.Error().Err(err).Msg("redis connection failed")
			return err
		}

		a.ratelimitStore = redisClient
	} else {
		a.ratelimitStore = NewNopRedisClient()
	}

	httpSrv := &http.Server{
		Addr:           a.conf.HostAndPort,
		WriteTimeout:   time.Second * 15,
		ReadTimeout:    time.Second * 15,
		IdleTimeout:    time.Second * 60,
		MaxHeaderBytes: 2 * 1024,
		Handler:        router(a.logger, a),
	}

	httpSrv.RegisterOnShutdown(func() {
		a.logger.Info().Msg("http shutdown started")
		a.ratelimitStore.Close()
	})

	wg, ctx := errgroup.WithContext(ctx)

	wg.Go(func() error {
		a.logger.Info().
			Str("addr", httpSrv.Addr).
			Str("redirect-url", a.conf.RedirectURL).
			Msg("starting http server")

		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}

		return nil
	})

	wg.Go(func() error {
		<-ctx.Done()

		ctx, cancel := context.WithTimeout(ctx, time.Second*15)
		defer cancel()

		if err := httpSrv.Shutdown(ctx); err != nil {
			return err
		}

		a.logger.Info().Msg("shutdown done")

		return nil
	})

	if err := wg.Wait(); err != nil {
		return err
	}

	return nil
}

func (a *API) getLoggerFrom(ctx context.Context) zerolog.Logger {
	if logger := ctx.Value(loggerKey); logger != nil {
		typed, ok := logger.(zerolog.Logger)

		if ok {
			return typed
		}
	}

	return a.logger
}
