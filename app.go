package tui

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode/utf8"

	"golang.org/x/term"
)

type KeyCode uint16

const (
	KeyUnknown KeyCode = iota
	KeyRune
	KeyEnter
	KeyTab
	KeyBackspace
	KeyEscape
	KeyBackTab
	KeyUp
	KeyDown
	KeyLeft
	KeyRight
	KeyHome
	KeyEnd
	KeyPageUp
	KeyPageDown
	KeyInsert
	KeyDelete
	KeyF1
	KeyF2
	KeyF3
	KeyF4
	KeyF5
	KeyF6
	KeyF7
	KeyF8
	KeyF9
	KeyF10
	KeyF11
	KeyF12
)

type MouseButton uint8

const (
	MouseLeft MouseButton = iota
	MouseMiddle
	MouseRight
)

// TypeEvent is a decoded keyboard event. For a printable key Key is KeyRune and
// Rune holds the character; for named keys (arrows, Enter, ...) Key is the
// corresponding KeyCode and Rune is 0.
//
// Control chords arrive as KeyRune with Ctrl set and Rune holding the key's
// caret-notation character: Ctrl+A..Ctrl+Z decode to the lower-case letters
// 'a'..'z', and the non-letter control bytes decode to their punctuation rune —
// e.g. Ctrl+] is {Key: KeyRune, Rune: ']', Ctrl: true} and Ctrl+\ is '\\'.
// Because the letters are folded to lower case, compare case-insensitively (or
// match against the lower-case rune) when testing for a Ctrl+<letter> chord.
type TypeEvent struct {
	Key   KeyCode
	Rune  rune
	Ctrl  bool
	Alt   bool
	Shift bool
}

type ClickEvent struct {
	X      int
	Y      int
	Button MouseButton
	// Down is true for a button press and stays true for the motion ("drag")
	// reports the terminal emits while the button is held; it is false on release.
	// Existing drag-aware widgets rely on Down remaining set during a drag, so use
	// Drag to tell a fresh press apart from a continued drag.
	Down bool
	// Drag is true for a motion report received while a button is held (Down is
	// also true). Move is true for a motion report with no button held (Down is
	// false). Both require the terminal's motion-tracking mode (?1002h/?1003h).
	Drag bool
	Move bool
	// Keyboard modifiers active when the event was generated, when the terminal
	// reports them (SGR mouse encoding bits 4/8/16).
	Shift bool
	Alt   bool
	Ctrl  bool
}

// PasteEvent carries a block of text the terminal delivered via bracketed paste
// (mode ?2004). It is reported as one event so a multi-line paste does not look
// like a stream of individual keypresses (and cannot trigger Enter submits).
type PasteEvent struct {
	Text string
}

type ScrollEvent struct {
	X     int
	Y     int
	Delta int
}

type ResizeEvent struct {
	Width  int
	Height int
}

// App is the terminal application engine: a double-buffered cell grid plus an
// input-driven event loop. Its zero value is NOT usable (it has no output writer
// and an unbuffered/nil post queue) — construct one with New, NewWithIO or
// NewWithSize.
type App struct {
	in      *os.File
	out     io.Writer
	termOut *os.File

	// writeMu serializes raw writes to out so a CopyToClipboard call from a
	// background goroutine cannot interleave its OSC 52 bytes with an Apply
	// frame and corrupt the escape stream.
	writeMu sync.Mutex

	// clipboardBackend selects how CopyToClipboard delivers text. Its zero value
	// is ClipboardOSC52AndNative.
	clipboardBackend ClipboardBackend

	width  int
	height int

	front screen
	back  screen
	lines []lineCell

	clickHandlers  []func(ClickEvent)
	scrollHandlers []func(ScrollEvent)
	typeHandlers   []func(TypeEvent)
	pasteHandlers  []func(PasteEvent)
	resizeHandlers []func(ResizeEvent)

	restoreState *term.State
	parser       inputParser
	started      bool

	// Posted closures live in an unbounded, mutex-guarded mailbox rather than a
	// fixed channel: Post must never block the event-loop goroutine (a handler that
	// re-enters Post on a full buffer would otherwise deadlock — issue #20).
	// postNotify is a wake-up signal (it carries no data); the loop drains postQueue.
	postMu     sync.Mutex
	postQueue  []func()
	postNotify chan struct{}

	// dirty/redrawFn implement redraw coalescing (issue #17): mutations request a
	// redraw instead of flushing, and the loop redraws at most once after draining a
	// burst of posts. redrawFn is nil for bare App users, who flush via Apply.
	dirty    bool
	redrawFn func()

	// flushBuf is reused across Apply calls so a frame's escape stream is built with
	// no per-cell allocation (issue #15).
	flushBuf []byte

	// applyErr holds the terminal write error (if any) recorded by the most
	// recent Apply. onApplyError, when set, is invoked with that error. Run
	// treats a non-nil applyErr as fatal so the app exits cleanly instead of
	// continuing to render against a dead output (e.g. a closed pipe).
	applyErr     error
	onApplyError func(error)

	// Hardware cursor state. cursorVisible/X/Y is the desired state; the front*
	// fields track what was last emitted so Apply only writes cursor escapes on
	// change.
	cursorVisible      bool
	cursorX            int
	cursorY            int
	frontCursorVisible bool
	frontCursorX       int
	frontCursorY       int
	// forceCursor makes the next Apply re-emit the desired cursor state regardless
	// of the front* record (set by invalidateFront). A single frontCursorVisible
	// sentinel cannot force the hidden branch — false reads as both "unknown" and
	// "already hidden" — so this flag drives the re-issue and is cleared after the
	// emit.
	forceCursor bool
}

// SetCursor positions the real terminal cursor and makes it visible. Widgets use
// it so the blinking hardware cursor marks the text insertion point instead of a
// painted block that would hide the character underneath.
func (a *App) SetCursor(x int, y int) {
	a.cursorVisible = true
	a.cursorX = x
	a.cursorY = y
}

// HideCursor hides the hardware cursor (the default while no input is focused).
func (a *App) HideCursor() {
	a.cursorVisible = false
}

