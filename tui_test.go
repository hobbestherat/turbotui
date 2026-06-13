package tui

import (
	"bytes"
	"strings"
	"testing"
)

func TestApplyWritesOnlyDelta(t *testing.T) {
	var output bytes.Buffer
	app := NewWithSize(4, 2, &output)
	app.Clear(DefaultCell())
	app.WriteCell(1, 0, Cell{Ch: 'A', FG: ANSIColor(7), BG: ANSIColor(0)})

	if err := app.Apply(); err != nil {
		t.Fatalf("first apply failed: %v", err)
	}
	first := output.String()
	if !strings.Contains(first, "A") {
		t.Fatalf("expected first frame to contain changed cell content")
	}

	output.Reset()
	if err := app.Apply(); err != nil {
		t.Fatalf("second apply failed: %v", err)
	}
	if output.Len() != 0 {
		t.Fatalf("expected second apply to be empty, got %q", output.String())
	}
}

func TestSmartLineConnectors(t *testing.T) {
	app := NewWithSize(5, 5, &bytes.Buffer{})
	fg := ANSIColor(15)
	bg := ANSIColor(0)

	app.AddLinePiece(1, 2, Horizontal, LineSingle, fg, bg)
	app.AddLinePiece(2, 2, Horizontal, LineSingle, fg, bg)
	app.AddLinePiece(2, 1, Vertical, LineSingle, fg, bg)
	app.AddLinePiece(2, 2, Vertical, LineSingle, fg, bg)

	if got := app.back.get(2, 2).Ch; got != '┼' {
		t.Fatalf("expected single cross connector, got %q", got)
	}

	app = NewWithSize(5, 5, &bytes.Buffer{})
	app.AddLinePiece(1, 2, Horizontal, LineSingle, fg, bg)
	app.AddLinePiece(2, 2, Horizontal, LineSingle, fg, bg)
	app.AddLinePiece(2, 1, Vertical, LineDouble, fg, bg)
	app.AddLinePiece(2, 2, Vertical, LineDouble, fg, bg)

	if got := app.back.get(2, 2).Ch; got != '╫' {
		t.Fatalf("expected mixed cross connector, got %q", got)
	}
}

func TestWriteWrappedText(t *testing.T) {
	app := NewWithSize(20, 5, &bytes.Buffer{})
	style := Cell{FG: ANSIColor(15), BG: ANSIColor(0)}
	lines := app.WriteWrappedText(0, 0, 8, "one two three four", style)
	if lines < 2 {
		t.Fatalf("expected wrapping to span multiple lines")
	}
	line0 := string([]rune{
		app.back.get(0, 0).Ch, app.back.get(1, 0).Ch, app.back.get(2, 0).Ch,
		app.back.get(3, 0).Ch, app.back.get(4, 0).Ch, app.back.get(5, 0).Ch,
		app.back.get(6, 0).Ch,
	})
	if strings.TrimSpace(line0) != "one two" {
		t.Fatalf("unexpected wrapped first line: %q", line0)
	}
}

func TestDeltaAfterFullRedraw(t *testing.T) {
	var output bytes.Buffer
	app := NewWithSize(6, 3, &output)
	style := Cell{FG: ANSIColor(15), BG: ANSIColor(0)}

	app.Clear(DefaultCell())
	app.WriteString(1, 1, "abc", style)
	if err := app.Apply(); err != nil {
		t.Fatalf("first apply failed: %v", err)
	}
	firstLen := output.Len()
	if firstLen == 0 {
		t.Fatalf("expected output for first draw")
	}

	output.Reset()
	app.Clear(DefaultCell())
	app.WriteString(1, 1, "abc", style)
	if err := app.Apply(); err != nil {
		t.Fatalf("second apply failed: %v", err)
	}
	if output.Len() != 0 {
		t.Fatalf("expected no output for identical full redraw, got %q", output.String())
	}
}

func TestParseAltRuneSequence(t *testing.T) {
	event, consumed, ok := parseEscape([]byte{0x1b, 'f'})
	if !ok || consumed != 2 {
		t.Fatalf("expected parsed alt rune, ok=%v consumed=%d", ok, consumed)
	}
	typeEvent, cast := event.(TypeEvent)
	if !cast {
		t.Fatalf("expected type event")
	}
	if typeEvent.Key != KeyRune || typeEvent.Rune != 'f' || !typeEvent.Alt {
		t.Fatalf("unexpected alt type event: %#v", typeEvent)
	}
}

func TestParseSS3ArrowSequence(t *testing.T) {
	event, consumed, ok := parseEscape([]byte{0x1b, 'O', 'B'})
	if !ok || consumed != 3 {
		t.Fatalf("expected parsed ss3 arrow, ok=%v consumed=%d", ok, consumed)
	}
	typeEvent, cast := event.(TypeEvent)
	if !cast {
		t.Fatalf("expected type event")
	}
	if typeEvent.Key != KeyDown {
		t.Fatalf("expected KeyDown from SS3 B, got %#v", typeEvent)
	}
}

func TestParseCSIUShiftEnter(t *testing.T) {
	event := parseCSI("13;2", 'u')
	typeEvent, cast := event.(TypeEvent)
	if !cast {
		t.Fatalf("expected type event")
	}
	if typeEvent.Key != KeyEnter || !typeEvent.Shift {
		t.Fatalf("expected Shift+Enter, got %#v", typeEvent)
	}
}
