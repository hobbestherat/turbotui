package tui

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"time"
)

// ClipboardBackend selects how CopyToClipboard delivers text to the system
// clipboard.
type ClipboardBackend int

const (
	// ClipboardOSC52AndNative writes via OSC 52 and, when running locally (i.e.
	// not over SSH), also pipes the text to a native clipboard command as a
	// fallback for terminals that ignore OSC 52. This is the default.
	ClipboardOSC52AndNative ClipboardBackend = iota
	// ClipboardOSC52Only writes only via OSC 52. Use it when the native
	// fallback is unwanted, or to make copy behaviour identical local and remote.
	ClipboardOSC52Only
	// ClipboardNativeOnly writes only via a native clipboard command (and only
	// when running locally). Use it to fully disable OSC 52, e.g. when the
	// terminal or operator has it switched off for security.
	ClipboardNativeOnly
)

// maxOSC52Bytes caps the size of an OSC 52 payload (counted on the base64
// encoding actually sent). OSC 52 carries the whole clipboard inline in a single
// escape sequence; multi-megabyte sequences are truncated or choked on by many
// terminals, so an oversized copy skips OSC 52 and relies on the native backend
// (when local) instead of emitting a sequence the terminal will mangle.
const maxOSC52Bytes = 1 << 20 // 1 MiB of base64

// SetClipboardBackend selects how CopyToClipboard delivers text. The default
// (ClipboardOSC52AndNative) writes OSC 52 and adds a native fallback only when
// running locally. Pass ClipboardOSC52Only to drop the native fallback, or
// ClipboardNativeOnly to disable OSC 52 entirely.
//
// Like CopyToClipboard, this should be configured on the event-loop goroutine
// (typically during setup, before Run).
func (a *App) SetClipboardBackend(backend ClipboardBackend) {
	a.clipboardBackend = backend
}

// CopyToClipboard puts text on the system clipboard. By default it writes an
// OSC 52 escape (which reaches the clipboard through most terminals and, unlike
// a local command, over SSH) and, as a best-effort fallback for terminals that
// ignore OSC 52 (notably macOS Terminal.app), also pipes the text to a native
// clipboard command — but only when running locally, never over SSH, where the
// native command would write the *remote* host's clipboard. Use
// SetClipboardBackend to change this policy.
//
// CopyToClipboard must be called on the event-loop goroutine (the desktop does
// this for you). From a background goroutine, schedule it with App.Post; calling
// it directly off the loop is guarded against corrupting the output stream but is
// still not part of the supported contract.
func (a *App) CopyToClipboard(text string) {
	backend := a.clipboardBackend

	if backend != ClipboardNativeOnly {
		encoded := base64.StdEncoding.EncodeToString([]byte(text))
		if len(encoded) <= maxOSC52Bytes {
			a.writeMu.Lock()
			_, _ = io.WriteString(a.out, "\x1b]52;c;"+encoded+"\a")
			a.writeMu.Unlock()
		}
	}

	// OSC 52 is self-contained: it writes the clipboard without touching the
	// grid, cursor or SGR, so there is no reason to invalidate the front buffer
	// and force a full repaint.

	if backend != ClipboardOSC52Only && !runningOverSSH() {
		nativeCopy(text)
	}
}

// nativeCopy is the native-clipboard hook, indirected through a variable so
// tests can observe whether the native backend was invoked without shelling out.
var nativeCopy = nativeClipboardCopy

// runningOverSSH reports whether the process appears to be running over an SSH
// connection, in which case a native clipboard command would target the remote
// host rather than the user's local terminal.
func runningOverSSH() bool {
	return os.Getenv("SSH_CONNECTION") != "" ||
		os.Getenv("SSH_CLIENT") != "" ||
		os.Getenv("SSH_TTY") != ""
}

// nativeClipboardCopy pipes text to the platform clipboard tool if present. It is
// best-effort: any error (tool missing) is silently ignored, because OSC 52
// already covers those cases. Callers must ensure this is not invoked over SSH.
func nativeClipboardCopy(text string) {
	name, args := clipboardCommand()
	if name == "" {
		return
	}
	go func() {
		cmd := exec.Command(name, args...)
		stdin, err := cmd.StdinPipe()
		if err != nil {
			return
		}
		if err := cmd.Start(); err != nil {
			return
		}
		_, _ = io.WriteString(stdin, text)
		_ = stdin.Close()
		_ = cmd.Wait()
	}()
}

