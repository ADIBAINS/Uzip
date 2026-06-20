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
		return model.Link{}, true, ErrNotFound
	}
	var link model.Link
	if err := json.Unmarshal(data, &link); err != nil {
		return link, false, err
	}
	return link, true, nil
}

func (c *Cache) SetLink(ctx context.Context, link model.Link) error {
	data, err := json.Marshal(link)
	if err != nil {
		return err
	}
	ttl := 15 * time.Minute
	if link.ExpiresAt != nil && time.Until(*link.ExpiresAt) < ttl {
		ttl = time.Until(*link.ExpiresAt)
	}
	if ttl <= 0 {
		ttl = time.Second
	}
	return c.Client.Set(ctx, "link:"+link.Alias, data, ttl).Err()
}

func (c *Cache) SetMissing(ctx context.Context, alias string) error {
	return c.Client.Set(ctx, "link:"+alias, "!", time.Minute).Err()
}

func (c *Cache) Delete(ctx context.Context, alias string) error {
	return c.Client.Del(ctx, "link:"+alias).Err()
}

func (c *Cache) Allow(ctx context.Context, key string, limit int64, window time.Duration) (bool, error) {
	script := redis.NewScript(`
		local count = redis.call("INCR", KEYS[1])
		if count == 1 then redis.call("PEXPIRE", KEYS[1], ARGV[1]) end
		return count
	`)
	count, err := script.Run(ctx, c.Client, []string{"rate:" + key}, window.Milliseconds()).Int64()
	if err != nil {
		return true, err
	}
	return count <= limit, nil
}
