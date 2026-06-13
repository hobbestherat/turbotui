package tui

import (
	"context"
	"errors"
	"io"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
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
	MouseWheelUp
	MouseWheelDown
)

type TypeEvent struct {
	Key  KeyCode
	Rune rune
	Ctrl bool
	Alt  bool
}

type ClickEvent struct {
	X      int
	Y      int
	Button MouseButton
	Down   bool
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

type App struct {
	in      *os.File
	out     io.Writer
	termOut *os.File

	width  int
	height int

	front screen
	back  screen
	lines []lineCell

	clickHandlers  []func(ClickEvent)
	scrollHandlers []func(ScrollEvent)
	typeHandlers   []func(TypeEvent)
	resizeHandlers []func(ResizeEvent)

	restoreState *term.State
	parser       inputParser
	started      bool
}

func New() (*App, error) {
	width, height, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || width < 1 || height < 1 {
		width = 80
		height = 25
	}
	return NewWithIO(os.Stdin, os.Stdout, width, height), nil
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
		out:    out,
		width:  width,
		height: height,
		front:  newScreen(width, height, Cell{}),
		back:   newScreen(width, height, DefaultCell()),
		lines:  make([]lineCell, width*height),
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

func (a *App) WriteCell(x int, y int, cell Cell) {
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
	for _, ch := range text {
		style.Ch = ch
		a.WriteCell(column, y, style)
		column++
	}
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
		needed := len(word)
		if col > 0 {
			needed++
		}
		if col+needed > width {
			line++
			col = 0
		}
		if col > 0 {
			style.Ch = ' '
			a.WriteCell(x+col, y+line, style)
			col++
		}
		for _, r := range word {
			if col >= width {
				line++
				col = 0
			}
			style.Ch = r
			a.WriteCell(x+col, y+line, style)
			col++
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
			if next.Ch == 0 {
				next.Ch = ' '
			}
			output.WriteString(moveCursor(x, y))
			codes := styleCodes(style, next)
			output.WriteString(sgr(codes))
			output.WriteRune(next.Ch)
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
	if output.Len() == 0 {
		return nil
	}
	_, err := io.WriteString(a.out, output.String())
	return err
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

	for {
		select {
		case <-ctx.Done():
			return nil
		case err := <-errorChannel:
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		case bytes := <-readChannel:
			for _, event := range a.parser.Feed(bytes) {
				a.dispatchEvent(event)
			}
		case <-resizeChannel:
			a.handleTerminalResize()
		}
	}
}

func (a *App) Close() {
	if a.restoreState != nil {
		_, _ = io.WriteString(a.out, "\x1b[0m\x1b[?1002l\x1b[?1006l\x1b[?25h\x1b[?1049l")
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
	_, _ = io.WriteString(a.out, "\x1b[0m\x1b[?1002l\x1b[?1006l\x1b[?25h\x1b[?1049l")
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
	_, err = io.WriteString(a.out, "\x1b[?1049h\x1b[?25l\x1b[?1002h\x1b[?1006h")
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
	if len(p.pending) > 4096 {
		p.pending = p.pending[:0]
	}
	return events
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
	if head == '\r' || head == '\n' {
		return TypeEvent{Key: KeyEnter}, 1, true
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
	event := parseCSI(params, final)
	return event, finalIndex + 1, true
}

func parseCSI(params string, final byte) any {
	switch final {
	case 'A':
		return TypeEvent{Key: KeyUp}
	case 'B':
		return TypeEvent{Key: KeyDown}
	case 'C':
		return TypeEvent{Key: KeyRight}
	case 'D':
		return TypeEvent{Key: KeyLeft}
	case 'Z':
		return TypeEvent{Key: KeyBackTab}
	case '~':
		value, _ := strconv.Atoi(params)
		switch value {
		case 1, 7:
			return TypeEvent{Key: KeyHome}
		case 2:
			return TypeEvent{Key: KeyInsert}
		case 3:
			return TypeEvent{Key: KeyDelete}
		case 4, 8:
			return TypeEvent{Key: KeyEnd}
		case 5:
			return TypeEvent{Key: KeyPageUp}
		case 6:
			return TypeEvent{Key: KeyPageDown}
		}
	case 'M', 'm':
		return parseMouse(params, final)
	}
	return nil
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