// New creates an App sized to the current terminal. A failure to read the
// terminal size (e.g. when stdout is not a tty) is not an error: it falls back
// to an 80×25 size, mirroring NewWithSize. The failure that actually matters —
// stdin/stdout not being a real terminal — surfaces later from Run. Use
// Validate to detect that up front.
func New() *App {
	width, height, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || width < 1 || height < 1 {
		width = 80
		height = 25
	}
	return NewWithIO(os.Stdin, os.Stdout, width, height)
}

// Validate reports whether the App is attached to a real terminal — the check
// that New (intentionally) does not perform, since size detection has a sane
// fallback. It returns nil for buffer-backed apps built with NewWithSize (which
// have no terminal by design). Call it right after New when you want to fail
// fast with a clear message instead of waiting for Run.
func (a *App) Validate() error {
	if a.termOut == nil {
		return nil
	}
	if a.in == nil || !term.IsTerminal(int(a.in.Fd())) {
		return errors.New("turbotui: stdin is not a terminal")
	}
	if !term.IsTerminal(int(a.termOut.Fd())) {
		return errors.New("turbotui: stdout is not a terminal")
	}
	return nil
}

func NewWithIO(in *os.File, out *os.File, width int, height int) *App {
	app := NewWithSize(width, height, out)
	app.in = in
	app.termOut = out
	return app
}

func NewWithSize(width int, height int, out io.Writer) *App {
	if width < 1 {
		width = 1
	}
	if height < 1 {
		height = 1
	}
	app := &App{
		out:        out,
		width:      width,
		height:     height,
		front:      newScreen(width, height, Cell{}),
		back:       newScreen(width, height, DefaultCell()),
		lines:      make([]lineCell, width*height),
		postNotify: make(chan struct{}, 1),
	}
	app.invalidateFront()
	return app
}

// Width returns the current grid width. Like Height and the handler-registration
// methods (OnClick, OnType, …), it reads loop-goroutine state without
// synchronization: call it before Run, or from inside a handler / a Post callback
// (which run on the loop goroutine). Reading it directly from a background
// goroutine while Run is active is a data race — route through Post (issue #21).
func (a *App) Width() int {
	return a.width
}

// Height returns the current grid height. See Width for the threading contract.
func (a *App) Height() int {
	return a.height
}

func (a *App) ReadCell(x int, y int) Cell {
	return a.back.get(x, y)
}

// OnClick registers a click handler. Handlers are dispatched on the event-loop
// goroutine and the handler slices are not synchronized, so register handlers
// before Run or from inside another handler / a Post callback — never from a
// background goroutine while Run is active (issue #21). The same applies to
// OnScroll, OnType, OnPaste and OnResize.
func (a *App) OnClick(handler func(ClickEvent)) {
	a.clickHandlers = append(a.clickHandlers, handler)
}

func (a *App) OnScroll(handler func(ScrollEvent)) {
	a.scrollHandlers = append(a.scrollHandlers, handler)
}

func (a *App) OnType(handler func(TypeEvent)) {
	a.typeHandlers = append(a.typeHandlers, handler)
}

// OnPaste registers a handler for bracketed-paste blocks. Apps should treat the
// text as literal input (insert it at the cursor) rather than re-interpreting it.
func (a *App) OnPaste(handler func(PasteEvent)) {
	a.pasteHandlers = append(a.pasteHandlers, handler)
}

// Post schedules fn to run on the event-loop goroutine. It is safe to call from
// any goroutine, which makes it the way to mutate UI state from a background
// task (timers, network calls, …) and then trigger a redraw.
//
// Post never blocks and never drops an update: closures are appended to an
// unbounded mailbox. In particular a handler running on the loop goroutine may
// re-enter Post safely — the loop keeps draining the mailbox, so there is no
// self-deadlock even under a burst of updates (issue #20).
func (a *App) Post(fn func()) {
	if fn == nil {
		return
	}
	a.postMu.Lock()
	a.postQueue = append(a.postQueue, fn)
	a.postMu.Unlock()
	// Wake the loop. The buffer-of-1 notify channel coalesces multiple posts into a
	// single wake-up; the loop drains everything once woken.
	select {
	case a.postNotify <- struct{}{}:
	default:
	}
}

// TryPost enqueues fn and reports whether it was scheduled. With the unbounded
// mailbox Post never blocks, so TryPost always succeeds for a non-nil fn; it is
// retained for API compatibility with callers that branched on its result.
func (a *App) TryPost(fn func()) bool {
	if fn == nil {
		return true
	}
	a.Post(fn)
	return true
}

// drainPosts runs every queued closure, including ones enqueued by those closures
// (re-entrant Post), until the mailbox is empty. It runs on the loop goroutine.
func (a *App) drainPosts() {
	for {
		a.postMu.Lock()
		if len(a.postQueue) == 0 {
			a.postQueue = a.postQueue[:0]
			a.postMu.Unlock()
			return
		}
		fn := a.postQueue[0]
		a.postQueue = a.postQueue[1:]
		a.postMu.Unlock()
		fn()
	}
}

// SetRedrawFn registers the function used to repaint the screen when a coalesced
// redraw is due (see RequestRedraw). The tv.Desktop sets this to its compose+Apply
// path; bare App users typically leave it unset and call Apply directly.
func (a *App) SetRedrawFn(fn func()) {
	a.redrawFn = fn
}

// RequestRedraw marks the screen dirty without painting. The event loop coalesces
// all requests accumulated while draining a burst of posts/events into a single
// redrawFn call per iteration, so streaming N updates costs one repaint and one
// write instead of N (issue #17). It is a no-op when no redrawFn is registered.
func (a *App) RequestRedraw() {
	a.dirty = true
}

// flushDirty performs at most one coalesced redraw. Called once per event-loop
// iteration after all immediately-available work has been drained.
func (a *App) flushDirty() {
	if a.dirty && a.redrawFn != nil {
		a.dirty = false
		a.redrawFn()
	}
}

// OnApplyError registers a callback invoked when Apply fails to write to the
// terminal (e.g. stdout was closed or redirected onto a broken pipe). It fires
// before Run treats the error as fatal, giving the app a chance to log or
// customise. Leaving it unset is fine: Run still returns the error and exits.
func (a *App) OnApplyError(fn func(error)) {
	a.onApplyError = fn
}

