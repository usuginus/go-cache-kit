package memorycache

import (
	"errors"
	"strings"
	"time"

	"github.com/patrickmn/go-cache"
)

// DefaultExpiration is the default duration for cache item expiration.
const DefaultExpiration = 10 * time.Second

// DefaultCleanupInterval is the interval at which expired cache items are cleaned up.
const DefaultCleanupInterval = 60 * time.Second

var (
	// ErrDataNotFound is returned when the requested cache entry does not exist.
	ErrDataNotFound = errors.New("cache data not found")
	// ErrAssertionFailed is returned when a cached value cannot be type-asserted to the expected type.
	ErrAssertionFailed = errors.New("cache data type assertion failed")
	// ErrNilDataFetcher is returned when a nil data-fetching function is passed to broker Exec.
	ErrNilDataFetcher = errors.New("data-fetching function is nil")
	// ErrInvalidCacheKey is returned when cache key is empty or whitespace only.
	ErrInvalidCacheKey = errors.New("cache key must not be empty")
	// ErrNilCacheClient is returned when WithCacheClient receives a nil cache instance.
	ErrNilCacheClient = errors.New("cache client must not be nil")
	// ErrInvalidCacheTTL is returned when broker TTL is not supported.
	ErrInvalidCacheTTL = errors.New("cache ttl must be positive, cache.DefaultExpiration, or cache.NoExpiration")
)

// defaultCacheClient is the underlying shared in-memory cache instance.
var defaultCacheClient = cache.New(DefaultExpiration, DefaultCleanupInterval)

// ProviderOption configures a MemoryCacheProvider.
type ProviderOption func(*providerConfig)

type providerConfig struct {
	client            *cache.Cache
	hasCustomClient   bool
	useCacheConfig    bool
	defaultExpiration time.Duration
	cleanupInterval   time.Duration
}

// WithCacheClient lets callers inject a custom cache client.
func WithCacheClient(client *cache.Cache) ProviderOption {
	return func(cfg *providerConfig) {
		cfg.hasCustomClient = true
		cfg.client = client
	}
}

// WithCacheConfig creates a dedicated cache client using the supplied settings.
// This option overrides WithCacheClient when both are provided.
func WithCacheConfig(defaultExpiration, cleanupInterval time.Duration) ProviderOption {
	return func(cfg *providerConfig) {
		cfg.useCacheConfig = true
		cfg.defaultExpiration = defaultExpiration
		cfg.cleanupInterval = cleanupInterval
	}
}

// WithIsolatedCache creates a dedicated cache client using package default settings.
func WithIsolatedCache() ProviderOption {
	return WithCacheConfig(DefaultExpiration, DefaultCleanupInterval)
}

// MemoryCacheProvider provides a generic interface for caching values of type T.
type MemoryCacheProvider[T any] struct {
	cacheKey string
	client   *cache.Cache
}

// NewMemoryCacheProvider creates a new MemoryCacheProvider with the specified cache key.
func NewMemoryCacheProvider[T any](cacheKey string, opts ...ProviderOption) (*MemoryCacheProvider[T], error) {
	if strings.TrimSpace(cacheKey) == "" {
		return nil, ErrInvalidCacheKey
	}

	cfg := providerConfig{client: defaultCacheClient}
	for _, opt := range opts {
		opt(&cfg)
	}

	if cfg.useCacheConfig {
		cfg.client = cache.New(cfg.defaultExpiration, cfg.cleanupInterval)
	} else if cfg.hasCustomClient && cfg.client == nil {
		return nil, ErrNilCacheClient
	}
	if cfg.client == nil {
		cfg.client = defaultCacheClient
	}
	return &MemoryCacheProvider[T]{
		cacheKey: cacheKey,
		client:   cfg.client,
	}, nil
}

// Get retrieves the cached value associated with the provider's cache key.
// If the cache entry is missing or if the type assertion fails, an error is returned.
func (m *MemoryCacheProvider[T]) Get() (T, error) {
	var result T
	raw, found := m.client.Get(m.cacheKey)
	if !found {
		return result, ErrDataNotFound
	}
	value, ok := raw.(T)
	if !ok {
		return result, ErrAssertionFailed
	}
	return value, nil
}

// Set caches the given value with a custom time-to-live (TTL).
func (m *MemoryCacheProvider[T]) Set(value T, ttl time.Duration) {
	m.client.Set(m.cacheKey, value, ttl)
}

// SetDefault caches the given value using the default expiration time.
func (m *MemoryCacheProvider[T]) SetDefault(value T) {
	m.client.SetDefault(m.cacheKey, value)
}

// Clear removes the cached entry associated with the provider's cache key.
func (m *MemoryCacheProvider[T]) Clear() {
	m.client.Delete(m.cacheKey)
}
