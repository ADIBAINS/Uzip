package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/adilbains/uzip/internal/model"
	"github.com/redis/go-redis/v9"
)

type Cache struct{ Client *redis.Client }

func OpenCache(url string) (*Cache, error) {
	options, err := redis.ParseURL(url)
	if err != nil {
		return nil, err
	}
	options.ContextTimeoutEnabled = true
	return &Cache{Client: redis.NewClient(options)}, nil
}

func (c *Cache) Close() error                   { return c.Client.Close() }
func (c *Cache) Ping(ctx context.Context) error { return c.Client.Ping(ctx).Err() }

func (c *Cache) GetLink(ctx context.Context, alias string) (model.Link, bool, error) {
	data, err := c.Client.Get(ctx, "link:"+alias).Bytes()
	if errors.Is(err, redis.Nil) {
		return model.Link{}, false, nil
	}
	if err != nil {
		return model.Link{}, false, err
	}
	if string(data) == "!" {
		// Entries written by older versions must not hide newly created aliases.
		return model.Link{}, false, nil
	}
	var link model.Link
	if err := json.Unmarshal(data, &link); err != nil {
		return link, false, err
	}
	return link, true, nil
}

func (c *Cache) CacheResolvedLink(ctx context.Context, link model.Link) error {
	data, err := json.Marshal(link)
	if err != nil {
		return err
	}
	// Limit how long an active entry can survive an archive during a Redis outage.
	ttl := time.Minute
	if link.ArchivedAt == nil && link.ExpiresAt != nil && time.Until(*link.ExpiresAt) < ttl {
		ttl = time.Until(*link.ExpiresAt)
	}
	if ttl <= 0 {
		ttl = time.Second
	}
	// A concurrent archive may have already replaced the entry.
	return c.Client.SetNX(ctx, "link:"+link.Alias, data, ttl).Err()
}

func (c *Cache) CacheArchivedLink(ctx context.Context, link model.Link) error {
	data, err := json.Marshal(link)
	if err != nil {
		return err
	}
	return c.Client.Set(ctx, "link:"+link.Alias, data, time.Minute).Err()
}

var tokenBucket = redis.NewScript(`
	local capacity = tonumber(ARGV[1])
	local window = tonumber(ARGV[2])
	local clock = redis.call("TIME")
	local now = clock[1] * 1000 + math.floor(clock[2] / 1000)
	local state = redis.call("HMGET", KEYS[1], "tokens", "updated")
	local updated = tonumber(state[2]) or now
	now = math.max(now, updated)
	local tokens = math.min(capacity, (tonumber(state[1]) or capacity) + (now - updated) * capacity / window)
	if tokens < 1 then
		return math.ceil((1 - tokens) * window / capacity)
	end
	redis.call("HSET", KEYS[1], "tokens", tokens - 1, "updated", now)
	redis.call("PEXPIRE", KEYS[1], window)
	return 0
`)

func (c *Cache) RateLimit(ctx context.Context, key string, capacity int, window time.Duration) (time.Duration, error) {
	delay, err := tokenBucket.Run(ctx, c.Client, []string{"bucket:" + key}, capacity, window.Milliseconds()).Int64()
	return time.Duration(delay) * time.Millisecond, err
}
