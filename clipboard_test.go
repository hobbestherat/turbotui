package tui

import (
	"bytes"
	"encoding/base64"
	"strings"
	"testing"
)

// stubNativeCopy replaces the native-clipboard hook for the duration of a test,
// recording the texts it would have copied, and restores the original after.
func stubNativeCopy(t *testing.T) *[]string {
	t.Helper()
	original := nativeCopy
	calls := []string{}
	nativeCopy = func(text string) {
		calls = append(calls, text)
	}
	t.Cleanup(func() { nativeCopy = original })
	return &calls
}

// clearSSHEnv makes the process look local regardless of how the test runner was
// launched (the test machine itself may be reached over SSH).
func clearSSHEnv(t *testing.T) {
	t.Helper()
	t.Setenv("SSH_CONNECTION", "")
	t.Setenv("SSH_CLIENT", "")
	t.Setenv("SSH_TTY", "")
}

func osc52(text string) string {
	return "\x1b]52;c;" + base64.StdEncoding.EncodeToString([]byte(text))
}

// TestCopyToClipboardDoesNotRepaint covers issue #18: a copy must not invalidate
// the front buffer, so a subsequent Apply with no grid changes writes nothing.
func TestCopyToClipboardDoesNotRepaint(t *testing.T) {
	clearSSHEnv(t)
	stubNativeCopy(t)

	var buf bytes.Buffer
	app := NewWithSize(20, 5, &buf)
	app.Clear(DefaultCell())
	app.WriteString(0, 0, "hello", DefaultCell())
	if err := app.Apply(); err != nil {
		t.Fatalf("first apply: %v", err)
	}

	app.CopyToClipboard("hello")

	// Nothing on the grid changed, so the next Apply must be a no-op write. If
	// the copy had invalidated the front buffer it would repaint every cell.
	buf.Reset()
	if err := app.Apply(); err != nil {
		t.Fatalf("second apply: %v", err)
	}
	if buf.Len() != 0 {
		t.Fatalf("expected no repaint after copy, got %q", buf.String())
	}
}

// TestCopyToClipboardSkipsNativeOverSSH covers issue #67: over SSH the native
// command would target the remote host, so only OSC 52 should be used.
func TestCopyToClipboardSkipsNativeOverSSH(t *testing.T) {
	clearSSHEnv(t)
	t.Setenv("SSH_CONNECTION", "10.0.0.1 222 10.0.0.2 22")
	calls := stubNativeCopy(t)

	var buf bytes.Buffer
	app := NewWithSize(20, 5, &buf)
	app.CopyToClipboard("secret")

	if len(*calls) != 0 {
		t.Fatalf("native clipboard must not run over SSH, got calls %v", *calls)
	}
	if !strings.Contains(buf.String(), osc52("secret")) {
		t.Fatalf("expected OSC 52 over SSH, got %q", buf.String())
	}
}

// TestCopyToClipboardSSHDetection table-tests each SSH indicator variable.
func TestCopyToClipboardSSHDetection(t *testing.T) {
	for _, env := range []string{"SSH_CONNECTION", "SSH_CLIENT", "SSH_TTY"} {
		t.Run(env, func(t *testing.T) {
			clearSSHEnv(t)
			t.Setenv(env, "set")
			calls := stubNativeCopy(t)

			var buf bytes.Buffer
			app := NewWithSize(20, 5, &buf)
			app.CopyToClipboard("x")
			if len(*calls) != 0 {
				t.Fatalf("%s should suppress native copy, got %v", env, *calls)
			}
		})
	}
}

// TestCopyToClipboardNativeWhenLocal covers the local default: both OSC 52 and
// the native fallback run.
func TestCopyToClipboardNativeWhenLocal(t *testing.T) {
	clearSSHEnv(t)
	calls := stubNativeCopy(t)

	var buf bytes.Buffer
	app := NewWithSize(20, 5, &buf)
	app.CopyToClipboard("local")

	if !strings.Contains(buf.String(), osc52("local")) {
		t.Fatalf("expected OSC 52 locally, got %q", buf.String())
	}
	if len(*calls) != 1 || (*calls)[0] != "local" {
		t.Fatalf("expected one native copy of %q, got %v", "local", *calls)
	}
}

