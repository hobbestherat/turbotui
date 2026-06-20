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
)

type MouseButton uint8

const (
	MouseLeft MouseButton = iota
	MouseMiddle
	MouseRight
)

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
	Down   bool
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

	postChannel chan func()

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
		out:         out,
		width:       width,
		height:      height,
		front:       newScreen(width, height, Cell{}),
		back:        newScreen(width, height, DefaultCell()),
		lines:       make([]lineCell, width*height),
		postChannel: make(chan func(), 64),
	}
	app.invalidateFront()
	return app
}

func (a *App) Width() int {
	return a.width
}

func (a *App) Height() int {
	return a.height
}

func (a *App) ReadCell(x int, y int) Cell {
	return a.back.get(x, y)
}

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
// Post BLOCKS once the 64-deep queue is full: a background producer that outruns
// a slow event loop will stall until the loop drains. That is the safe default
// (it never drops an update), but if a producer cannot afford to block, use
// TryPost instead.
func (a *App) Post(fn func()) {
	if fn == nil {
		return
	}
	a.postChannel <- fn
}

// TryPost is the non-blocking analogue of Post: it enqueues fn and returns true
// when there was room, and returns false (leaving fn unscheduled) when the queue
// is full. Use it from background producers that must not stall against a slow
// event loop; prefer Post when every update has to run.
func (a *App) TryPost(fn func()) bool {
	if fn == nil {
		return true
	}
	select {
	case a.postChannel <- fn:
		return true
	default:
		return false
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
}

func (a *App) Apply() error {
	// On a real terminal, suppress flushes until Run has switched to the
	// alternate screen; otherwise frames composed during setup would leak onto
	// the normal buffer and remain visible after the alt screen is torn down.
	// Buffer-backed apps (termOut == nil, used in tests) always flush.
	if a.termOut != nil && !a.started {
		return nil
	}
	var output strings.Builder
	style := styleState{}
	for y := 0; y < a.height; y++ {
		rowOffset := y * a.width
		for x := 0; x < a.width; x++ {
			index := rowOffset + x
			next := a.back.cells[index]
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
			if next.Ch == 0 {
				next.Ch = ' '
			}
			output.WriteString(moveCursor(x, y))
			codes := styleCodes(style, next)
			output.WriteString(sgr(codes))
			output.WriteRune(next.Ch)
			if next.Combining != "" {
				output.WriteString(next.Combining)
			}
			style = styleState{
				valid:     true,
				fg:        next.FG,
				bg:        next.BG,
				bold:      next.Bold,
				underline: next.Underline,
			}
			a.front.cells[index] = next
		}
	}
	output.WriteString(a.cursorEscapes())
	if output.Len() == 0 {
		return nil
	}
	a.writeMu.Lock()
	_, err := io.WriteString(a.out, output.String())
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

// cursorEscapes returns the control sequence needed to bring the real terminal
// cursor in line with the desired state, or "" when nothing changed.
func (a *App) cursorEscapes() string {
	if a.cursorVisible {
		if a.frontCursorVisible && a.frontCursorX == a.cursorX && a.frontCursorY == a.cursorY {
			return ""
		}
		a.frontCursorVisible = true
		a.frontCursorX = a.cursorX
		a.frontCursorY = a.cursorY
		return moveCursor(a.cursorX, a.cursorY) + "\x1b[?25h"
	}
	if !a.frontCursorVisible {
		return ""
	}
	a.frontCursorVisible = false
	return "\x1b[?25l"
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

	resizeChannel := make(chan os.Signal, 1)
	signal.Notify(resizeChannel, syscall.SIGWINCH)
	defer signal.Stop(resizeChannel)

	// A lone ESC byte can't be parsed immediately: it might begin an escape
	// sequence. We hold it briefly and, if nothing follows, flush it as KeyEscape.
	var escTimer <-chan time.Time
	for {
		select {
		case <-ctx.Done():
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
		case fn := <-a.postChannel:
			fn()
		case <-resizeChannel:
			a.handleTerminalResize()
		}
		// A handler above (e.g. a redraw triggered by an event or Post) may have
		// failed to flush to the terminal. Treat that as fatal so the app exits
		// cleanly instead of spinning against a dead output.
		if a.applyErr != nil {
			return a.applyErr
		}
	}
}

func (a *App) Close() {
	if a.restoreState != nil {
		_, _ = io.WriteString(a.out, "\x1b[0m\x1b[?2004l\x1b[?1002l\x1b[?1006l\x1b[?25h\x1b[?1049l")
		_ = term.Restore(int(a.in.Fd()), a.restoreState)
		a.restoreState = nil
	}
	a.started = false
}

// CloseWithMessage restores the terminal, tears down the alternate screen (so the
// TUI disappears and the prompt returns), then prints message in the normal
// buffer right after the command that launched the app. The message may span
// multiple lines and contain ANSI styling (see Styled).
func (a *App) CloseWithMessage(message string) {
	_, _ = io.WriteString(a.out, "\x1b[0m\x1b[?2004l\x1b[?1002l\x1b[?1006l\x1b[?25h\x1b[?1049l")
	if a.restoreState != nil {
		_ = term.Restore(int(a.in.Fd()), a.restoreState)
		a.restoreState = nil
	}
	a.started = false
	message = strings.Trim(message, "\n")
	if strings.TrimSpace(message) == "" {
		return
	}
	_, _ = io.WriteString(a.out, message+"\n")
}

func (a *App) setupTerminal() error {
	state, err := term.MakeRaw(int(a.in.Fd()))
	if err != nil {
		return err
	}
	a.restoreState = state
	a.started = true
	_, err = io.WriteString(a.out, "\x1b[?1049h\x1b[?25l\x1b[?1002h\x1b[?1006h\x1b[?2004h")
	return err
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
	a.invalidateFront()
	_, _ = io.WriteString(a.out, "\x1b[2J\x1b[H")

	event := ResizeEvent{Width: width, Height: height}
	for _, handler := range a.resizeHandlers {
		handler(event)
	}
	_ = a.Apply()
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
		return TypeEvent{
			Key:  KeyRune,
			Rune: rune(head + 'a' - 1),
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
		event := parseSS3(data)
		if event == nil {
			return nil, 0, false
		}
		return event, 3, true
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
		if keyCode == 13 {
			return TypeEvent{Key: KeyEnter, Shift: shift, Alt: alt, Ctrl: ctrl}
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
	}
	return nil
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
	down := final == 'M' && (cb&3) != 3
	return ClickEvent{
		X:      x,
		Y:      y,
		Button: button,
		Down:   down,
	}
}
