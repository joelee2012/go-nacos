[![test](https://github.com/joelee2012/go-nacos/actions/workflows/test.yml/badge.svg)](https://github.com/joelee2012/go-nacos/actions/workflows/test.yml)
[![codecov](https://codecov.io/gh/joelee2012/go-nacos/graph/badge.svg?token=PY470EX7J6)](https://codecov.io/gh/joelee2012/go-nacos)
# go-nacos
A Go client for the [Nacos](https://nacos.io/) server, supporting both the v1
and v3 console/admin APIs. It auto-detects the server's API version so a single
client works against Nacos `v2.x` (v1 console) and `v3.x` (v3 console).

## Install

```sh
go get github.com/joelee2012/go-nacos
```

## Quick start

`NewClient` returns a client that is **not** ready for use until `Init` has
probed the server and detected its API version. All resource methods require a
`context.Context` and the client is safe for concurrent use once initialized.

```go
package main

import (
	"context"
	"log"

	"github.com/joelee2012/go-nacos"
)

func main() {
	client, err := nacos.NewClient("http://localhost:8848", "nacos", "nacos")
	if err != nil {
		log.Fatal(err)
	}
	ctx := context.Background()
	if err := client.Init(ctx); err != nil {
		log.Fatalf("init: %v", err)
	}

	// Publish (create or update) a config.
	err = client.PublishConfig(ctx, &nacos.PublishCfgOpts{
		NamespaceID: "some-id",
		Group:       "some-group",
		DataID:      "some-data-id",
		Content:     "key=value",
		Type:        "properties",
	})
	if err != nil {
		log.Fatal(err)
	}

	// Read it back.
	cfg, err := client.GetConfig(ctx, &nacos.GetCfgOpts{
		NamespaceID: "some-id",
		Group:       "some-group",
		DataID:      "some-data-id",
	})
	if err != nil {
		log.Fatal(err)
	}
	log.Print(cfg.Content)
}
```

## Configuration

`NewClient` accepts functional options to customize behavior:

```go
client, err := nacos.NewClient(host, user, pass,
	nacos.WithHTTPClient(&http.Client{Timeout: 10 * time.Second}),
)
```

| Option | Description |
| --- | --- |
| `WithHTTPClient(hc)` | Override the default `*http.Client` (30s timeout). Passing `nil` is a no-op. |
| `WithAPIVersion(v)` | Pin the API version (`"v1"` or `"v3"`), skipping the `/console/server/state` auto-detection probe. |

## Pinning the API version

By default the client probes `/console/server/state` during `Init` to detect
whether the server speaks the v1 or v3 console API. Some deployments restrict
access to that endpoint; in that case pin the version with `WithAPIVersion` and
`Init` becomes a no-op (no probe is made):

```go
client, err := nacos.NewClient(host, user, pass, nacos.WithAPIVersion("v3"))
if err != nil {
    log.Fatal(err)
}
// No Init() needed; resource methods work directly.
cfg, err := client.GetConfig(ctx, opts)
```

Since no probe runs, `GetVersion` returns `("", nil)` (the server version is
unknown). An unsupported version is rejected by `NewClient` as
`nacos.ErrInvalidAPIVersion`.

## Not found

Methods that look up a single resource (`GetConfig`, `GetNamespace`,
`GetUser`, `GetRole`, `GetPermission`) return `(nil, nacos.ErrNotFound)` when
the resource does not exist. A server-side HTTP 404 also satisfies
`errors.Is`, so you can check absence uniformly:

```go
cfg, err := client.GetConfig(ctx, opts)
switch {
case errors.Is(err, nacos.ErrNotFound):
	// absent
case err != nil:
	// other error
}
```

Calling any method before `Init` returns `nacos.ErrNotInitialized`.

## API

### Lifecycle
- `NewClient(urlStr, user, password string, opts ...Option) (*Client, error)`
- `Init(ctx) error` — probe server, detect v1/v3 (idempotent; safe to retry after failure)
- `GetVersion(ctx) (string, error)` — cached server version, no I/O (returns `"", nil` when pinned)
- `GetToken(ctx) (string, error)` — access token (auto-refreshed)
- `HTTPClient() *http.Client` — the underlying transport

### Namespace
- `ListNamespace` / `GetNamespace` / `CreateNamespace` / `UpdateNamespace` /
  `DeleteNamespace` / `CreateOrUpdateNamespace`

### Config
- `GetConfig` / `ListConfig` / `ListConfigInNs` / `ListAllConfig` /
  `PublishConfig` / `DeleteConfig`

### Auth (users / roles / permissions)
- `ListUser` / `GetUser` / `CreateUser` / `DeleteUser`
- `ListRole` / `GetRole` / `CreateRole` / `DeleteRole`
- `ListPermission` / `GetPermission` / `CreatePermission` / `DeletePermission`

## Testing

Unit tests run without a server. Acceptance tests (against a live Nacos) are
gated by `ACC=true` and expect `NACOS_HOST`, `NACOS_USERNAME`,
`NACOS_PASSWORD`:

```sh
make test       # unit + coverage
ACC=true make testacc
```
