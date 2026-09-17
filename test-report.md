# Test report

`go test -race ./...` failed before any tests executed. The root package and `turbotv` both emitted this exact fatal message:

```text
FATAL: ThreadSanitizer: unsupported VMA range
FATAL: Found 47 - Supported 48
```

No test assertion failed, so there is no assertion message to report. In particular, this environment could not produce a race-enabled verdict for `TestConcurrentAddLayerKeepsEveryLayer`.

This looks like an environment/toolchain defect, not a defect in either the implementation or the test: ThreadSanitizer exits during startup because the host VMA range does not match the range it supports.

As a fallback, `go test -count=1 ./...` passed. `github.com/hobbestherat/turbotui` and `github.com/hobbestherat/turbotui/turbotv` were `ok`; all command packages reported `[no test files]`. This ordinary GREEN result is an early signal only and cannot establish that the race is fixed. The separate cluster gate remains authoritative.
