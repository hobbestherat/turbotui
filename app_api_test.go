package tui

import (
	"bytes"
	"errors"
	"os"
	"testing"
)

// failingWriter is an io.Writer whose writes always fail, simulating a closed
// or broken terminal output (e.g. a redirected stdout whose pipe is gone).
type failingWriter struct{}

func (failingWriter) Write(p []byte) (int, error) {
	return 0, errors.New("write: broken pipe")
}

func TestNewReturnsAppWithoutError(t *testing.T) {
	// New() no longer returns an error; an unreadable size falls back to 80x25.
	app := New()
	if app == nil {
		t.Fatal("New() returned nil")
	}
	if app.Width() < 1 || app.Height() < 1 {
		t.Fatalf("New() produced invalid size %dx%d", app.Width(), app.Height())
	}
}

func TestValidateNilForBufferBackedApp(t *testing.T) {
	// NewWithSize apps have no terminal by design; Validate is a no-op for them.
	app := NewWithSize(80, 25, &bytes.Buffer{})
	if err := app.Validate(); err != nil {
		t.Fatalf("buffer-backed app should validate clean, got %v", err)
	}
}

func TestValidateRejectsNonTerminal(t *testing.T) {
	tmp, err := os.CreateTemp("", "turbotui-*")
	if err != nil {
		t.Fatalf("create temp: %v", err)
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()

	app := NewWithIO(tmp, tmp, 80, 25)
	if err := app.Validate(); err == nil {
		t.Fatal("expected Validate to reject a non-terminal stdin/stdout")
	}
}

func TestApplyErrorIsRecordedAndReported(t *testing.T) {
	app := NewWithSize(10, 1, failingWriter{})
	called := 0
	app.OnApplyError(func(err error) {
		called++
		if err == nil {
			t.Fatal("OnApplyError received nil error")
		}
	})

	app.Clear(DefaultCell())
	app.WriteCell(0, 0, Cell{Ch: 'X', FG: ANSIColor(15), BG: ANSIColor(0)})

	err := app.Apply()
	if err == nil {
		t.Fatal("expected Apply to return the write error")
	}
	if !errors.Is(app.LastApplyError(), err) {
		t.Fatalf("LastApplyError = %v; want %v (the error Run would exit on)", app.LastApplyError(), err)
	}
	if called != 1 {
		t.Fatalf("expected OnApplyError callback once, got %d", called)
	}
}

func TestApplySuccessDoesNotReport(t *testing.T) {
	var output bytes.Buffer
	app := NewWithSize(10, 1, &output)
	called := 0
	app.OnApplyError(func(error) { called++ })

	app.Clear(DefaultCell())
	app.WriteCell(0, 0, Cell{Ch: 'Y', FG: ANSIColor(15), BG: ANSIColor(0)})
	if err := app.Apply(); err != nil {
		t.Fatalf("unexpected Apply error: %v", err)
	}
	if app.LastApplyError() != nil {
		t.Fatalf("expected no apply error on success, got %v", app.LastApplyError())
	}
	if called != 0 {
		t.Fatalf("OnApplyError should not fire on success, got %d", called)
	}
}

func TestTryPostNeverBlocks(t *testing.T) {
	app := NewWithSize(10, 10, &bytes.Buffer{})
	noop := func() {}

	// The mailbox is unbounded, so far more than the old 64-deep limit can be
	// enqueued without ever blocking, and TryPost always succeeds.
	for i := 0; i < 200; i++ {
		if !app.TryPost(noop) {
			t.Fatalf("TryPost #%d returned false; the mailbox should be unbounded", i)
		}
	}
}

func TestPostDeliversInOrder(t *testing.T) {
	app := NewWithSize(10, 10, &bytes.Buffer{})
	var order []int
	for i := 0; i < 5; i++ {
		i := i
		app.Post(func() { order = append(order, i) })
	}
	// Simulate the event loop draining the mailbox.
	app.drainPosts()
	if len(order) != 5 {
		t.Fatalf("expected 5 posted fns to run, got %d", len(order))
	}
	for i, v := range order {
		if v != i {
			t.Fatalf("posts ran out of order: %v", order)
		}
	}
}

// TestReentrantPostDoesNotDeadlock covers issue #20: a posted closure that itself
// calls Post (from the loop goroutine) must not deadlock and its re-posted work
// must still run within the same drain.
func TestReentrantPostDoesNotDeadlock(t *testing.T) {
	app := NewWithSize(10, 10, &bytes.Buffer{})
	inner := false
	app.Post(func() {
		app.Post(func() { inner = true })
	})
	app.drainPosts()
	if !inner {
		t.Fatal("re-entrant Post did not run; drain stopped early or deadlocked")
	}
}
