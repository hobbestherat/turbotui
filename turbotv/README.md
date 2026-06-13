# turbotv

`turbotv` (package `tv`) is the retained-mode widget toolkit of
[turbotui](../README.md). It gives you a Turbo-Vision-style desktop with stacked
layers, a top menu bar, windows, dialogs and a set of focusable widgets — all
with built-in keyboard navigation, mouse support and mnemonics.

```go
import (
    tui "github.com/hobbestherat/turbotui"
    tv  "github.com/hobbestherat/turbotui/turbotv"
)
```

## The big picture

```
Desktop                       owns the screen, the menu bar and a stack of layers
 ├── MenuBar                  always drawn on top of every layer
 └── []*Layer                 painted bottom-to-top; the top input layer has focus
       └── Root Widget        a Window, a Dialog, or any VisualComponent tree
             └── children     Button, Label, TextBox, Select, …
```

- A **`Desktop`** wraps a `*tui.App`, composes all layers each frame, routes
  input, and manages focus. Create it with `tv.NewDesktop(app)` and run it with
  `desktop.Run(ctx)`.
- A **`Layer`** is one entry in the z-stack. Helpers: `tv.NewFullscreenLayer`
  (background), `tv.NewWindowLayer` (normal window — menu shortcuts from below
  stay live), `tv.NewModalLayer` (dialog — captures all input while on top).
- A **`Widget`** is anything exposing `Root() *VisualComponent`. Every widget in
  this package satisfies it, so you add them directly to windows and layers.

## Minimal app

```go
package main

import (
    "context"
    "os"
    "os/signal"
    "syscall"

    tui "github.com/hobbestherat/turbotui"
    tv  "github.com/hobbestherat/turbotui/turbotv"
)

func main() {
    app, _ := tui.New()
    desktop := tv.NewDesktop(app)

    ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
    defer stop()

    // A window with one button.
    window := tv.NewWindow("Hello", tv.Rect{X: 5, Y: 3, W: 40, H: 10}, tui.LineDouble)
    window.OnClose = func(_ *tv.Window) { desktop.RemoveTopLayer() }

    button := tv.NewButton("&Quit", tv.Rect{X: 2, Y: 2, W: 12, H: 1}, func() { stop() })
    window.AddContent(button)
    desktop.AddLayer(tv.NewWindowLayer("main", window))

    _ = desktop.Run(ctx)
    app.CloseWithMessage("Bye!")
}
```

## Widgets

| Widget               | Constructor                                            | Notes |
|----------------------|--------------------------------------------------------|-------|
| `Window`             | `NewWindow(title, bounds, border)`                     | `AddContent`, `AddBottom`, `OnClose`, draggable, close button |
| `Button`             | `NewButton(label, bounds, onPress)`                    | Focus shown as `►Label◄`; Enter/Space activate |
| `Label`              | `NewLabel(text, bounds)`                               | `SetTarget(widget)` forwards its mnemonic to another widget |
| `TextBox`            | `NewTextBox(text, bounds)`                             | Single-line input; `GetText`/`SetText` |
| `MultiLineInput`     | `NewMultiLineInput(text, bounds)`                      | Multi-line editor; `GetText` |
| `TextView`           | `NewTextView(text, bounds)`                            | Read-only, mouse-wheel scrollable; `SetText` |
| `Select`             | `NewSelect(desktop, options, bounds)`                  | Drop-down combo; `Value`, `GetSelected`, `SetSelected`, `OnChange` |
| `MenuBar`            | `NewMenuBar(bounds, menus...)`                         | See below |
| `Dialog`             | `NewDialog(title, x, y, w, h)`                         | Centered panel for modal layers |

All input widgets are focusable; `Tab`/`Shift+Tab` and arrow keys move focus
within the top layer. State is read/written with explicit methods
(`GetText()/SetText()`, `Value()/SetSelected()`) — there are no getters/setters.

### Select (drop-down)

