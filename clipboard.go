package tui

import (
	"encoding/base64"
	"io"
	"os/exec"
	"runtime"
)

// CopyToClipboard puts text on the system clipboard. It writes an OSC 52 escape
// (which reaches the clipboard through most terminals and over SSH) and, as a
// best-effort fallback for terminals that ignore OSC 52 (notably macOS
// Terminal.app), also pipes the text to a native clipboard command when one is
// available on the local machine.
func (a *App) CopyToClipboard(text string) {
	encoded := base64.StdEncoding.EncodeToString([]byte(text))
	_, _ = io.WriteString(a.out, "\x1b]52;c;"+encoded+"\a")
	a.invalidateFront()
	nativeClipboardCopy(text)
}

// nativeClipboardCopy pipes text to the platform clipboard tool if present. It is
// best-effort: any error (tool missing, running over SSH) is silently ignored,
// because OSC 52 already covers those cases.
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
