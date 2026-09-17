# Test report

`go test -race ./...` failed before the tests could run. Both `github.com/hobbestherat/turbotui` and `github.com/hobbestherat/turbotui/turbotv` reported the exact runtime error:

```text
FATAL: ThreadSanitizer: unsupported VMA range
FATAL: Found 47 - Supported 48
```

There was no test assertion failure, so no assertion message is available and the race-sensitive result of `TestConcurrentAddLayerKeepsEveryLayer` could not be observed in this environment. This looks like a test-environment/toolchain incompatibility, not a defect in either the implementation or the test, because ThreadSanitizer terminates before executing tests.

As a fallback, `go test ./...` passed. Packages `github.com/hobbestherat/turbotui` and `github.com/hobbestherat/turbotui/turbotv` were `ok`; the remaining command packages had no test files. This ordinary GREEN run is only an early signal and does not establish that the data race is fixed; the cluster gate remains authoritative.