// LastApplyError returns the terminal write error recorded by the most recent
// Apply, or nil. Run returns this same error and exits the loop when it is
// non-nil, so callers can inspect it after Run returns.
func (a *App) LastApplyError() error {
	return a.applyErr
}

func (a *App) OnResize(handler func(ResizeEvent)) {
	a.resizeHandlers = append(a.resizeHandlers, handler)
}

func (a *App) Clear(cell Cell) {
	if cell.Ch == 0 {
		cell.Ch = ' '
	}
	a.back.fill(cell)
	for index := range a.lines {
		a.lines[index] = lineCell{}
	}
}

// WriteCell writes a single cell, accounting for the glyph's display width: a
// double-width rune also lays down a continuation cell to its right, and writing
// over either half of an existing wide glyph blanks its orphaned other half.
func (a *App) WriteCell(x int, y int, cell Cell) {
	cell.cont = false
	width := RuneWidth(cell.Ch)
	if width < 1 {
		// A bare combining mark has nothing to attach to here; render it as a
		// single placeholder column rather than collapsing the cell.
		width = 1
	}
	a.place(x, y, cell, width)
}

// place writes cell at (x,y) as a glyph that occupies width columns (1 or 2),
// keeping the cell model consistent with what the terminal will render.
func (a *App) place(x int, y int, cell Cell, width int) {
	a.clearWideAt(x, y)
	if width >= 2 {
		// A wide glyph that would straddle the right edge cannot be drawn without
		// the terminal wrapping; substitute a blank so the row stays aligned.
		if !a.back.inBounds(x+1, y) {
			cell.Ch = ' '
			cell.Combining = ""
			a.setCellTracked(x, y, cell)
			return
		}
		a.clearWideAt(x+1, y)
		a.setCellTracked(x, y, cell)
		a.setCellTracked(x+1, y, Cell{
			Ch:        ' ',
			FG:        cell.FG,
			BG:        cell.BG,
			Bold:      cell.Bold,
			Underline: cell.Underline,
			Italic:    cell.Italic,
			cont:      true,
		})
		return
	}
	a.setCellTracked(x, y, cell)
}

// clearWideAt blanks the dangling half of a wide glyph that an upcoming write to
// (x,y) would partially overwrite, so a leftover continuation column or orphaned
// left half never lingers on screen.
func (a *App) clearWideAt(x int, y int) {
	if !a.back.inBounds(x, y) {
		return
	}
	current := a.back.get(x, y)
	if current.cont {
		if a.back.inBounds(x-1, y) {
			base := a.back.get(x-1, y)
			base.Ch = ' '
			base.Combining = ""
			a.setCellTracked(x-1, y, base)
		}
		return
	}
	if RuneWidth(current.Ch) >= 2 && a.back.inBounds(x+1, y) {
		right := a.back.get(x+1, y)
		if right.cont {
			a.setCellTracked(x+1, y, Cell{Ch: ' ', FG: right.FG, BG: right.BG})
		}
	}
}

// setCellTracked writes a cell verbatim and invalidates the line-drawing cache
// for that position. It performs no width handling and is the low-level write
// shared by place and clearWideAt.
func (a *App) setCellTracked(x int, y int, cell Cell) {
	if cell.Ch == 0 {
		cell.Ch = ' '
	}
	a.back.set(x, y, cell)
	idx := a.lineIndex(x, y)
	if idx >= 0 {
		a.lines[idx] = lineCell{}
	}
}

func (a *App) WriteString(x int, y int, text string, style Cell) {
	column := x
	lastBase := -1
	for _, ch := range text {
		width := RuneWidth(ch)
		if width == 0 {
			a.attachCombining(lastBase, y, ch)
			continue
		}
		cell := style
		cell.Ch = ch
		cell.Combining = ""
		a.place(column, y, cell, width)
		lastBase = column
		column += width
	}
}

// attachCombining folds a zero-width mark into the base cell previously written
// at (x,y) so the grapheme renders together. Marks with no base (e.g. a string
// that opens with a combining char) are dropped.
func (a *App) attachCombining(x int, y int, mark rune) {
	if x < 0 || !a.back.inBounds(x, y) {
		return
	}
	base := a.back.get(x, y)
	if base.cont {
		return
	}
	base.Combining += string(mark)
	a.setCellTracked(x, y, base)
}

func (a *App) WriteWrappedText(x int, y int, width int, text string, style Cell) int {
	if width < 1 {
		return 0
	}
	line := 0
	col := 0
	runes := []rune(text)
	word := make([]rune, 0, 16)
	flushWord := func(forceSpace bool) {
		if len(word) == 0 {
			if forceSpace && col > 0 {
				col++
			}
			return
		}
		wordWidth := 0
		for _, r := range word {
			wordWidth += RuneWidth(r)
		}
		needed := wordWidth
		if col > 0 {
			needed++
		}
		if col+needed > width {
			line++
			col = 0
		}
		if col > 0 {
			style.Ch = ' '
			style.Combining = ""
			a.place(x+col, y+line, style, 1)
			col++
		}
		lastBase := -1
		for _, r := range word {
			runeWidth := RuneWidth(r)
			if runeWidth == 0 {
				if lastBase >= 0 {
					a.attachCombining(x+lastBase, y+line, r)
				}
				continue
			}
			if col+runeWidth > width {
				line++
				col = 0
				lastBase = -1
			}
			style.Ch = r
			style.Combining = ""
			a.place(x+col, y+line, style, runeWidth)
			lastBase = col
			col += runeWidth
		}
		word = word[:0]
	}
	for _, ch := range runes {
		if ch == '\n' {
			flushWord(false)
			line++
			col = 0
			continue
		}
		if ch == ' ' || ch == '\t' {
			flushWord(true)
			continue
		}
		word = append(word, ch)
	}
	flushWord(false)
	return line + 1
}

func (a *App) lineIndex(x int, y int) int {
	if !a.back.inBounds(x, y) {
		return -1
	}
	return y*a.width + x
}

