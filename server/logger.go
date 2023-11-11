package server

import (
	"context"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5/middleware"

	"github.com/rs/zerolog"
)

type ctxKeyLogger int

const loggerKey ctxKeyLogger = 0

func requestLogger(logger zerolog.Logger) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		fn := func(w http.ResponseWriter, r *http.Request) {
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			t := time.Now()

			log := logger.With().Logger()

			if id := r.Context().Value(middleware.RequestIDKey); id != nil {
				if str, ok := id.(string); ok {
					log = log.With().Str("request_id", str).Logger()
				}
			}

			ctx := context.WithValue(r.Context(), loggerKey, log)
			r = r.WithContext(ctx)

			defer func() {
				log.Info().
					Dict("request_data", zerolog.Dict().Str("type", "access").Fields(map[string]interface{}{
						"remote_ip":  r.RemoteAddr,
						"url":        r.URL.Path,
						"proto":      r.Proto,
						"method":     r.Method,
						"user_agent": r.Header.Get("User-Agent"),
						"status":     ww.Status(),
						"latency_ms": float64(time.Since(t).Milliseconds()),
						"bytes_in":   r.Header.Get("Content-Length"),
						"bytes_out":  ww.BytesWritten(),
					})).
					Msg("incoming_request")
			}()

			next.ServeHTTP(ww, r)
		}

		return http.HandlerFunc(fn)
	}
}
