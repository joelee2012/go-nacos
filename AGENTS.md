# Agent Documentation for go-nacos

## Overview
This document provides essential information for AI agents working with the go-nacos repository, a Go client for Nacos (https://nacos.io/).

## Essential Commands
```sh
# Run tests with coverage report
make test

# Run quick tests
make testshort

# Format code
make fmt

# Lint code (requires staticcheck)
make lint

# Create release snapshot
goreleaser release --snapshot --clean
```

## Project Structure
- `nacos.go`: Main client implementation (Nacos API client)
- `type.go`: Data structures and types
- `respones.go`: HTTP response handling utilities
- `nacos_test.go`: Comprehensive test suite

## Key Patterns
1. **API Versioning**: Supports both Nacos v1 and v3 APIs with auto-detection
2. **HTTP Client**: Uses standard `net/http` with token-based auth
3. **Testing**: Uses Go's testing package with `github.com/stretchr/testify` assertions
4. **Pagination**: Implements pagination for list operations

## Gotchas
1. **API Compatibility**: Client works with Nacos versions 2.1.x to 2.5.x
2. **Error Handling**: Some Nacos endpoints return HTML errors (not JSON) - handled in `respones.go`
3. **Token Management**: Tokens are auto-refreshed when expired

## Testing Notes
- Tests use mock HTTP servers (`httptest`) for most cases
- Some tests require environment variables for integration testing:
  ```sh
  export NACOS_HOST=your_nacos_url
  export NACOS_USER=username
  export NACOS_PASSWORD=password
  export ACC=true
  ```

## Code Style
- Standard Go formatting (`go fmt`)
- Uses Go 1.25 features (generics for pagination)