# Test report

`go test -race ./...` failed before any test executed. Both `github.com/hobbestherat/turbotui` and `github.com/hobbestherat/turbotui/turbotv` emitted this exact fatal message:

```text
FATAL: ThreadSanitizer: unsupported VMA range
FATAL: Found 47 - Supported 48
```

No test assertion failed, so there is no assertion message to report. In particular, this host could not produce a race-enabled verdict for `TestConcurrentAddLayerKeepsEveryLayer`.

This is an environment/toolchain failure, not evidence of a defect in the implementation or the test: ThreadSanitizer aborts during startup because the host VMA range does not match the range supported by this race runtime.

As a fallback, the uncached ordinary suite, `go test -count=1 ./...`, passed. The root and `turbotv` packages were `ok`; all command packages reported `[no test files]`. This ordinary GREEN result is only an early signal and cannot establish that the data race is fixed. The separate clean-tree cluster gate remains authoritative.