// errNoClipboardReader is returned by ReadClipboard when no native clipboard-read
// command is present on the host at all. It is distinct from the error returned
// when a reader was found but failed (which wraps that backend's own error), so a
// caller can tell "nothing installed" from "installed but broke". Either way the
// Ctrl+V / Paste path treats a non-nil error as "nothing to paste" and no-ops.
var errNoClipboardReader = errors.New("turbotui: no clipboard reader available")

// clipboardReadCmd is one native clipboard-read backend: the command name looked
// up on PATH plus the arguments that make it print the clipboard to stdout.
type clipboardReadCmd struct {
	name string
	args []string
}

// clipboardReaders lists the native clipboard-read backends ReadClipboard tries,
// in priority order; the first one present on PATH whose invocation succeeds
// wins. It is a variable (not a const) so tests can inject fakes. OSC 52 has a
// read-query form (ESC ] 52 ; c ; ?) but it is unreliable and unsupported across
// many terminals, so it is intentionally omitted in favour of these commands.
var clipboardReaders = []clipboardReadCmd{
	{name: "pbpaste"},  // macOS
	{name: "wl-paste"}, // Wayland
	{name: "xclip", args: []string{"-selection", "clipboard", "-o"}}, // X11
	{name: "xsel", args: []string{"-b", "-o"}},                       // X11 (xsel)
}

// clipboardLookPath resolves a clipboard-read command on PATH. It is indirected
// through a variable so tests can control which backends appear "available"
// without depending on the host's installed tools.
var clipboardLookPath = exec.LookPath

// clipboardReadRun runs a resolved clipboard-read command and returns its stdout.
// It is indirected through a variable so tests can inject a fake runner. The
// default bounds the call with a short timeout so a wedged clipboard tool cannot
// hang the event loop that drives a synchronous paste.
var clipboardReadRun = func(name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return exec.CommandContext(ctx, name, args...).Output()
}

// ReadClipboard returns the current system clipboard text on a best-effort basis.
// It tries the native clipboard-read commands in clipboardReaders order (pbpaste,
// wl-paste, xclip, xsel), returning the stdout of the first that is present on
// PATH and exits cleanly. The output is returned verbatim (no trimming) so the
// read is faithful to what the backend reported.
//
// Reading the clipboard from a terminal is not universally possible — OSC 52 is a
// write-oriented protocol and its read-query is unreliable — so ReadClipboard is
// the paste counterpart to CopyToClipboard's native fallback, not a guaranteed
// channel. It never panics or blocks indefinitely. When no reader backend is
// present at all it returns errNoClipboardReader; when one or more backends were
// found but every invocation failed it returns the last failure (wrapped), so the
// two cases are distinguishable. The Ctrl+V / Paste path treats any non-nil error
// as "nothing to paste" and no-ops gracefully.
func (a *App) ReadClipboard() (string, error) {
	var lastErr error
	for _, r := range clipboardReaders {
		path, err := clipboardLookPath(r.name)
		if err != nil {
			continue
		}
		out, err := clipboardReadRun(path, r.args...)
		if err != nil {
			lastErr = err
			continue
		}
		return string(out), nil
	}
	if lastErr != nil {
		return "", fmt.Errorf("turbotui: clipboard read failed: %w", lastErr)
	}
	return "", errNoClipboardReader
}

func clipboardCommand() (string, []string) {
	switch runtime.GOOS {
	case "darwin":
		if path, err := exec.LookPath("pbcopy"); err == nil {
			return path, nil
		}
	case "windows":
		if path, err := exec.LookPath("clip"); err == nil {
			return path, nil
		}
	default:
		if path, err := exec.LookPath("wl-copy"); err == nil {
			return path, nil
		}
		if path, err := exec.LookPath("xclip"); err == nil {
			return path, []string{"-selection", "clipboard"}
		}
		if path, err := exec.LookPath("xsel"); err == nil {
			return path, []string{"--clipboard", "--input"}
		}
	}
	return "", nil
}
