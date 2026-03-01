package store

import (
	"context"
	"encoding/json"
	"time"

	"github.com/redis/go-redis/v9"
)

type Cache struct {
	client *redis.Client
}

type CachedStatus struct {
	IsUp           bool   `json:"is_up"`
	StatusCode     int    `json:"status_code"`
	ResponseTimeMs int    `json:"response_time_ms"`
	CheckedAt      int64  `json:"checked_at"`
	Error          string `json:"error,omitempty"`
}

func NewCache(redisURL string) (*Cache, error) {
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, err
	}
	client := redis.NewClient(opts)
	if err := client.Ping(context.Background()).Err(); err != nil {
		return nil, err
	}
	return &Cache{client: client}, nil
}

func (c *Cache) SetMonitorStatus(ctx context.Context, monitorID string, status CachedStatus) error {
	data, err := json.Marshal(status)
	if err != nil {
		return err
	}
	return c.client.Set(ctx, "monitor:status:"+monitorID, data, 5*time.Minute).Err()
}

func (c *Cache) GetMonitorStatus(ctx context.Context, monitorID string) (*CachedStatus, error) {
	data, err := c.client.Get(ctx, "monitor:status:"+monitorID).Bytes()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var status CachedStatus
	if err := json.Unmarshal(data, &status); err != nil {
		return nil, err
	}
	return &status, nil
}

func (c *Cache) DeleteMonitorStatus(ctx context.Context, monitorID string) error {
	return c.client.Del(ctx, "monitor:status:"+monitorID).Err()
}

func (c *Cache) Close() error {
	return c.client.Close()
}
