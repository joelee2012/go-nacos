[![test](https://github.com/joelee2012/go-nacos/actions/workflows/test.yml/badge.svg)](https://github.com/joelee2012/go-nacos/actions/workflows/test.yml)
[![codecov](https://codecov.io/gh/joelee2012/go-nacos/graph/badge.svg?token=PY470EX7J6)](https://codecov.io/gh/joelee2012/go-nacos)
# go-nacos
Go client for the [Nacos](https://nacos.io/), API version must be compatible with [api v1](https://nacos.io/docs/v1/open-api/?spm=5238cd80.2ef5001f.0.0.3f613b7cibLcyN)
  - v2.1.x
  - v2.2.x
  - v2.3.x
  - v2.4.x
  - v2.5.x
  
# Usage

```sh
go get https://github.com/joelee2012/go-nacos
```

# Example

```go
client := NewNacosClient()
opts := CreateCfgOpts{NamespaceID: "some-id", Group: "some-group", DataID: "some-data-id", Content: "config content"}
client.CreateConfig(&opts)
```
