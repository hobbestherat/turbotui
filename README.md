# turbotui

A small, dependency-light terminal-UI toolkit for Go, in the spirit of classic
Turbo Vision. It comes in two layers, both in this one module:

- **`turbotui`** (package `tui`) — the low-level engine: a double-buffered cell
  grid, an input parser (keyboard incl. Alt/Ctrl, mouse click/scroll, resize),
  box/line drawing, the alternate-screen lifecycle, and a `Run` event loop.
- **`turbotui/turbotv`** (package `tv`) — a retained-mode widget toolkit built on
  top: a desktop with stacked layers, windows, a drop-down menu bar with
  mnemonics and accelerators, buttons, text inputs, a multi-line editor, a
  scrollable text view, a drop-down select, labels, and dialogs.

Use `tui` directly when you want to paint cells yourself; use `tv` when you want
ready-made widgets, focus management and layering.

## Install

```sh
go get github.com/hobbestherat/turbotui
```

```go
import (
    tui "github.com/hobbestherat/turbotui"
    tv  "github.com/hobbestherat/turbotui/turbotv" // optional widget layer
)
```

Requires Go 1.22+. The only external dependency is `golang.org/x/term` (used to
put the terminal into raw mode).

## Quick start (low-level `tui`)

```go
package main

import (
    "context"
    "os"
    "os/signal"
    "syscall"

    tui "github.com/hobbestherat/turbotui"
)

func main() {
    app, err := tui.New()
    if err != nil {
        panic(err)
    }

    white := tui.ANSIColor(15)
    blue := tui.ANSIColor(4)

    draw := func() {
        app.Clear(tui.Cell{Ch: ' ', BG: blue})
        app.DrawBox(0, 0, app.Width(), app.Height(), tui.LineDouble, white, blue)
        app.WriteString(2, 1, "Hello terminal! Press q to quit.", tui.Cell{FG: white, BG: blue})
        _ = app.Apply() // flush the buffer to the screen
    }

    ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
    defer stop()

    app.OnType(func(event tui.TypeEvent) {
        if event.Key == tui.KeyRune && event.Rune == 'q' {
            stop()
        }
    })
    app.OnResize(func(_ tui.ResizeEvent) { draw() })

    draw()
    _ = app.Run(ctx)         // blocks until ctx is cancelled
    app.Close()              // restore the normal screen
}
```

### Core concepts

- **Cells.** Every screen position is a `tui.Cell{Ch, FG, BG, Bold, Underline}`.
  Colours come from `tui.ANSIColor(0..255)`, `tui.RGBColor(r,g,b)` or
  `tui.DefaultColor()`.
- **Double buffering.** Drawing calls (`WriteCell`, `WriteString`, `DrawBox`,
  `Clear`, …) mutate an off-screen buffer; `app.Apply()` diffs it against the
  visible frame and writes only what changed.
- **Event loop.** Register `OnType`, `OnClick`, `OnScroll`, `OnResize`, then call
  `app.Run(ctx)`. It enters the alternate screen, enables mouse reporting, and
  pumps input until `ctx` is cancelled.
- **Input.** `TypeEvent` carries `Key` (a `KeyCode` such as `KeyEnter`,
  `KeyUp`, `KeyRune`), `Rune`, and `Alt`/`Ctrl` modifiers. `ClickEvent` and
  `ScrollEvent` carry `X`/`Y` and button/direction.
- **Shutdown.** `app.Close()` tears the alternate screen down cleanly.
  `app.CloseWithMessage(msg)` does the same and then prints `msg` (multi-line and
  ANSI-coloured via `tui.Styled`) to the normal buffer — handy for printing run
  stats right after the UI exits.

### Construction helpers

- `tui.New()` — auto-sizes to the current terminal (the usual choice).
- `tui.NewWithIO(in, out, w, h)` — explicit files/size (e.g. a PTY).
- `tui.NewWithSize(w, h, out)` — buffer-backed, no real terminal; ideal for tests
  that inspect rendered output via `app.ReadCell(x, y)`.

## The widget layer

For real applications you usually want the `tv` package: a `Desktop` that owns
layers and a menu bar, plus composable widgets with built-in focus handling,
keyboard navigation and mnemonics. See **[turbotv/README.md](turbotv/README.md)**
for the full guide, and `turbotv/cmd/demo` for a complete example app.

## Demos

```sh
go run ./cmd/demo            # low-level tui demo (mouse, keyboard, resize)
go run ./turbotv/cmd/demo    # full widget demo (menus, windows, inputs, select)
```

In a demo, press `q` (or `Ctrl+Q` in the widget demo) to quit.

## Project layout

```
github.com/hobbestherat/turbotui      package tui   (engine; repo root)
  ├── app.go, screen.go, cell.go …    cell buffer, input, drawing, run loop
  ├── cmd/demo/                        low-level demo
  └── turbotv/                         package tv   (widget toolkit)
        ├── desktop.go, layer.go …     desktop, layers, compositing
        ├── window.go, menu.go …       widgets
        ├── widget_*.go                buttons, inputs, select, labels, …
        └── cmd/demo/                  widget demo
```

## Testing

```sh
go test ./...
```

Widget rendering is tested against buffer-backed apps (`tui.NewWithSize`) that
read cells back with `app.ReadCell`.

## License

See `LICENSE` (add one before publishing if you intend others to depend on it).
