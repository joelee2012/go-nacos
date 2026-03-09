[![test](https://github.com/joelee2012/go-nacos/actions/workflows/test.yml/badge.svg)](https://github.com/joelee2012/go-nacos/actions/workflows/test.yml)
[![codecov](https://codecov.io/gh/joelee2012/go-nacos/graph/badge.svg?token=PY470EX7J6)](https://codecov.io/gh/joelee2012/go-nacos)
# go-nacos
Go client for the [Nacos](https://nacos.io/)
  
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
