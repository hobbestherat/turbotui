# Test report

`go test -race ./...` failed before any tests executed. The root package and `turbotv` both emitted the exact runtime failure:

```text
FATAL: ThreadSanitizer: unsupported VMA range
FATAL: Found 47 - Supported 48
```

No test assertion failed, so there is no assertion message to report. In particular, this environment could not produce a race-enabled verdict for `TestConcurrentAddLayerKeepsEveryLayer`. The failure is an environment/toolchain incompatibility rather than a defect in the implementation or the test: ThreadSanitizer exits during startup because the host VMA range does not match the range it supports.

As a fallback, an uncached `go test -count=1 ./...` passed. `github.com/hobbestherat/turbotui` and `github.com/hobbestherat/turbotui/turbotv` were `ok`; all command packages reported `[no test files]`. This ordinary GREEN result is an early signal only and cannot establish that the race is fixed. The separate cluster gate remains authoritative.