func (a *App) invalidateFront() {
	for index := range a.front.cells {
		a.front.cells[index] = Cell{}
	}
	a.frontCursorVisible = false
	a.forceCursor = true
}

// Invalidate forces the next Apply to repaint every cell and re-issue the cursor
// state, by discarding the App's record of what is currently on the terminal.
//
// Apply normally writes only cells whose logical content changed since the last
// frame. If the real terminal has drifted out of sync with that record — e.g. raw
// escape bytes reached the terminal out of band and scrambled it — the diff keeps
// skipping the genuinely-wrong cells and the artifact survives ordinary repaints
// until that exact cell happens to change. Invalidate breaks that stall: the
// following Apply redraws the whole screen, healing any such drift.
//
// Call it on the event-loop goroutine (e.g. from a Post callback), like the other
// mutating methods. Prefer eliminating out-of-band writes (see WriteControl,
// CopyToClipboard) over papering over them with a full repaint.
func (a *App) Invalidate() {
	a.invalidateFront()
}

// WriteControl writes a self-contained terminal control/escape sequence to the
// output, serialised against frame flushes through the same lock Apply uses. It is
// the notification counterpart of CopyToClipboard: it lets a caller emit a
// sequence such as a BEL or an OSC notification (OSC 9 / OSC 777) without its bytes
// splicing into an in-flight Apply frame and corrupting the escape stream.
//
// seq MUST be self-contained — it must not move the cursor, change SGR state, or
// otherwise touch the cell grid — because Apply's front-buffer diff has no record
// of it. (BEL, OSC 9, OSC 777 and OSC 52 all satisfy this.) For a sequence that
// does disturb rendering, call Invalidate afterwards to force a full repaint.
//
// Unlike Apply, WriteControl is NOT gated on the alternate-screen switch: it emits
// even before Run has started. That is intentional — a bell or notification is not
// screen content (it is not torn down with the alt screen), so it should fire
// whenever it is requested. Do not route grid output through it expecting the
// alt-screen suppression Apply provides.
//
// The write is best-effort: a failed write (closed/broken output) is discarded
// rather than recorded, matching CopyToClipboard. A genuinely dead output is still
// surfaced by the next Apply via LastApplyError / OnApplyError, which is what drives
// Run's clean exit.
//
// It is safe to call from any goroutine; the write is mutex-guarded. As with
// CopyToClipboard, calling it on the event-loop goroutine (or via Post) is the
// supported contract.
func (a *App) WriteControl(seq string) {
	if seq == "" {
		return
	}
	_ = a.writeOut(seq)
}

// writeOut writes s to the output holding writeMu, so it can never interleave with
// an Apply frame, a WriteControl / CopyToClipboard sequence, or another control
// write racing in from a background goroutine. Every raw write to a.out outside
// Apply's frame flush goes through it — including the setup and teardown sequences,
// which a background WriteControl could otherwise splice into and corrupt (the
// terminal restore in particular). It returns the write error for callers that
// surface it (setupTerminal); teardown paths ignore it.
func (a *App) writeOut(s string) error {
	a.writeMu.Lock()
	_, err := io.WriteString(a.out, s)
	a.writeMu.Unlock()
	return err
}

func (a *App) Apply() error {
	// On a real terminal, suppress flushes until Run has switched to the
	// alternate screen; otherwise frames composed during setup would leak onto
	// the normal buffer and remain visible after the alt screen is torn down.
	// Buffer-backed apps (termOut == nil, used in tests) always flush.
	if a.termOut != nil && !a.started {
		return nil
	}
	// Wrap the frame in a DEC 2026 synchronized update so the terminal swaps it
	// atomically instead of rendering partial bytes as they stream in; terminals
	// that don't understand ?2026 ignore it, so it is safe unconditionally (#16).
	buf := append(a.flushBuf[:0], syncBegin...)
	bodyStart := len(buf)
	style := styleState{}
	// Track where the terminal cursor sits after the previous write so a run of
	// adjacent changed cells emits a single CUP instead of one per cell (#14).
	cursorX, cursorY := -1, -1
	for y := 0; y < a.height; y++ {
		rowOffset := y * a.width
		for x := 0; x < a.width; x++ {
			index := rowOffset + x
			next := a.back.cells[index]
			// Coerce a zero rune to a space BEFORE the diff so the value compared and
			// the value stored into front match; otherwise front(' ') != back(0)
			// would repaint this cell every frame forever (issue #13).
			if next.Ch == 0 {
				next.Ch = ' '
			}
			if a.front.cells[index] == next {
				continue
			}
			// The continuation half of a wide glyph carries no output of its own;
			// the wide glyph to its left already advanced the terminal over this
			// column. Sync the front buffer and move on.
			if next.cont {
				a.front.cells[index] = next
				continue
			}
			if cursorY != y || cursorX != x {
				buf = appendCursorMove(buf, x, y)
			}
			buf = appendStyle(buf, style, next)
			buf = appendRune(buf, next.Ch)
			if next.Combining != "" {
				buf = append(buf, next.Combining...)
			}
			width := RuneWidth(next.Ch)
			if width < 1 {
				width = 1
			}
			cursorX = x + width
			cursorY = y
			style = styleState{
				valid:     true,
				fg:        next.FG,
				bg:        next.BG,
				bold:      next.Bold,
				underline: next.Underline,
				italic:    next.Italic,
			}
			a.front.cells[index] = next
		}
	}
	buf = a.appendCursorEscapes(buf)
	if len(buf) == bodyStart {
		a.flushBuf = buf[:0] // nothing changed; keep the capacity, write nothing
		return nil
	}
	buf = append(buf, syncEnd...)
	a.flushBuf = buf
	a.writeMu.Lock()
	_, err := a.out.Write(buf)
	a.writeMu.Unlock()
	if err != nil {
		// The terminal went away (broken pipe, redirected/closed stdout, …).
		// Record it so Run can exit cleanly and surface it to any callback
		// rather than silently rendering into a dead output.
		a.applyErr = err
		if a.onApplyError != nil {
			a.onApplyError(err)
		}
	}
	return err
}

