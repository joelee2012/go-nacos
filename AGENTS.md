# Agent Guidelines for go-nacos

This document provides essential information for AI agents working with the go-nacos repository.

## Project Overview

A Go client library for interacting with Nacos (Dynamic Naming and Configuration Service). Supports both v1 and v3 API versions.

## Key Commands

- `make test`: Run tests with coverage report
- `make testshort`: Run tests with short timeout (30s)
- `source .env && make testacc`: Run tests with ACC=true
- `make fmt`: Format code
- `make lint`: Lint code using staticcheck
- `make release`: Create a release snapshot using goreleaser

## Code Structure

- `nacos.go`: Main implementation file with Client struct and core API methods
- `nacos_test.go`: unit test suite
- `acc_test.go`: acceptance test suite
- `type.go`: Type definitions (not shown in directory but referenced)

## Patterns and Conventions

1. **API Version Handling**: 
   - Client automatically detects API version (v1 or v3)
   - Methods handle version differences internally

2. **Error Handling**:
   - Uses `checkErr` helper to validate HTTP responses
   - Custom error types may be added (commented in code)

3. **Testing**:
   - Mock HTTP server in tests (`startServer` helper)
   - Tests cover both v1 and v3 API versions
   - Uses testify/assert for assertions

## Gotchas

- Some endpoints return different structures between v1 and v3
- Empty responses may indicate "404 Not Found" errors
- Pagination is handled internally in methods like `ListConfigInNs`

## Dependencies

- Go 1.25.0+
- `github.com/stretchr/testify` for testing

## CI/CD

GitHub Actions workflow (`test.yml`) runs tests on push/pull request.
