//go:build windows

package tui

import (
	"context"
	"os"
	"syscall"
	"time"

	"golang.org/x/term"
)

// resizePollSignal is the os.Signal value the Windows resize poller pushes onto
// the resize channel. Its concrete type is irrelevant to the Run loop, which
// only needs a wakeup to re-query the terminal size.
type resizePollSignal struct{}

func (resizePollSignal) String() string { return "resize" }
func (resizePollSignal) Signal()        {}

// resizePollInterval is how often the Windows poller samples the console size.
// 50ms is responsive to the eye while costing a single cheap syscall per tick.
const resizePollInterval = 50 * time.Millisecond

// notifyResize starts a background poller that watches the console window size
// and pushes a wakeup onto ch whenever it changes, returning a stop function
// that tears the poller down. Windows does not deliver terminal-resize as a
// signal (there is no SIGWINCH), and reading WINDOW_BUFFER_SIZE_EVENT from the
// console input handle would steal key events from the input reader, so polling
// the output handle's size is the safe, non-intrusive approach.
func (a *App) notifyResize(ctx context.Context, ch chan<- os.Signal) func() {
	if a.termOut == nil {
		return func() {}
	}
	fd := int(a.termOut.Fd())
	stop := make(chan struct{})
	go func() {
		ticker := time.NewTicker(resizePollInterval)
		defer ticker.Stop()
		lastW, lastH, _ := term.GetSize(fd)
		for {
			select {
			case <-ctx.Done():
				return
			case <-stop:
				return
			case <-ticker.C:
				w, h, err := term.GetSize(fd)
				if err != nil || (w == lastW && h == lastH) {
					continue
				}
				lastW, lastH = w, h
				// Non-blocking: a pending wakeup already covers this change.
				select {
				case ch <- resizePollSignal{}:
				default:
				}
			}
		}
	}()
	return func() { close(stop) }
}

// fatalSignals lists the signals Run traps so it can restore the terminal
// before the process dies. Go on Windows synthesises SIGINT/SIGTERM from
// console control events; SIGHUP is never delivered, so it is omitted.
func fatalSignals() []os.Signal {
	return []os.Signal{syscall.SIGINT, syscall.SIGTERM}
}

// reraiseSignal cannot re-raise a signal with its default disposition on
// Windows (there is no syscall.Kill). The terminal has already been restored by
// the caller, so Run simply returns and the process exits normally.
func reraiseSignal(syscall.Signal) {}

