# Test report

Command run:

```text
go test ./... -count=1
```

Result: **GREEN**.

Passing packages:

- `github.com/hobbestherat/turbotui`
- `github.com/hobbestherat/turbotui/turbotv`

Packages with no test files:

- `github.com/hobbestherat/turbotui/cmd/demo`
- `github.com/hobbestherat/turbotui/turbotv/cmd/chat`
- `github.com/hobbestherat/turbotui/turbotv/cmd/demo`
- `github.com/hobbestherat/turbotui/turbotv/cmd/tabs`

No tests failed, so there are no assertion messages to report and no implementation-versus-test defect to classify.

This was an uncached local run. It is an early positive signal only; the separately run cluster gate on the clean pushed commit remains authoritative, particularly for the task's required race-detector verdict.