```go
region := tv.NewSelect(desktop, []string{"Europe", "Asia", "Americas"}, tv.Rect{X: 2, Y: 4, W: 18, H: 1})
region.OnChange = func(index int) { /* ... */ }
window.AddContent(region)
```

The list opens on a desktop-owned popup layer, so it is **never clipped** by the
window that hosts it. Keyboard: `Enter`/`Space` open, `↑`/`↓` move, `Enter`
pick, `Esc` cancel. Mouse: click to open, click an item to pick, click outside to
dismiss.

## Menus, mnemonics and accelerators

Build a menu tree and hand it to the desktop:

```go
menu := tv.NewMenuBar(tv.Rect{},
    tv.NewSubMenu("&File",
        tv.NewMenuItem("&Open", onOpen).WithShortcut("Ctrl+O", tui.KeyRune, 'o', true),
        tv.NewMenuItem("&Quit", onQuit),
    ),
    tv.NewSubMenu("&Help",
        tv.NewMenuItem("&About", onAbout),
    ),
)
desktop.SetMenuBar(menu) // drawn on top of all layers; do NOT add it to a layer
```

- **Mnemonics** are marked with `&` in any label (`"&File"`, `"C&onfirm"`,
  `"&Name"`; a literal ampersand is `"&&"`). The marked character is highlighted
  and activated with **Alt+letter**.
- **Scope & layering.** Only mnemonics in the active top layer (plus the menu
  bar, unless a modal layer is on top) are live, and the hot characters are
  highlighted only there. If two widgets in the same scope claim the same letter,
  the first one wins.
- **Chained menu navigation.** `Alt+F` opens File; with it open, a plain mnemonic
  letter selects an item — so `Alt+F` then `f` runs File→Find. `←`/`→` switch
  between top menus, `↑`/`↓` move within a dropdown, `Enter` selects, `Esc`
  closes.
- **Accelerators** (`WithShortcut`) such as `Ctrl+O` fire from anywhere, whether
  or not a menu is open.
- **Label forwarding.** `label.SetTarget(input)` makes the label's mnemonic move
  focus to another widget — e.g. a `&Name` label above a name field so `Alt+N`
  focuses the field.

## Dialogs

```go
tv.ShowConfirmYesNo(desktop, "Confirm", "Apply values?", func(yes bool) {
    // ...
})
```

`ShowConfirmYesNo` pushes a modal layer and returns it. For custom dialogs build
a `tv.NewDialog(...)`, add widgets, and push it with `tv.NewModalLayer(...)`.

## Theming

`desktop.SetTheme(theme)` accepts a `tv.Theme` (start from `tv.DefaultTheme` and
override fields). It controls window/desktop/button/input/dialog colours, the
mnemonic highlight colour, button focus colours, and more.

## Building your own widget

A widget is just a struct holding a `*VisualComponent` and wiring callbacks:

```go
type Gauge struct{ Component *tv.VisualComponent; value int }

func NewGauge(bounds tv.Rect) *Gauge {
    g := &Gauge{}
    g.Component = tv.NewComponent(bounds)
    g.Component.Focusable = true
    g.Component.DrawFn = g.draw                 // paint into the clipped Surface
    g.Component.OnTypeFn = g.handleType         // return true if you consumed the key
    return g
}
func (g *Gauge) Root() *tv.VisualComponent { return g.Component }
func (g *Gauge) draw(c *tv.VisualComponent, s tv.Surface) { /* s.Fill / s.WriteString / s.DrawBox */ }
func (g *Gauge) handleType(_ *tv.VisualComponent, e tui.TypeEvent) bool { return false }
```

Set `Component.DrawOutside = true` for widgets (like menus or focused buttons
with chevrons) that need to paint slightly beyond their own bounds.

## Demo

```sh
go run ./turbotv/cmd/demo
```

The demo wires up a menu bar, a non-modal window with labelled text fields, a
multi-line editor, a scrollable note view, a drop-down `Select`, a focusable
button, and a coloured multi-line exit message. Quit with `Ctrl+Q` or `q`.
