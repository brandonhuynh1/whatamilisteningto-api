package database

import (
	"context"
	"fmt"
	"time"

	"github.com/brandonhuynh1/whatamilisteningto-api/internal/config"
	"github.com/go-redis/redis/v8"
)

// RedisClient wraps the redis.Client with additional functionality
type RedisClient struct {
	client *redis.Client
}

// NewRedisClient creates a new Redis client
func NewRedisClient(cfg config.RedisConfig) (*RedisClient, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
		Password: cfg.Password,
		DB:       cfg.DB,
	})

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	return &RedisClient{client: client}, nil
}

// Close closes the Redis client connection
func (rc *RedisClient) Close() error {
	return rc.client.Close()
}

// Set sets a key-value pair with an optional expiration
func (rc *RedisClient) Set(ctx context.Context, key string, value interface{}, expiration time.Duration) error {
	return rc.client.Set(ctx, key, value, expiration).Err()
}

// Get retrieves a value by key
func (rc *RedisClient) Get(ctx context.Context, key string) (string, error) {
	return rc.client.Get(ctx, key).Result()
}

// Delete deletes a key
func (rc *RedisClient) Delete(ctx context.Context, key string) error {
	return rc.client.Del(ctx, key).Err()
}

// HashSet sets a field in a hash stored at key
func (rc *RedisClient) HashSet(ctx context.Context, key, field string, value interface{}) error {
	return rc.client.HSet(ctx, key, field, value).Err()
}

// HashGet retrieves a field from a hash stored at key
func (rc *RedisClient) HashGet(ctx context.Context, key, field string) (string, error) {
	return rc.client.HGet(ctx, key, field).Result()
}

// HashGetAll retrieves all fields and values of a hash stored at key
func (rc *RedisClient) HashGetAll(ctx context.Context, key string) (map[string]string, error) {
	return rc.client.HGetAll(ctx, key).Result()
}

// Publish publishes a message to a channel
func (rc *RedisClient) Publish(ctx context.Context, channel string, message interface{}) error {
	return rc.client.Publish(ctx, channel, message).Err()
}

// Subscribe subscribes to channels and returns a message channel
func (rc *RedisClient) Subscribe(ctx context.Context, channels ...string) *redis.PubSub {
	return rc.client.Subscribe(ctx, channels...)
}

// AddToSet adds members to a set
func (rc *RedisClient) AddToSet(ctx context.Context, key string, members ...interface{}) error {
	return rc.client.SAdd(ctx, key, members...).Err()
}

// GetSetMembers returns all members of a set
func (rc *RedisClient) GetSetMembers(ctx context.Context, key string) ([]string, error) {
	return rc.client.SMembers(ctx, key).Result()
}

// RemoveFromSet removes members from a set
func (rc *RedisClient) RemoveFromSet(ctx context.Context, key string, members ...interface{}) error {
	return rc.client.SRem(ctx, key, members...).Err()
}

// GetSetSize returns the number of members in a set
func (rc *RedisClient) GetSetSize(ctx context.Context, key string) (int64, error) {
	return rc.client.SCard(ctx, key).Result()
}

// IncrementCounter increments a counter by 1 and returns the new value
func (rc *RedisClient) IncrementCounter(ctx context.Context, key string) (int64, error) {
	return rc.client.Incr(ctx, key).Result()
}

// DecrementCounter decrements a counter by 1 and returns the new value
func (rc *RedisClient) DecrementCounter(ctx context.Context, key string) (int64, error) {
	return rc.client.Decr(ctx, key).Result()
}

// SetExpiration sets an expiration on a key
func (rc *RedisClient) SetExpiration(ctx context.Context, key string, expiration time.Duration) error {
	return rc.client.Expire(ctx, key, expiration).Err()
}
