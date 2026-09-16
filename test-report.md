# Test report

Command run:

```text
go test ./...
```

Result: **GREEN**.

```text
ok  github.com/hobbestherat/turbotui          (cached)
ok  github.com/hobbestherat/turbotui/turbotv (cached)
```

The following packages reported `[no test files]`:

- `github.com/hobbestherat/turbotui/cmd/demo`
- `github.com/hobbestherat/turbotui/turbotv/cmd/chat`
- `github.com/hobbestherat/turbotui/turbotv/cmd/demo`
- `github.com/hobbestherat/turbotui/turbotv/cmd/tabs`

No tests failed, so there are no assertion messages and no failures to classify
as implementation or test defects. This cached local run is an early signal
only; the separate clean-tree cluster gate remains authoritative.