// syncBegin/syncEnd wrap a flush in a DEC 2026 synchronized update (issue #16).
const (
	syncBegin = "\x1b[?2026h"
	syncEnd   = "\x1b[?2026l"
)

// appendCursorEscapes appends the control sequence needed to bring the real
// terminal cursor in line with the desired state to buf (nothing when unchanged).
func (a *App) appendCursorEscapes(buf []byte) []byte {
	// Consume the force-redraw request (set by invalidateFront): it makes both the
	// show and hide branches below re-emit even when the front record already
	// matches. This runs only after Apply's pre-Run suppression check, so a
	// suppressed frame leaves forceCursor set for the first real flush.
	force := a.forceCursor
	a.forceCursor = false
	if a.cursorVisible {
		if !force && a.frontCursorVisible && a.frontCursorX == a.cursorX && a.frontCursorY == a.cursorY {
			return buf
		}
		a.frontCursorVisible = true
		a.frontCursorX = a.cursorX
		a.frontCursorY = a.cursorY
		buf = appendCursorMove(buf, a.cursorX, a.cursorY)
		return append(buf, "\x1b[?25h"...)
	}
	if !force && !a.frontCursorVisible {
		return buf
	}
	a.frontCursorVisible = false
	return append(buf, "\x1b[?25l"...)
}

func (a *App) Run(ctx context.Context) error {
	if a.in == nil || a.termOut == nil {
		return errors.New("Run requires terminal stdin/stdout")
	}
	if err := a.setupTerminal(); err != nil {
		return err
	}
	defer a.Close()

	a.invalidateFront()
	if err := a.Apply(); err != nil {
		return err
	}

	readChannel := make(chan []byte, 4)
	errorChannel := make(chan error, 1)
	go a.readInput(readChannel, errorChannel)
	// Unblock the reader on every exit path: a.in.Read blocks on the shared stdin,
	// so without a deadline the goroutine would leak and steal the next keystroke
	// meant for the parent process or the next TUI (issue #9).
	defer func() { _ = a.in.SetReadDeadline(time.Now()) }()

	resizeChannel := make(chan os.Signal, 1)
	stopResize := a.notifyResize(ctx, resizeChannel)
	defer stopResize()

	// Best-effort terminal restoration on fatal signals: a plain defer does not run
	// when the process is killed by an unhandled signal, which would leave the
	// terminal in raw mode on the alt screen (issue #22). Ctrl+C is delivered as a
	// byte in raw mode, so trapping SIGINT only catches an external `kill -INT`.
	fatalChannel := make(chan os.Signal, 1)
	signal.Notify(fatalChannel, fatalSignals()...)
	defer signal.Stop(fatalChannel)

	// A lone ESC byte can't be parsed immediately: it might begin an escape
	// sequence. We hold it briefly and, if nothing follows, flush it as KeyEscape.
	var escTimer <-chan time.Time
	for {
		select {
		case <-ctx.Done():
			return nil
		case sig := <-fatalChannel:
			// Restore the terminal, then re-raise with the default disposition so the
			// process dies as the signal intended.
			a.Close()
			if s, ok := sig.(syscall.Signal); ok {
				signal.Reset(s)
				reraiseSignal(s)
			}
			return nil
		case err := <-errorChannel:
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		case data := <-readChannel:
			for _, event := range a.parser.Feed(data) {
				a.dispatchEvent(event)
			}
			if a.parser.pendingLoneEscape() {
				escTimer = time.After(40 * time.Millisecond)
			} else {
				escTimer = nil
			}
		case <-escTimer:
			escTimer = nil
			for _, event := range a.parser.flushLoneEscape() {
				a.dispatchEvent(event)
			}
		case <-a.postNotify:
			// Drain the whole mailbox (including posts enqueued by the posts we run)
			// before redrawing, so a burst of background updates coalesces into a
			// single redraw/flush (issues #17, #20).
			a.drainPosts()
		case <-resizeChannel:
			a.handleTerminalResize()
		}
		// Apply any redraw requested by the work above exactly once.
		a.flushDirty()
		// A handler above (e.g. a redraw triggered by an event or Post) may have
		// failed to flush to the terminal. Treat that as fatal so the app exits
		// cleanly instead of spinning against a dead output.
		if a.applyErr != nil {
			return a.applyErr
		}
	}
}

// teardownSequence resets SGR and disables the modes setupTerminal enabled
// (bracketed paste, mouse tracking, hidden cursor, alt screen), returning the
// normal screen. It is written before term.Restore on every teardown path.
const teardownSequence = "\x1b[0m\x1b[?2004l\x1b[?1002l\x1b[?1006l\x1b[?25h\x1b[?1049l"

// restoreTerminal writes the teardown escape sequence and restores cooked mode.
// It is idempotent: the second call is a no-op once restoreState is cleared.
func (a *App) restoreTerminal() {
	if a.restoreState != nil {
		_ = a.writeOut(teardownSequence)
		_ = term.Restore(int(a.in.Fd()), a.restoreState)
		a.restoreState = nil
	}
	a.started = false
}

func (a *App) Close() {
	a.restoreTerminal()
}

// CloseWithMessage restores the terminal, tears down the alternate screen (so the
// TUI disappears and the prompt returns), then prints message in the normal
// buffer right after the command that launched the app. The message may span
// multiple lines and contain ANSI styling (see Styled).
func (a *App) CloseWithMessage(message string) {
	if a.restoreState != nil {
		a.restoreTerminal()
	} else {
		_ = a.writeOut(teardownSequence)
		a.started = false
	}
	message = strings.Trim(message, "\n")
	if strings.TrimSpace(message) == "" {
		return
	}
	_ = a.writeOut(message + "\n")
}

func (a *App) setupTerminal() error {
	state, err := term.MakeRaw(int(a.in.Fd()))
	if err != nil {
		return err
	}
	a.restoreState = state
	a.started = true
	// Clear any read deadline a previous Run left behind when it unblocked the
	// reader on exit (issue #9); otherwise this Run's reads on a shared stdin would
	// fail immediately against a deadline already in the past.
	_ = a.in.SetReadDeadline(time.Time{})
	return a.writeOut("\x1b[?1049h\x1b[?25l\x1b[?1002h\x1b[?1006h\x1b[?2004h")
}

