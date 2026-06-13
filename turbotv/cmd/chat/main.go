package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	tui "github.com/hobbestherat/turbotui"
	tv "github.com/hobbestherat/turbotui/turbotv"
)

// Colours used across the chat demo.
var (
	colorUser  = tui.ANSIColor(14) // bright cyan
	colorAgent = tui.ANSIColor(10) // bright green
	colorNote  = tui.ANSIColor(8)  // dim grey
	colorLog   = tui.ANSIColor(11) // bright yellow
	colorInfo  = tui.ANSIColor(12) // bright blue
)

func main() {
	startTime := time.Now()
	app, err := tui.New()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to initialize TUI: %v\n", err)
		os.Exit(1)
	}
	desktop := tv.NewDesktop(app)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	menu := tv.NewMenuBar(tv.Rect{X: 0, Y: 0, W: app.Width(), H: 1},
		tv.NewSubMenu("&File",
			tv.NewMenuItem("E&xit", func() {
				stop()
			}).WithShortcut("Ctrl+Q", tui.KeyRune, 'q', true),
		),
		tv.NewSubMenu("&Help",
			tv.NewMenuItem("&About", func() {
				tv.ShowConfirmYesNo(desktop, "Chat demo", "Split-pane chat example for TurboTV.", nil)
			}),
		),
	)
	desktop.SetMenuBar(menu)

	// One full-screen window. compose() keeps it sized to the terminal, and the
	// Content LayoutFn below reflows the 80/20 split on every resize.
	window := tv.NewWindow("TurboTV Chat", tv.Rect{X: 0, Y: 1, W: app.Width(), H: app.Height() - 1}, tui.LineSingle)
	window.Shadow = false
	window.ShowClose = false

	history := tv.NewTextView("", tv.Rect{})
	history.Wrap = true
	history.AddColored("[Agent] Welcome! Type a message and press Enter (Shift+Enter for a new line) or Send.", colorAgent)

	input := tv.NewMultiLineInput("", tv.Rect{})
	// Default mode: Enter sends, Shift+Enter inserts a newline.
	input.SubmitMode = tv.MultiLineSubmitOnEnter
	sendButton := tv.NewButton("&Send", tv.Rect{}, nil)

	log := tv.NewTextView("", tv.Rect{})
	log.Wrap = true
	log.AddColored("[log] session started", colorInfo)

	modelLabel := tv.NewLabel("&Model", tv.Rect{})
	model := tv.NewSelect(desktop, []string{"GPT-4o", "Claude 3.5 Sonnet", "Llama 3.1", "Gemini 1.5", "Mistral Large"}, tv.Rect{})
	modelLabel.SetTarget(model)
	model.OnChange = func(index int) {
		log.AddColored("[model] switched to "+model.Options[index], colorLog)
		desktop.Redraw()
	}

	tools := []*tv.Button{
		tv.NewButton("Summ&arize", tv.Rect{}, func() {
			log.AddColored("[tool] summarize requested", colorLog)
			desktop.Redraw()
		}),
		tv.NewButton("Sear&ch", tv.Rect{}, func() {
			log.AddColored("[tool] search the workspace", colorLog)
			desktop.Redraw()
		}),
		tv.NewButton("&Run", tv.Rect{}, func() {
			log.AddColored("[tool] run build & tests", colorLog)
			desktop.Redraw()
		}),
	}

	send := func() {
		text := strings.TrimSpace(input.GetText())
		if text == "" {
			return
		}
		history.AddColored("[User] "+text, colorUser)
		input.Clear()
		log.AddColored(fmt.Sprintf("[chat] sent %d chars to %s", len([]rune(text)), model.Value()), colorLog)

		// The agent entry is foldable: its child note can be collapsed by clicking
		// the ▾ marker. We stream the reversed text into the entry's own line.
		reply := history.AddColored("[Agent] ", colorAgent)
		reply.AddColored("(simulated reply: your message reversed)", colorNote)
		history.ScrollToBottom()
		desktop.Redraw()

		runes := []rune(reverseRunes([]rune(text)))
		go func() {
			for _, r := range runes {
				time.Sleep(35 * time.Millisecond)
				chunk := string(r)
				desktop.Post(func() {
					reply.AppendText(chunk)
				})
			}
		}()
	}
	sendButton.OnPress = send
	input.OnSubmit = send

	// splitRatio is the left pane's fraction of the content width; the divider can
	// be dragged with the mouse to change it (where the terminal reports drags).
	splitRatio := 80
	divider := tv.NewComponent(tv.Rect{})
	divider.DrawFn = func(component *tv.VisualComponent, surface tv.Surface) {
		abs := component.AbsoluteBounds()
		for y := 0; y < abs.H; y++ {
			surface.SetCell(abs.X, abs.Y+y, tui.Cell{Ch: '│', FG: tui.ANSIColor(7), BG: tui.ANSIColor(4)})
		}
	}
	divider.OnClickFn = func(component *tv.VisualComponent, event tui.ClickEvent) bool {
		if !event.Down {
			return true
		}
		content := component.Parent
		if content == nil {
			return true
		}
		abs := content.AbsoluteBounds()
		if abs.W < 4 {
			return true
		}
		ratio := (event.X - abs.X) * 100 / abs.W
		if ratio < 20 {
			ratio = 20
		}
		if ratio > 90 {
			ratio = 90
		}
		splitRatio = ratio
		content.SetBounds(content.Bounds) // re-run the split LayoutFn
		desktop.Redraw()
		return true
	}

	leftLabel := tv.NewLabel("Chat", tv.Rect{})
	rightLabel := tv.NewLabel("Log", tv.Rect{})

	left := tv.NewComponent(tv.Rect{})
	left.AddChild(leftLabel)
	left.AddChild(history)
	left.AddChild(input)
	left.AddChild(sendButton)
	left.LayoutFn = func(component *tv.VisualComponent) {
		w := component.Bounds.W
		h := component.Bounds.H
		inputH := 3
		leftLabel.Component.SetBounds(tv.Rect{X: 1, Y: 0, W: w - 2, H: 1})
		history.Component.SetBounds(tv.Rect{X: 1, Y: 1, W: w - 1, H: h - inputH - 2})
		input.Component.SetBounds(tv.Rect{X: 1, Y: h - inputH - 1, W: w - 11, H: inputH})
		sendButton.Component.SetBounds(tv.Rect{X: w - 9, Y: h - inputH - 1, W: 8, H: 1})
	}

	right := tv.NewComponent(tv.Rect{})
	right.AddChild(rightLabel)
	right.AddChild(log)
	right.AddChild(modelLabel)
	right.AddChild(model)
	for _, button := range tools {
		right.AddChild(button)
	}
	right.LayoutFn = func(component *tv.VisualComponent) {
		w := component.Bounds.W
		h := component.Bounds.H
		toolsH := 2 + len(tools) + 1
		rightLabel.Component.SetBounds(tv.Rect{X: 1, Y: 0, W: w - 2, H: 1})
		log.Component.SetBounds(tv.Rect{X: 1, Y: 1, W: w - 1, H: h - toolsH - 1})
		base := h - toolsH
		modelLabel.Component.SetBounds(tv.Rect{X: 1, Y: base, W: 7, H: 1})
		model.Component.SetBounds(tv.Rect{X: 1, Y: base + 1, W: w - 2, H: 1})
		for index, button := range tools {
			button.Component.SetBounds(tv.Rect{X: 1, Y: base + 3 + index, W: w - 2, H: 1})
		}
	}

	window.AddContent(left)
	window.AddContent(divider)
	window.AddContent(right)
	// Reflow the split whenever the window (and thus its content) is resized.
	window.Content.LayoutFn = func(content *tv.VisualComponent) {
		w := content.Bounds.W
		h := content.Bounds.H
		leftW := w * splitRatio / 100
		if leftW < 1 {
			leftW = 1
		}
		if leftW > w-2 {
			leftW = w - 2
		}
		left.SetBounds(tv.Rect{X: 0, Y: 0, W: leftW, H: h})
		divider.SetBounds(tv.Rect{X: leftW, Y: 0, W: 1, H: h})
		right.SetBounds(tv.Rect{X: leftW + 1, Y: 0, W: w - leftW - 1, H: h})
	}

	desktop.AddLayer(tv.NewFullscreenLayer("chat", window))
	desktop.SetFocus(input)

	quitting := false
	app.OnType(func(event tui.TypeEvent) {
		if event.Key == tui.KeyRune && event.Rune == 'c' && event.Ctrl {
			if quitting {
				return
			}
			quitting = true
			tv.ShowConfirmYesNo(desktop, "Quit", "Quit the chat demo?", func(yes bool) {
				quitting = false
				if yes {
					stop()
				}
			})
		}
	})

	if err := desktop.Run(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "chat run failed: %v\n", err)
		os.Exit(1)
	}

	elapsed := time.Since(startTime).Round(time.Millisecond)
	summary := strings.Join([]string{
		tui.Styled("TurboTV chat demo closed.", tui.ANSIColor(15), tui.DefaultColor(), true),
		tui.Styled(fmt.Sprintf("  Ran for %s", elapsed), tui.ANSIColor(10), tui.DefaultColor(), false),
		tui.Styled(fmt.Sprintf("  Last model: %s", model.Value()), tui.ANSIColor(14), tui.DefaultColor(), false),
	}, "\n")
	app.CloseWithMessage(summary)
}

func reverseRunes(runes []rune) string {
	out := make([]rune, len(runes))
	for index, r := range runes {
		out[len(runes)-1-index] = r
	}
	return string(out)
}
