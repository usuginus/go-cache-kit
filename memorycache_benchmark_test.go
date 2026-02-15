package memorycache

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/patrickmn/go-cache"
)

func newBenchmarkProvider(b *testing.B, key string) *MemoryCacheProvider[int] {
	b.Helper()
	client := cache.New(10*time.Minute, 10*time.Minute)
	provider, err := NewMemoryCacheProvider[int](key, WithCacheClient(client))
	if err != nil {
		b.Fatalf("failed to create benchmark provider: %v", err)
	}
	return provider
}

func newBenchmarkBroker(b *testing.B, key string, ttl time.Duration) *MemoryCacheBroker[int] {
	b.Helper()
	client := cache.New(10*time.Minute, 10*time.Minute)
	broker, err := NewMemoryCacheBroker[int](key, ttl, WithCacheClient(client))
	if err != nil {
		b.Fatalf("failed to create benchmark broker: %v", err)
	}
	return broker
}

func BenchmarkMemoryCacheProviderSet(b *testing.B) {
	provider := newBenchmarkProvider(b, "bench:provider:set")

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		provider.Set(i, time.Minute)
	}
}

func BenchmarkMemoryCacheProviderGetHit(b *testing.B) {
	provider := newBenchmarkProvider(b, "bench:provider:get-hit")
	provider.Set(42, time.Minute)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := provider.Get(); err != nil {
			b.Fatalf("unexpected error: %v", err)
		}
	}
}

func BenchmarkMemoryCacheProviderGetMiss(b *testing.B) {
	provider := newBenchmarkProvider(b, "bench:provider:get-miss")
	provider.Clear()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := provider.Get(); err != ErrDataNotFound {
			b.Fatalf("expected ErrDataNotFound, got: %v", err)
		}
	}
}

func BenchmarkMemoryCacheBrokerExecHit(b *testing.B) {
	broker := newBenchmarkBroker(b, "bench:broker:hit", time.Minute)
	if _, err := broker.Exec(func() (int, error) { return 42, nil }); err != nil {
		b.Fatalf("failed to warm cache: %v", err)
	}

	fetcher := func() (int, error) {
		b.Fatalf("fetcher should not be called on cache hit")
		return 0, nil
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := broker.Exec(fetcher); err != nil {
			b.Fatalf("unexpected error: %v", err)
		}
	}
}

func BenchmarkMemoryCacheBrokerExecMiss(b *testing.B) {
	broker := newBenchmarkBroker(b, "bench:broker:miss", time.Minute)
	var calls int64
	fetcher := func() (int, error) {
		atomic.AddInt64(&calls, 1)
		return 42, nil
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		broker.Clear()
		if _, err := broker.Exec(fetcher); err != nil {
			b.Fatalf("unexpected error: %v", err)
		}
	}

	b.StopTimer()
	if calls != int64(b.N) {
		b.Fatalf("expected fetcher calls %d, got %d", b.N, calls)
	}
}

func BenchmarkMemoryCacheBrokerExecHitParallel(b *testing.B) {
	broker := newBenchmarkBroker(b, "bench:broker:hit-parallel", time.Minute)
	if _, err := broker.Exec(func() (int, error) { return 42, nil }); err != nil {
		b.Fatalf("failed to warm cache: %v", err)
	}

	fetcher := func() (int, error) {
		b.Fatalf("fetcher should not be called on cache hit")
		return 0, nil
	}

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if _, err := broker.Exec(fetcher); err != nil {
				b.Fatalf("unexpected error: %v", err)
			}
		}
	})
}
