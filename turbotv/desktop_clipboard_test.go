package tv

import (
	"bytes"
	"encoding/base64"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	tui "github.com/hobbestherat/turbotui"
)

func osc52Payload(text string) string {
	return "\x1b]52;c;" + base64.StdEncoding.EncodeToString([]byte(text))
}

func newClipboardDesktop(t *testing.T) (*Desktop, *bytes.Buffer) {
	t.Helper()
	var output bytes.Buffer
	app := tui.NewWithSize(40, 10, &output)
	app.SetClipboardBackend(tui.ClipboardOSC52Only)
	return NewDesktop(app), &output
}

func installFakeClipboardReader(t *testing.T, name string, output string, exitCode int) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake clipboard command tests use POSIX shell scripts")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, name)
	script := "#!/bin/sh\n"
	if output != "" {
		script += "printf '%s' \"" + strings.ReplaceAll(output, "\"", "\\\"") + "\"\n"
	}
	if exitCode != 0 {
		script += "exit 1\n"
	}
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake clipboard reader: %v", err)
	}
	t.Setenv("PATH", dir)
}

func TestExportedCopyAndCutFocusedDelegateToFocusedWidget(t *testing.T) {
	desktop, output := newClipboardDesktop(t)
	widget := NewComponent(Rect{X: 0, Y: 0, W: 10, H: 1})
	widget.Focusable = true
	copyCalls := 0
	cutCalls := 0
	widget.CopyFn = func(*VisualComponent) (string, bool) {
		copyCalls++
		return "copy text", true
	}
	widget.CutFn = func(*VisualComponent) (string, bool) {
		cutCalls++
		return "cut text", true
	}
	desktop.AddLayer(NewFullscreenLayer("base", widget))
	desktop.SetFocus(widget)

	if !desktop.CopyFocused() {
		t.Fatalf("CopyFocused returned false for focused copyable widget")
	}
	if copyCalls != 1 {
		t.Fatalf("CopyFocused called CopyFn %d times, want 1", copyCalls)
	}
	if !strings.Contains(output.String(), osc52Payload("copy text")) {
		t.Fatalf("CopyFocused did not copy focused widget text, output %q", output.String())
	}

	output.Reset()
	if !desktop.CutFocused() {
		t.Fatalf("CutFocused returned false for focused cuttable widget")
	}
	if cutCalls != 1 {
		t.Fatalf("CutFocused called CutFn %d times, want 1", cutCalls)
	}
	if !strings.Contains(output.String(), osc52Payload("cut text")) {
		t.Fatalf("CutFocused did not copy cut text, output %q", output.String())
	}
}

func TestExportedCopyAndCutFocusedNoopWithoutFocusedCapability(t *testing.T) {
	desktop, output := newClipboardDesktop(t)
	if desktop.CopyFocused() {
		t.Fatalf("CopyFocused returned true with no focused widget")
	}
	if desktop.CutFocused() {
		t.Fatalf("CutFocused returned true with no focused widget")
	}
	if output.Len() != 0 {
		t.Fatalf("clipboard output written for no-op copy/cut: %q", output.String())
	}

	widget := NewComponent(Rect{X: 0, Y: 0, W: 10, H: 1})
	widget.Focusable = true
	desktop.AddLayer(NewFullscreenLayer("base", widget))
	desktop.SetFocus(widget)
	if desktop.CopyFocused() {
		t.Fatalf("CopyFocused returned true without CopyFn")
	}
	if desktop.CutFocused() {
		t.Fatalf("CutFocused returned true without CutFn")
	}
}

func TestExportedPasteReadsClipboardAndBubblesToFocusedWidget(t *testing.T) {
	installFakeClipboardReader(t, "pbpaste", "from clipboard", 0)
	desktop, _ := newClipboardDesktop(t)
	root := NewComponent(Rect{X: 0, Y: 0, W: 40, H: 10})
	child := NewComponent(Rect{X: 1, Y: 1, W: 10, H: 1})
	child.Focusable = true
	root.AddChild(child)
	var pasted []string
	root.OnPasteFn = func(_ *VisualComponent, text string) bool {
		pasted = append(pasted, text)
		return true
	}
	desktop.AddLayer(NewFullscreenLayer("base", root))
	desktop.SetFocus(child)

	if !desktop.Paste() {
		t.Fatalf("Paste returned false with a focused widget and readable clipboard")
	}
	if len(pasted) != 1 || pasted[0] != "from clipboard" {
		t.Fatalf("Paste bubbled %v, want one clipboard payload", pasted)
	}
}

func TestExportedPasteNoopsOnMissingOrEmptyClipboardRead(t *testing.T) {
	desktop, _ := newClipboardDesktop(t)
	t.Setenv("PATH", t.TempDir())
	widget := NewComponent(Rect{X: 0, Y: 0, W: 10, H: 1})
	widget.Focusable = true
	pasteCalls := 0
	widget.OnPasteFn = func(_ *VisualComponent, _ string) bool {
		pasteCalls++
		return true
	}
	desktop.AddLayer(NewFullscreenLayer("base", widget))
	desktop.SetFocus(widget)

	if desktop.Paste() {
		t.Fatalf("Paste returned true with no clipboard reader")
	}
	if pasteCalls != 0 {
		t.Fatalf("Paste called OnPasteFn %d times after read failure, want 0", pasteCalls)
	}

	installFakeClipboardReader(t, "pbpaste", "", 0)
	if desktop.Paste() {
		t.Fatalf("Paste returned true for empty clipboard output")
	}
	if pasteCalls != 0 {
		t.Fatalf("Paste called OnPasteFn %d times for empty clipboard, want 0", pasteCalls)
	}
}

