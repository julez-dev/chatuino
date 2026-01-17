package server

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog/log"
)

// RedisClient wraps the Redis client for our use cases
type RedisClient struct {
	client *redis.Client
}

// RedisConfig holds Redis connection configuration
type RedisConfig struct {
	// Addr is the Redis server address (host:port)
	Addr string
	// Password for Redis authentication (optional)
	Password string
	// DB is the Redis database number (default 0)
	DB int
}

// NewRedisClient creates a new Redis client with the given configuration.
// Returns an error if the connection fails.
func NewRedisClient(ctx context.Context, cfg RedisConfig) (*RedisClient, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     cfg.Addr,
		Password: cfg.Password,
		DB:       cfg.DB,
	})

	// Test connection with timeout
	pingCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	if err := client.Ping(pingCtx).Err(); err != nil {
		client.Close()
		return nil, err
	}

	log.Info().Str("addr", cfg.Addr).Msg("redis connected")
	return &RedisClient{client: client}, nil
}

func (r *RedisClient) Close() error {
	return r.client.Close()
}

type NopRedisClient struct{}

// NewNopRedisClient returns a no-op Redis client that does nothing.
// Used when Redis is disabled or unavailable (fail-open behavior).
func NewNopRedisClient() *NopRedisClient {
	return &NopRedisClient{}
}

func (n *NopRedisClient) Close() error {
	return nil
}
