# Benchmark and verification evidence

This is a local measurement record, not a latency guarantee or an application
throughput estimate. The previous README's 76.4 ns/op, zero-allocation, sub-50 µs
Lua, and sub-1 ms Redis claims lacked a reproducible recorded context; they have
been replaced with executed evidence.

## Environment and command

Run date: 2026-10-03. Source: the working-tree changes in this PR based on
`8f00ea9`; the [environment record](benchmark-records/2026-10-03/environment.txt)
includes the base commit and dirty-file list. The Go source measured here is the
benchmark/test/example implementation in this PR.

- Go `go1.27.0`, Linux amd64, Ubuntu 24.04; kernel `7.0.0-34-generic`.
- Intel Core i7-8665U, four physical cores/eight logical CPUs; `GOMAXPROCS=4`.
- Redis `7.0.15`, standalone, local TCP `127.0.0.1:16379`, jemalloc 5.3.0,
  persistence disabled (`--save '' --appendonly no`), no TLS.
- go-redis `v9.17.3`; no-op metrics recorder, retries disabled in test clients.
- Shared development laptop with other workloads running. This is not an
  isolated performance lab; power state and load were not controlled.

The local Redis binary came from Ubuntu's Redis packages extracted into a
temporary directory, without a system installation. A container is an alternative:

```bash
docker run --rm -p 16379:6379 redis:7-alpine
REDIS_ADDR=127.0.0.1:16379 GOMAXPROCS=4 ./scripts/verify.sh
```

A `redis:7-alpine` image can have a different patch version and allocator from
this run. The test log records the actual Redis version. CI uses Go 1.24.x and
Redis 7; its artifacts are separate measurements, not directly interchangeable
with this Go 1.27 local run.

## Three measured samples per workload

Commands: `go test -run '^$' -bench . -benchmem -benchtime=1s -count=3 .`
with the environment above. [Full raw benchmark output](benchmark-records/2026-10-03/benchmarks.txt).

| Workload | ns/op, three samples | B/op | allocs/op |
| --- | --- | --- | --- |
| Memory, allowed | 190.8 / 182.6 / 181.5 | 16 | 1 |
| Memory, denied | 229.6 / 211.0 / 164.5 | 16 | 1 |
| Memory, parallel shared bucket | 228.6 / 200.5 / 202.4 | 16 | 1 |
| Redis, allowed, sequential | 78,464 / 72,371 / 86,828 | 1,279 | 23 |
| Redis, denied, sequential | 69,389 / 68,595 / 102,453 | 1,279 / 1,279 / 1,271 | 21 |

Each benchmark repeatedly checks one initialized identity. Allowed cases use a
large burst/high refill rate and assert allowance on every operation. Denied
cases consume the initial token first and use a one-day refill period. Bucket
creation and Redis script loading are outside the timed loop. The parallel
memory case uses `RunParallel` against one bucket with four workers by default.

These Redis numbers include client/network/response decoding and inline metrics
hooks. They do not measure Redis-side Lua execution time or p95/p99 HTTP request
latency. Redis parallel throughput, high-cardinality identities, failover, and
cross-host networks have not been benchmarked here. The memory implementation's
global mutex also serializes different identities.

## Profiles from separate runs

`verify.sh` executes CPU and heap profiling for the allowed/denied memory paths
and mutex profiling for the parallel shared bucket. Profile-run timings are not
included in the sample table.

- [CPU top](benchmark-records/2026-10-03/cpu-top.txt): 6.72 seconds of samples;
  `time.runtimeNow` accounts for 20.83% flat and `runtime.concatstrings` for
  11.31% flat. String concatenation and allocation are visible costs in this run.
- [Heap allocation top](benchmark-records/2026-10-03/heap-top.txt): sampled
  allocation space attributes 465.51 MB (99.31%) to `MemoryLimiter.Allow` across
  the profiling run. This is cumulative allocated space, not retained memory or
  a per-request measurement; the benchmark reports 16 B/op and one allocation.
- [Mutex top](benchmark-records/2026-10-03/mutex-top.txt): sampled aggregate mutex
  delay is 4.47 seconds, attributed to the lock's release in `MemoryLimiter.Allow`.
  Delay sums across workers and is not the latency of one request.

The script writes raw `.pprof` files and the test executable into `evidence/`.
They are ignored locally and uploaded with CI artifacts; the committed text
records above preserve this run's output. Inspect freshly generated profiles:

```bash
go tool pprof -http=localhost:8081 evidence/limiter.test evidence/cpu.pprof
go tool pprof -alloc_space evidence/limiter.test evidence/heap.pprof
go tool pprof evidence/limiter.test evidence/mutex.pprof
```

## Correctness checks executed

[Full race-test log](benchmark-records/2026-10-03/tests.txt) and
[ten-repeat contention output](benchmark-records/2026-10-03/contention.txt)
record successful checks with Redis available. No integration tests skipped.
`go vet ./...` completed with no findings.

The contention test starts 256 requests across eight independent client pools
sharing a burst of 37 and a one-day refill interval. It checks exactly 37
allowances, 219 denials, bounded remaining tokens, and positive retry hints.
The ten-repeat race run passed. This demonstrates shared-bucket admission
correctness under this workload; it does not establish performance under load.

Dependency tests first verify a healthy allowance, then exercise a closed client,
wrong Redis key type, or a deliberately missing script SHA. They check a zero
error decision, error counter tags, and error latency tags. The missing SHA models
script-cache loss without flushing a shared Redis instance. This is not a full
Redis restart/failover simulation. Pre-cancelled/expired contexts, constructor
dial failure, allowed/denied metric tags, and the example's fail-open behavior
are also checked.

The required-Redis guard was separately checked with
`REDIS_ADDR=127.0.0.1:1 REQUIRE_REDIS=1 go test -run '^TestRedisLimiter_Integration$' -count=1 .`.
It exited 1 with `Redis required ... connection refused`, confirming an
unreachable backend fails rather than skips.