func TestCtrlVPastesClipboardBeforeFocusedTypeAndFallthrough(t *testing.T) {
	installFakeClipboardReader(t, "pbpaste", "ctrl-v payload", 0)
	desktop, _ := newClipboardDesktop(t)
	widget := NewComponent(Rect{X: 0, Y: 0, W: 10, H: 1})
	widget.Focusable = true
	pasted := ""
	typed := 0
	fallthroughCalls := 0
	widget.OnPasteFn = func(_ *VisualComponent, text string) bool {
		pasted = text
		return true
	}
	widget.OnTypeFn = func(_ *VisualComponent, _ tui.TypeEvent) bool {
		typed++
		return true
	}
	desktop.SetUnhandledKeyFn(func(tui.TypeEvent) { fallthroughCalls++ })
	desktop.AddLayer(NewFullscreenLayer("base", widget))
	desktop.SetFocus(widget)

	desktop.handleType(tui.TypeEvent{Key: tui.KeyRune, Rune: 'V', Ctrl: true})

	if pasted != "ctrl-v payload" {
		t.Fatalf("Ctrl+V pasted %q, want clipboard payload", pasted)
	}
	if typed != 0 {
		t.Fatalf("Ctrl+V reached focused OnTypeFn %d times, want 0", typed)
	}
	if fallthroughCalls != 0 {
		t.Fatalf("Ctrl+V reached fallthrough handler %d times, want 0", fallthroughCalls)
	}
}

func TestCtrlVReadFailureIsConsumedAsNoop(t *testing.T) {
	desktop, _ := newClipboardDesktop(t)
	t.Setenv("PATH", t.TempDir())
	widget := NewComponent(Rect{X: 0, Y: 0, W: 10, H: 1})
	widget.Focusable = true
	typed := 0
	fallthroughBinding := 0
	unhandled := 0
	widget.OnTypeFn = func(_ *VisualComponent, _ tui.TypeEvent) bool {
		typed++
		return true
	}
	desktop.ScopedBindings().Register(
		KeyBinding{Chord: Chord{Key: tui.KeyRune, Rune: 'v', Ctrl: true}, Scope: ScopeFallthrough},
		func() bool {
			fallthroughBinding++
			return true
		},
	)
	desktop.SetUnhandledKeyFn(func(tui.TypeEvent) { unhandled++ })
	desktop.AddLayer(NewFullscreenLayer("base", widget))
	desktop.SetFocus(widget)

	desktop.handleType(tui.TypeEvent{Key: tui.KeyRune, Rune: 'v', Ctrl: true})

	if typed != 0 {
		t.Fatalf("Ctrl+V read failure reached focused OnTypeFn %d times, want 0", typed)
	}
	if fallthroughBinding != 0 {
		t.Fatalf("Ctrl+V read failure reached fallthrough binding %d times, want 0", fallthroughBinding)
	}
	if unhandled != 0 {
		t.Fatalf("Ctrl+V read failure reached unhandled-key handler %d times, want 0", unhandled)
	}
}

func TestCtrlCAndCtrlXStillUseExistingFocusedClipboardPaths(t *testing.T) {
	desktop, output := newClipboardDesktop(t)
	widget := NewComponent(Rect{X: 0, Y: 0, W: 10, H: 1})
	widget.Focusable = true
	widget.CopyFn = func(*VisualComponent) (string, bool) { return "copied", true }
	widget.CutFn = func(*VisualComponent) (string, bool) { return "cut", true }
	desktop.AddLayer(NewFullscreenLayer("base", widget))
	desktop.SetFocus(widget)

	desktop.handleType(tui.TypeEvent{Key: tui.KeyRune, Rune: 'C', Ctrl: true})
	if !strings.Contains(output.String(), osc52Payload("copied")) {
		t.Fatalf("Ctrl+C did not copy focused widget text, output %q", output.String())
	}

	output.Reset()
	desktop.handleType(tui.TypeEvent{Key: tui.KeyRune, Rune: 'X', Ctrl: true})
	if !strings.Contains(output.String(), osc52Payload("cut")) {
		t.Fatalf("Ctrl+X did not cut focused widget text, output %q", output.String())
	}
}

func TestBracketedPasteStillRoutesLiteralTextWithoutClipboardReader(t *testing.T) {
	desktop, _ := newClipboardDesktop(t)
	t.Setenv("PATH", t.TempDir())
	widget := NewComponent(Rect{X: 0, Y: 0, W: 10, H: 1})
	widget.Focusable = true
	pasted := ""
	widget.OnPasteFn = func(_ *VisualComponent, text string) bool {
		pasted = text
		return true
	}
	desktop.AddLayer(NewFullscreenLayer("base", widget))
	desktop.SetFocus(widget)

	desktop.handlePaste(tui.PasteEvent{Text: "literal\npaste"})

	if pasted != "literal\npaste" {
		t.Fatalf("bracketed paste routed %q, want literal text", pasted)
	}
}
