#!/usr/bin/env bash
set -euo pipefail

# Run from any directory; use a dedicated Redis instance for test keys.
cd "$(dirname "$0")/.."
export REQUIRE_REDIS=1
export REDIS_ADDR="${REDIS_ADDR:-localhost:6379}"
export GOMAXPROCS="${GOMAXPROCS:-4}"
mkdir -p evidence
{
  date -u '+%Y-%m-%dT%H:%M:%SZ'
  git rev-parse HEAD
  git status --short
  go version
  go env GOOS GOARCH
  go list -m github.com/redis/go-redis/v9
  uname -srmo
  if command -v lscpu >/dev/null; then
    lscpu | awk '/^(Architecture:|CPU\(s\):|Model name:|Thread\(s\) per core:|Core\(s\) per socket:|Socket\(s\):)/'
  fi
  echo "REDIS_ADDR=$REDIS_ADDR GOMAXPROCS=$GOMAXPROCS"
} > evidence/environment.txt

go vet ./... 2>&1 | tee evidence/vet.txt
go test -v -race -count=1 ./... 2>&1 | tee evidence/tests.txt
go test -run '^TestRedisLimiter_ConcurrentSharedBucket$' -race -count=10 . 2>&1 | tee evidence/contention.txt
go test -run '^$' -bench . -benchmem -benchtime=1s -count=3 . 2>&1 | tee evidence/benchmarks.txt
# Profiles use separate runs; their timings are not mixed into benchmark claims.
go test -run '^$' -bench '^BenchmarkMemoryLimiter_Decisions$' -benchtime=2s -cpuprofile=evidence/cpu.pprof -memprofile=evidence/heap.pprof -o evidence/limiter.test . 2>&1 | tee evidence/profile-run.txt
go tool pprof -top evidence/limiter.test evidence/cpu.pprof > evidence/cpu-top.txt
go tool pprof -top -alloc_space evidence/limiter.test evidence/heap.pprof > evidence/heap-top.txt
go test -run '^$' -bench '^BenchmarkMemoryLimiter_ParallelSharedBucket$' -benchtime=2s -mutexprofile=evidence/mutex.pprof -o evidence/limiter.test . 2>&1 | tee evidence/mutex-run.txt
go tool pprof -top evidence/limiter.test evidence/mutex.pprof > evidence/mutex-top.txt