func (a *App) readInput(target chan<- []byte, errorsOut chan<- error) {
	buffer := make([]byte, 256)
	for {
		count, err := a.in.Read(buffer)
		if count > 0 {
			chunk := make([]byte, count)
			copy(chunk, buffer[:count])
			target <- chunk
		}
		if err != nil {
			errorsOut <- err
			return
		}
	}
}

func (a *App) dispatchEvent(event any) {
	switch e := event.(type) {
	case TypeEvent:
		for _, handler := range a.typeHandlers {
			handler(e)
		}
	case PasteEvent:
		for _, handler := range a.pasteHandlers {
			handler(e)
		}
	case ClickEvent:
		for _, handler := range a.clickHandlers {
			handler(e)
		}
	case ScrollEvent:
		for _, handler := range a.scrollHandlers {
			handler(e)
		}
	case ResizeEvent:
		for _, handler := range a.resizeHandlers {
			handler(e)
		}
	}
}

// Resize changes the logical terminal size and runs the registered resize
// handlers, exactly as a real terminal SIGWINCH would. It lets embedded/headless
// callers (and tests) drive reflow without a tty.
func (a *App) Resize(width int, height int) {
	a.resize(width, height)
}

func (a *App) handleTerminalResize() {
	if a.termOut == nil {
		return
	}
	width, height, err := term.GetSize(int(a.termOut.Fd()))
	if err != nil {
		return
	}
	a.resize(width, height)
}

func (a *App) resize(width int, height int) {
	if width < 1 || height < 1 {
		return
	}
	oldWidth := a.width
	oldHeight := a.height
	oldBack := a.back
	oldLines := a.lines

	a.width = width
	a.height = height
	a.front = newScreen(width, height, Cell{})
	a.back = newScreen(width, height, DefaultCell())
	a.lines = make([]lineCell, width*height)

	// An immediate-mode toolkit (tv.Desktop) recomposes the whole tree from its
	// resize handler, discarding any preserved content — so copying the old buffer
	// is dead work and the trailing Apply would be a redundant second flush. Only
	// preserve content (and self-flush) for bare App users with no resize handler,
	// who draw once and rely on resize keeping what is on screen (issue #19).
	hasHandlers := len(a.resizeHandlers) > 0
	if !hasHandlers {
		copyWidth := oldWidth
		if width < copyWidth {
			copyWidth = width
		}
		copyHeight := oldHeight
		if height < copyHeight {
			copyHeight = height
		}
		for y := 0; y < copyHeight; y++ {
			for x := 0; x < copyWidth; x++ {
				sourceIndex := y*oldWidth + x
				targetIndex := y*width + x
				a.back.cells[targetIndex] = oldBack.cells[sourceIndex]
				if sourceIndex < len(oldLines) {
					a.lines[targetIndex] = oldLines[sourceIndex]
				}
			}
		}
	}
	a.invalidateFront()
	_ = a.writeOut("\x1b[2J\x1b[H")

	event := ResizeEvent{Width: width, Height: height}
	for _, handler := range a.resizeHandlers {
		handler(event)
	}
	if !hasHandlers {
		_ = a.Apply()
	}
}

type inputParser struct {
	pending []byte
}

func (p *inputParser) Feed(chunk []byte) []any {
	p.pending = append(p.pending, chunk...)
	events := make([]any, 0, 8)
	for len(p.pending) > 0 {
		event, consumed, ok := parseOneInput(p.pending)
		if !ok {
			break
		}
		if consumed <= 0 {
			break
		}
		p.pending = p.pending[consumed:]
		if event != nil {
			events = append(events, event)
		}
	}
	if len(p.pending) > maxPendingBytes {
		// Keep buffering an in-progress paste until its terminator arrives, but
		// give up on a runaway (malformed) paste so memory stays bounded.
		if !bytes.HasPrefix(p.pending, pasteStartSeq) || len(p.pending) > maxPasteBytes {
			p.pending = p.pending[:0]
		}
	}
	return events
}

// pendingLoneEscape reports that the only buffered byte is ESC, which the parser
// is holding back in case it begins an escape sequence.
func (p *inputParser) pendingLoneEscape() bool {
	return len(p.pending) == 1 && p.pending[0] == 0x1b
}

// flushLoneEscape resolves a held-back lone ESC as a KeyEscape event when no
// follow-up bytes arrived in time.
func (p *inputParser) flushLoneEscape() []any {
	if !p.pendingLoneEscape() {
		return nil
	}
	p.pending = p.pending[:0]
	return []any{TypeEvent{Key: KeyEscape}}
}

func parseOneInput(data []byte) (any, int, bool) {
	if len(data) == 0 {
		return nil, 0, false
	}
	head := data[0]
	if head == 0x1b {
		return parseEscape(data)
	}
	if head == 0x7f {
		return TypeEvent{Key: KeyBackspace}, 1, true
	}
	if head == '\r' {
		return TypeEvent{Key: KeyEnter}, 1, true
	}
	// LF (^J) is what most terminals send for Ctrl+Enter; surface it as a
	// Ctrl-modified Enter so apps can use it as a "submit" key.
	if head == '\n' {
		return TypeEvent{Key: KeyEnter, Ctrl: true}, 1, true
	}
	if head == '\t' {
		return TypeEvent{Key: KeyTab}, 1, true
	}
	if head < 0x20 {
		// Decode C0 control bytes to their printable form via the standard
		// caret-notation mapping (XOR 0x40), which is correct across the whole
		// range: 0x01->'A' .. 0x1A->'Z', 0x1C->'\', 0x1D->']', 0x1E->'^',
		// 0x1F->'_', 0x00->'@'. The earlier letter-only offset (head + 'a' - 1)
		// was valid only for 0x01..0x1A and mis-mapped 0x1C..0x1F (e.g.
		// Ctrl+] -> '}') and 0x00 -> '`'. Letters (0x01..0x1A) are folded to
		// lower case to keep the historical rune output for the already-working
		// Ctrl+<letter> shortcuts; the non-letter bytes (0x00, 0x1C..0x1F) now
		// follow caret notation, so Rune is upper/punctuation, not lower case.
		// (ESC 0x1B is handled earlier by parseEscape and never reaches here.)
		r := rune(head ^ 0x40)
		if r >= 'A' && r <= 'Z' {
			r += 'a' - 'A'
		}
		return TypeEvent{
			Key:  KeyRune,
			Rune: r,
			Ctrl: true,
		}, 1, true
	}
	if !utf8.FullRune(data) {
		return nil, 0, false
	}
	r, size := utf8.DecodeRune(data)
	if r == utf8.RuneError && size == 1 {
		return nil, 1, true
	}
	return TypeEvent{Key: KeyRune, Rune: r}, size, true
}

