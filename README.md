# go-cache-kit

`go-cache-kit` is a lightweight, generic helper for in-memory caching in Go.
It wraps [go-cache](https://github.com/patrickmn/go-cache) with type-safe APIs and a cache-aside execution helper.

## Features

- Type-safe cache access with Go generics
- `MemoryCacheBroker.Exec` for cache-aside flow (`hit -> return`, `miss -> fetch -> set -> return`)
- Concurrent miss de-duplication per broker instance
- Configurable cache client via options
- Constructor input validation (`key`, `ttl`, and nil custom client checks)

## Installation

```bash
go get github.com/usuginus/go-cache-kit
```

```go
import memorycache "github.com/usuginus/go-cache-kit"
```

## Quick Start

### MemoryCacheProvider

```go
package main

import (
	"fmt"
	"time"

	memorycache "github.com/usuginus/go-cache-kit"
)

func main() {
	provider, err := memorycache.NewMemoryCacheProvider[int]("example:key")
	if err != nil {
		fmt.Println("provider init error:", err)
		return
	}

	provider.Set(42, 10*time.Second)

	value, err := provider.Get()
	if err != nil {
		fmt.Println("provider get error:", err)
		return
	}
	fmt.Println("cached value:", value)
}
```

### MemoryCacheBroker

```go
package main

import (
	"fmt"
	"time"

	memorycache "github.com/usuginus/go-cache-kit"
)

func main() {
	broker, err := memorycache.NewMemoryCacheBroker[string]("example:broker", 30*time.Second)
	if err != nil {
		fmt.Println("broker init error:", err)
		return
	}

	value, err := broker.Exec(func() (string, error) {
		return "Hello, Cached World!", nil
	})
	if err != nil {
		fmt.Println("broker exec error:", err)
		return
	}

	fmt.Println("result:", value)
}
```

## Configuration Options

- `WithCacheClient(client)`:
  reuse an existing `*cache.Cache` instance
- `WithCacheConfig(defaultExpiration, cleanupInterval)`:
  create a dedicated cache client with custom settings
- `WithIsolatedCache()`:
  create a dedicated cache client with package defaults

By default, providers share one package-level cache instance.
Use unique keys across your application when relying on the shared default.

## Validation Rules

- `NewMemoryCacheProvider`:
  `key` must not be empty or whitespace
- `WithCacheClient(nil)`:
  rejected with `ErrNilCacheClient`
- `NewMemoryCacheBroker`:
  `ttl` must be positive, `cache.DefaultExpiration`, or `cache.NoExpiration`
- `MemoryCacheBroker.Exec(nil)`:
  rejected with `ErrNilDataFetcher`

## License

MIT License. See [LICENSE](LICENSE).
