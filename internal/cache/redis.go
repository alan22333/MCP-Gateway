package cache

import (
	"context"
	"encoding/json"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisConfig Redis 连接配置
type RedisConfig struct {
	Addr     string
	Password string
	DB       int
}

// RedisCache 基于 Redis 的缓存实现
type RedisCache struct {
	client *redis.Client
}

// NewRedisCache 创建 Redis 缓存实例，如果连接失败返回 error
func NewRedisCache(cfg RedisConfig) (*RedisCache, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     cfg.Addr,
		Password: cfg.Password,
		DB:       cfg.DB,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, err
	}
	return &RedisCache{client: client}, nil
}

// Get 从 Redis 查询缓存
func (r *RedisCache) Get(ctx context.Context, group, toolName string, args json.RawMessage) (*Entry, bool) {
	key := CacheKey(group, toolName, args)
	data, err := r.client.Get(ctx, key).Bytes()
	if err != nil {
		return nil, false
	}
	return &Entry{ToolName: toolName, Args: args, Result: data, HitAt: time.Now()}, true
}

// Set 写入 Redis 缓存
func (r *RedisCache) Set(ctx context.Context, group, toolName string, args json.RawMessage, result json.RawMessage, ttl time.Duration) {
	key := CacheKey(group, toolName, args)
	r.client.Set(ctx, key, string(result), ttl)
}

// InvalidateGroup 使用 SCAN + DEL 批量清除指定 group 的所有 key
func (r *RedisCache) InvalidateGroup(ctx context.Context, group string) {
	pattern := keyPrefix(group) + "*"
	var cursor uint64
	for {
		keys, nextCursor, err := r.client.Scan(ctx, cursor, pattern, 100).Result()
		if err != nil {
			return
		}
		if len(keys) > 0 {
			r.client.Del(ctx, keys...)
		}
		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}
}

// Close 关闭 Redis 连接
func (r *RedisCache) Close() error {
	return r.client.Close()
}
