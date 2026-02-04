# Contributing to Gateway Rate Limiter

First off, thank you for considering contributing to this project! It's people like you that make the open-source community such an amazing place to learn, inspire, and create.

## How Can I Contribute?

### 1. Reporting Bugs
- Ensure the bug was not already reported by searching on GitHub under Issues.
- If you're unable to find an open issue addressing the problem, open a new one. Be sure to include a title and clear description, as well as a code sample or an executable test case demonstrating the expected behavior that is not occurring.

### 2. Suggesting Enhancements
- Open a new issue with a clear title and detailed description.
- Explain why this enhancement would be useful to most users.

### 3. Pull Requests
We love PRs! Here is the workflow we recommend:

1. Fork the repo and create your branch from `main`.
2. Test your changes. We have a high bar for stability.
3. Lint your code using `golangci-lint` or standard `go fmt`.
4. Submit that PR.

## Development Guidelines

### 1. Style & Linting
- We follow standard Go idioms (Effective Go).
- Run `gofmt -s -w .` before committing.
- Ensure your code passes the linter (if applicable) and has no ineffectual assignments.

### 2. Testing Requirements (Strict)
This library is used in high-throughput hot paths. We cannot accept PRs that break stability.

- Unit Tests: Any new logic must be covered by a unit test.
- Race Detection: Run tests with the race detector enabled.

```bash
go test -v -race ./...
```

- Allocations: If you modify the MemoryLimiter or the hot path of RedisLimiter, please run benchmarks to ensure no unnecessary allocations were added.

```bash
go test -bench=. -benchmem ./...
```

### 3. Documentation
- If you add a new public method, you must add a GoDoc comment.
- If you change the behavior of the limiter, please update the README.md.

## Commit Messages
We try to follow the Conventional Commits specification:

- feat: allow custom burst rates
- fix: handle redis connection timeouts
- docs: update readme with performance section
- chore: update dependencies

## License
By contributing, you agree that your contributions will be licensed under its MIT License.
