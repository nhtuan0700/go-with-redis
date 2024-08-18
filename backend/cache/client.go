package cache

import (
	"context"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
)

var ErrCacheMiss = errors.New("cache miss")

type OrderType uint16

const (
	OrderAsc  OrderType = 0
	OrderDesc OrderType = 1
)

type Client interface {
	Get(ctx context.Context, key string) (any, error)
	Set(ctx context.Context, key string, data any, ttl time.Duration) error
	Expire(ctx context.Context, key string, ttl time.Duration) error
	Incr(ctx context.Context, key string) error

	AddToSet(ctx context.Context, key string, data ...any) error
	IsDataInSet(ctx context.Context, key string, data any) (bool, error)

	AddToSortedSet(ctx context.Context, key string, data ...redis.Z) error
	GetSortedSet(ctx context.Context, key string, order OrderType) ([]redis.Z, error)

	AddHyperLogLog(ctx context.Context, key string, data ...any) error
	GetHyperLogLogCount(ctx context.Context, key string) (uint64, error)
}

type redisClient struct {
	redisClient *redis.Client
}

func NewRedisClient() Client {
	return &redisClient{
		redisClient: redis.NewClient(&redis.Options{
			Username: "",
			Password: "",
			Addr:     "redis-1:6379",
		}),
	}
}

func (c *redisClient) Get(ctx context.Context, key string) (any, error) {
	result, err := c.redisClient.Get(ctx, key).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, ErrCacheMiss
		}
		return nil, err
	}

	return result, err
}

func (c *redisClient) Set(ctx context.Context, key string, data any, ttl time.Duration) error {
	err := c.redisClient.Set(ctx, key, data, ttl).Err()
	if err != nil {
		return err
	}

	return nil
}

func (c *redisClient) Incr(ctx context.Context, key string) error {
	err := c.redisClient.Incr(ctx, key).Err()
	if err != nil {
		return err
	}

	return nil
}

func (c *redisClient) Expire(ctx context.Context, key string, ttl time.Duration) error {
	err := c.redisClient.Expire(ctx, key, ttl).Err()
	if err != nil {
		return err
	}

	return nil
}

func (c *redisClient) AddToSet(ctx context.Context, key string, data ...any) error {
	if err := c.redisClient.SAdd(ctx, key, data).Err(); err != nil {
		return err
	}

	return nil
}

func (c *redisClient) IsDataInSet(ctx context.Context, key string, data any) (bool, error) {
	result, err := c.redisClient.SIsMember(ctx, key, data).Result()
	if err != nil {
		return false, err
	}

	return result, err
}

func (c *redisClient) AddToSortedSet(ctx context.Context, key string, data ...redis.Z) error {
	if err := c.redisClient.ZAdd(ctx, key, data...).Err(); err != nil {
		return err
	}

	return nil
}

func (c *redisClient) GetSortedSet(ctx context.Context, key string, order OrderType) ([]redis.Z, error) {
	var result []redis.Z
	var err error

	switch order {
	case OrderAsc:
		result, err = c.redisClient.ZRangeWithScores(ctx, key, 0, -1).Result()
	case OrderDesc:
		result, err = c.redisClient.ZRevRangeWithScores(ctx, key, 0, -1).Result()
	}

	if err != nil {
		return []redis.Z{}, nil
	}

	return result, nil
}

func (c *redisClient) AddHyperLogLog(ctx context.Context, key string, data ...any) error {
	err := c.redisClient.PFAdd(ctx, key, data...).Err()
	if err != nil {
		return err
	}

	return nil
}

func (c *redisClient) GetHyperLogLogCount(ctx context.Context, key string) (uint64, error) {
	count, err := c.redisClient.PFCount(ctx, key).Result()
	if err != nil {
		return 0, err
	}

	return uint64(count), nil
}
