package tui

import (
	"encoding/base64"
	"io"
	"os"
	"os/exec"
	"runtime"
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