// TestClipboardBackend covers issue #67/#69: the backend selector controls which
// mechanisms fire.
func TestClipboardBackend(t *testing.T) {
	tests := []struct {
		name       string
		backend    ClipboardBackend
		wantOSC52  bool
		wantNative bool
	}{
		{"default", ClipboardOSC52AndNative, true, true},
		{"osc52-only", ClipboardOSC52Only, true, false},
		{"native-only", ClipboardNativeOnly, false, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			clearSSHEnv(t)
			calls := stubNativeCopy(t)

			var buf bytes.Buffer
			app := NewWithSize(20, 5, &buf)
			app.SetClipboardBackend(tc.backend)
			app.CopyToClipboard("data")

			gotOSC52 := strings.Contains(buf.String(), osc52("data"))
			if gotOSC52 != tc.wantOSC52 {
				t.Errorf("OSC 52 emitted=%v, want %v (output %q)", gotOSC52, tc.wantOSC52, buf.String())
			}
			gotNative := len(*calls) > 0
			if gotNative != tc.wantNative {
				t.Errorf("native called=%v, want %v", gotNative, tc.wantNative)
			}
		})
	}
}

// TestClipboardNativeOnlyStillSuppressedOverSSH ensures NativeOnly does not
// resurrect the remote-host copy bug over SSH.
func TestClipboardNativeOnlyStillSuppressedOverSSH(t *testing.T) {
	clearSSHEnv(t)
	t.Setenv("SSH_TTY", "/dev/pts/0")
	calls := stubNativeCopy(t)

	var buf bytes.Buffer
	app := NewWithSize(20, 5, &buf)
	app.SetClipboardBackend(ClipboardNativeOnly)
	app.CopyToClipboard("data")

	if len(*calls) != 0 {
		t.Fatalf("native-only must still be suppressed over SSH, got %v", *calls)
	}
	if strings.Contains(buf.String(), "\x1b]52") {
		t.Fatalf("native-only must not emit OSC 52, got %q", buf.String())
	}
}

// TestCopyToClipboardLargePayloadSkipsOSC52 covers issue #69: an oversized
// payload is not emitted as a single giant OSC 52 sequence; the native backend
// (local) still receives the full text.
func TestCopyToClipboardLargePayloadSkipsOSC52(t *testing.T) {
	clearSSHEnv(t)
	calls := stubNativeCopy(t)

	// Raw length chosen so its base64 encoding exceeds maxOSC52Bytes.
	big := strings.Repeat("a", maxOSC52Bytes) // encodes to ~4/3 * len > cap

	var buf bytes.Buffer
	app := NewWithSize(20, 5, &buf)
	app.CopyToClipboard(big)

	if strings.Contains(buf.String(), "\x1b]52") {
		t.Fatalf("oversized payload must not emit OSC 52")
	}
	if len(*calls) != 1 || (*calls)[0] != big {
		t.Fatalf("native backend should still receive the full text")
	}
}

// TestCopyToClipboardPayloadAtCapIsEmitted confirms the cap is inclusive at the
// boundary: a payload whose encoding is exactly at the limit is still sent.
func TestCopyToClipboardPayloadAtCapIsEmitted(t *testing.T) {
	clearSSHEnv(t)
	stubNativeCopy(t)

	// 3 raw bytes -> 4 base64 bytes; pick a length encoding to exactly maxOSC52Bytes.
	rawLen := maxOSC52Bytes / 4 * 3
	text := strings.Repeat("a", rawLen)
	if len(base64.StdEncoding.EncodeToString([]byte(text))) != maxOSC52Bytes {
		t.Skipf("boundary length not exact for cap %d", maxOSC52Bytes)
	}

	var buf bytes.Buffer
	app := NewWithSize(20, 5, &buf)
	app.CopyToClipboard(text)
	if !strings.Contains(buf.String(), "\x1b]52") {
		t.Fatalf("payload at cap should be emitted")
	}
}
