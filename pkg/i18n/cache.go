package i18n

import (
	"context"
	"time"
)

// Cache interface for translation caching
type Cache interface {
	Get(ctx context.Context, key string) (*Translation, error)
	Set(ctx context.Context, key string, value *Translation, ttl time.Duration) error
	Delete(ctx context.Context, key string) error
	Clear(ctx context.Context) error
	Stats() CacheStats
}

// CacheStats provides cache statistics
type CacheStats struct {
	Hits       int64
	Misses     int64
	Sets       int64
	Deletes    int64
	Size       int64
	MaxSize    int64
	HitRate    float64
}