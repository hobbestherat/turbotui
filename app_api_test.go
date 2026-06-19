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

func TestTryPostNonBlocking(t *testing.T) {
	app := NewWithSize(10, 10, &bytes.Buffer{})
	noop := func() {}

	// Fill the 64-deep queue without ever blocking (TryPost never blocks).
	for i := 0; i < 64; i++ {
		if !app.TryPost(noop) {
			t.Fatalf("TryPost #%d returned false before the queue was full", i)
		}
	}
	// The queue is now full: TryPost must refuse rather than block.
	if app.TryPost(noop) {
		t.Fatal("TryPost returned true on a full queue; expected it to refuse")
	}
	// Drain one slot and the next TryPost should succeed again.
	<-app.postChannel
	if !app.TryPost(noop) {
		t.Fatal("TryPost returned false after draining a slot")
	}
}

func TestPostEnqueuesFn(t *testing.T) {
	app := NewWithSize(10, 10, &bytes.Buffer{})
	ran := false
	app.Post(func() { ran = true })

	// Simulate the event loop draining the queue.
	fn := <-app.postChannel
	fn()
	if !ran {
		t.Fatal("posted fn did not run when drained")
	}
}