func parseEscape(data []byte) (any, int, bool) {
	if len(data) == 1 {
		return nil, 0, false
	}
	if data[1] == 'O' {
		// Distinguish "incomplete" from "unrecognized": only ask for more bytes when
		// fewer than 3 are present. Once 3 are buffered we always consume them, even
		// if parseSS3 doesn't recognize the final byte (it returns a nil event).
		// Otherwise a stray SS3 such as ESC O P (F1) would never be consumed and
		// would wedge the parser behind it (issue #8).
		if len(data) < 3 {
			return nil, 0, false
		}
		return parseSS3(data), 3, true
	}
	if data[1] != '[' {
		if utf8.FullRune(data[1:]) {
			r, size := utf8.DecodeRune(data[1:])
			if r != utf8.RuneError || size > 1 {
				return TypeEvent{Key: KeyRune, Rune: r, Alt: true}, 1 + size, true
			}
		}
		return TypeEvent{Key: KeyEscape}, 1, true
	}
	finalIndex := -1
	for index := 2; index < len(data); index++ {
		ch := data[index]
		if ch >= 0x40 && ch <= 0x7e {
			finalIndex = index
			break
		}
	}
	if finalIndex < 0 {
		return nil, 0, false
	}
	params := string(data[2:finalIndex])
	final := data[finalIndex]
	// Bracketed paste: ESC[200~ <text> ESC[201~ is delivered as one literal block.
	if final == '~' && params == "200" {
		return parseBracketedPaste(data, finalIndex+1)
	}
	event := parseCSI(params, final)
	return event, finalIndex + 1, true
}

const (
	maxPendingBytes = 4096
	maxPasteBytes   = 1 << 20
)

var (
	pasteStartSeq = []byte("\x1b[200~")
	pasteEndSeq   = []byte("\x1b[201~")
)

// parseBracketedPaste captures everything between the paste-start sequence and
// the next ESC[201~ terminator. It reports need-more-bytes until the terminator
// has been buffered so a paste split across reads is reassembled intact.
func parseBracketedPaste(data []byte, contentStart int) (any, int, bool) {
	index := bytes.Index(data[contentStart:], pasteEndSeq)
	if index < 0 {
		return nil, 0, false
	}
	text := string(data[contentStart : contentStart+index])
	consumed := contentStart + index + len(pasteEndSeq)
	return PasteEvent{Text: text}, consumed, true
}

func parseCSI(params string, final byte) any {
	shift, alt, ctrl := parseCSIModifiers(params)
	switch final {
	case 'A':
		return TypeEvent{Key: KeyUp, Shift: shift, Alt: alt, Ctrl: ctrl}
	case 'B':
		return TypeEvent{Key: KeyDown, Shift: shift, Alt: alt, Ctrl: ctrl}
	case 'C':
		return TypeEvent{Key: KeyRight, Shift: shift, Alt: alt, Ctrl: ctrl}
	case 'D':
		return TypeEvent{Key: KeyLeft, Shift: shift, Alt: alt, Ctrl: ctrl}
	case 'Z':
		return TypeEvent{Key: KeyBackTab}
	case 'u':
		keyCode, mod := parseCSIParams(params)
		shift, alt, ctrl := decodeCSIModifier(mod)
		// Terminals in Kitty / modifyOtherKeys mode encode many keys as CSI-u. Map
		// the standard special codepoints (and Kitty's functional-key range) before
		// falling back to a literal rune; otherwise Tab/Backspace/Esc and the
		// function keys silently vanish or arrive as wrong runes (issue #12).
		if key, ok := csiUSpecialKey(keyCode); ok {
			return TypeEvent{Key: key, Shift: shift, Alt: alt, Ctrl: ctrl}
		}
		// Modified printable keys (e.g. Ctrl+Shift+C) arrive as CSI-u when the
		// terminal reports them; surface them as rune events.
		if keyCode > 0x20 && keyCode != 0x7f {
			return TypeEvent{Key: KeyRune, Rune: rune(keyCode), Shift: shift, Alt: alt, Ctrl: ctrl}
		}
	case '~':
		value, mod := parseCSIParams(params)
		shift, alt, ctrl := decodeCSIModifier(mod)
		switch value {
		case 1, 7:
			return TypeEvent{Key: KeyHome, Shift: shift, Alt: alt, Ctrl: ctrl}
		case 2:
			return TypeEvent{Key: KeyInsert, Shift: shift, Alt: alt, Ctrl: ctrl}
		case 3:
			return TypeEvent{Key: KeyDelete, Shift: shift, Alt: alt, Ctrl: ctrl}
		case 4, 8:
			return TypeEvent{Key: KeyEnd, Shift: shift, Alt: alt, Ctrl: ctrl}
		case 5:
			return TypeEvent{Key: KeyPageUp, Shift: shift, Alt: alt, Ctrl: ctrl}
		case 6:
			return TypeEvent{Key: KeyPageDown, Shift: shift, Alt: alt, Ctrl: ctrl}
		// Function keys in the CSI-tilde encoding: F1–F4 as 11–14 (older terminals)
		// and F5–F12 as 15/17–21/23/24.
		case 11:
			return TypeEvent{Key: KeyF1, Shift: shift, Alt: alt, Ctrl: ctrl}
		case 12:
			return TypeEvent{Key: KeyF2, Shift: shift, Alt: alt, Ctrl: ctrl}
		case 13:
			return TypeEvent{Key: KeyF3, Shift: shift, Alt: alt, Ctrl: ctrl}
		case 14:
			return TypeEvent{Key: KeyF4, Shift: shift, Alt: alt, Ctrl: ctrl}
		case 15:
			return TypeEvent{Key: KeyF5, Shift: shift, Alt: alt, Ctrl: ctrl}
		case 17:
			return TypeEvent{Key: KeyF6, Shift: shift, Alt: alt, Ctrl: ctrl}
		case 18:
			return TypeEvent{Key: KeyF7, Shift: shift, Alt: alt, Ctrl: ctrl}
		case 19:
			return TypeEvent{Key: KeyF8, Shift: shift, Alt: alt, Ctrl: ctrl}
		case 20:
			return TypeEvent{Key: KeyF9, Shift: shift, Alt: alt, Ctrl: ctrl}
		case 21:
			return TypeEvent{Key: KeyF10, Shift: shift, Alt: alt, Ctrl: ctrl}
		case 23:
			return TypeEvent{Key: KeyF11, Shift: shift, Alt: alt, Ctrl: ctrl}
		case 24:
			return TypeEvent{Key: KeyF12, Shift: shift, Alt: alt, Ctrl: ctrl}
		}
	case 'M', 'm':
		return parseMouse(params, final)
	}
	return nil
}

