//go:build !windows

package tui

import (
	"context"
	"os"
	"os/signal"
	"syscall"
)

// notifyResize subscribes ch to terminal-resize notifications and returns a
// stop function that unsubscribes. On Unix the kernel delivers SIGWINCH when the
// controlling terminal is resized; the Run loop reacts by re-querying the
// terminal size. ctx is unused on Unix.
func (a *App) notifyResize(_ context.Context, ch chan<- os.Signal) func() {
	signal.Notify(ch, syscall.SIGWINCH)
	return func() { signal.Stop(ch) }
}

// fatalSignals lists the signals Run traps so it can restore the terminal
// (leave raw mode / the alt screen) before the process dies.
func fatalSignals() []os.Signal {
	return []os.Signal{syscall.SIGTERM, syscall.SIGHUP, syscall.SIGINT}
}

// reraiseSignal re-raises sig with its default disposition after the terminal
// has already been restored, so the process dies exactly as the signal
// intended (correct exit status, parent shell job control, etc.).
func reraiseSignal(sig syscall.Signal) {
	_ = syscall.Kill(syscall.Getpid(), sig)
}