func parseSS3(data []byte) any {
	if len(data) < 3 {
		return nil
	}
	switch data[2] {
	case 'A':
		return TypeEvent{Key: KeyUp}
	case 'B':
		return TypeEvent{Key: KeyDown}
	case 'C':
		return TypeEvent{Key: KeyRight}
	case 'D':
		return TypeEvent{Key: KeyLeft}
	case 'H':
		return TypeEvent{Key: KeyHome}
	case 'F':
		return TypeEvent{Key: KeyEnd}
	// Many terminals emit F1–F4 as SS3 P/Q/R/S and keypad Enter as SS3 M.
	case 'P':
		return TypeEvent{Key: KeyF1}
	case 'Q':
		return TypeEvent{Key: KeyF2}
	case 'R':
		return TypeEvent{Key: KeyF3}
	case 'S':
		return TypeEvent{Key: KeyF4}
	case 'M':
		return TypeEvent{Key: KeyEnter}
	}
	return nil
}

// csiUSpecialKey maps a CSI-u codepoint to a KeyCode for the keys that are not
// plain printable runes: the C0 control codepoints (Tab/Enter/Esc/Backspace) and
// the Kitty keyboard-protocol functional-key range (F1–F12 at 57364–57375).
func csiUSpecialKey(code int) (KeyCode, bool) {
	switch code {
	case 9:
		return KeyTab, true
	case 13:
		return KeyEnter, true
	case 27:
		return KeyEscape, true
	case 127:
		return KeyBackspace, true
	}
	// Kitty functional keys: F1 = 57364 (0xE004) … F12 = 57375.
	if code >= 57364 && code <= 57375 {
		return KeyF1 + KeyCode(code-57364), true
	}
	return KeyUnknown, false
}

func parseCSIModifiers(params string) (bool, bool, bool) {
	if params == "" {
		return false, false, false
	}
	parts := strings.Split(params, ";")
	if len(parts) < 2 {
		return false, false, false
	}
	mod, err := strconv.Atoi(parts[len(parts)-1])
	if err != nil {
		return false, false, false
	}
	return decodeCSIModifier(mod)
}

// parseCSIParams splits a CSI parameter string into its leading numeric code and
// modifier value (e.g. "3;5" -> 3, 5). Missing or malformed fields default to a
// code of 0 and an unmodified value of 1. It serves both the CSU-u ('u') and
// CSI-tilde ('~') key encodings, which share this layout.
func parseCSIParams(params string) (int, int) {
	parts := strings.Split(params, ";")
	if len(parts) == 0 {
		return 0, 1
	}
	keyCode, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, 1
	}
	mod := 1
	if len(parts) > 1 {
		value, err := strconv.Atoi(parts[1])
		if err == nil {
			mod = value
		}
	}
	return keyCode, mod
}

func decodeCSIModifier(mod int) (bool, bool, bool) {
	if mod <= 1 {
		return false, false, false
	}
	flags := mod - 1
	shift := flags&1 != 0
	alt := flags&2 != 0
	ctrl := flags&4 != 0
	return shift, alt, ctrl
}

func parseMouse(params string, final byte) any {
	if len(params) == 0 || params[0] != '<' {
		return nil
	}
	parts := strings.Split(params[1:], ";")
	if len(parts) != 3 {
		return nil
	}
	cb, err1 := strconv.Atoi(parts[0])
	cx, err2 := strconv.Atoi(parts[1])
	cy, err3 := strconv.Atoi(parts[2])
	if err1 != nil || err2 != nil || err3 != nil {
		return nil
	}
	x := cx - 1
	y := cy - 1
	// SGR-encoded modifier and motion bits (shared by clicks and wheel).
	shift := cb&4 != 0
	alt := cb&8 != 0
	ctrl := cb&16 != 0
	motion := cb&32 != 0
	if cb&64 != 0 {
		delta := -1
		if cb&1 == 0 {
			delta = 1
		}
		return ScrollEvent{X: x, Y: y, Delta: delta}
	}
	button := MouseLeft
	switch cb & 3 {
	case 0:
		button = MouseLeft
	case 1:
		button = MouseMiddle
	case 2:
		button = MouseRight
	case 3:
		button = MouseLeft
	}
	noButton := (cb & 3) == 3
	// Down stays true for the motion ("drag") reports the terminal sends while a
	// button is held, so existing drag-aware widgets keep working; Drag/Move let
	// new code tell a fresh press, a drag, and a button-less move apart (issue #11).
	down := final == 'M' && !noButton
	drag := down && motion
	move := motion && noButton
	return ClickEvent{
		X:      x,
		Y:      y,
		Button: button,
		Down:   down,
		Drag:   drag,
		Move:   move,
		Shift:  shift,
		Alt:    alt,
		Ctrl:   ctrl,
	}
}
