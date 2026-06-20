// Package tv is the retained-mode widget toolkit of turbotui, built on top of
// the low-level engine in package tui. It gives you a Turbo-Vision-style
// [Desktop] with stacked [Layer]s, a drop-down [MenuBar] with mnemonics and
// accelerators, windows, dialogs and a set of focusable widgets (buttons, text
// inputs, a multi-line editor, a scrollable text view, a drop-down select,
// labels, checkboxes and a tree) — all with built-in keyboard navigation, mouse
// support and focus management.
//
// # The model
//
//	Desktop                owns the screen, the menu bar and a stack of layers
//	 ├── MenuBar           always drawn on top of every layer
//	 └── []*Layer          painted bottom-to-top; the top input layer has focus
//	       └── Root Widget a Window, a Dialog or any VisualComponent tree
//	             └── children   Button, Label, TextBox, Select, …
//
// A [Desktop] wraps a *tui.App, composes every layer each frame, routes input
// and manages focus. Build it with [NewDesktop], add layers with
// [Desktop.AddLayer], and run it with [Desktop.Run]. The desktop is
// single-threaded: mutate it from an input handler or, from a background
// goroutine, via [Desktop.Post]. A [Layer] is one entry in the z-stack;
// [NewFullscreenLayer], [NewWindowLayer] and [NewModalLayer] cover the common
// cases (a modal layer captures all input while it is on top).
//
// # Widgets and VisualComponent
//
// A [Widget] is anything exposing Root() *[VisualComponent]; every widget in this
// package satisfies it, so you add them directly to windows and layers. The
// shared node is [VisualComponent]: a bounds rectangle, visibility/enabled/focus
// state, parent/children links and a set of On*Fn callbacks the desktop invokes
// (DrawFn to paint, OnTypeFn/OnClickFn/OnScrollFn for input, OnFocusFn on focus
// change, and so on). A callback that handles input returns true to consume the
// event; returning false lets it bubble to the parent. You can build a widget
// two ways: assign closures to those fields (see the custom-widget example), or
// implement the matching capability interface ([Painter], [Typer], [Clicker], …)
// on your own type and wire it in with [VisualComponent.Bind] for a
// compile-time-checked contract.
//
// # Focus and mnemonics
//
// Within the top layer, Tab/Shift+Tab and the arrow keys move focus between
// focusable widgets in reading order (top-to-bottom, then left-to-right);
// [VisualComponent].TabIndex overrides the order. A label declared with an '&'
// (for example "&Quit") gets a mnemonic: while its scope is active, Alt plus the
// hot letter triggers OnActivateFn or moves focus to the widget. The active
// [Theme] highlights the hot character; swap palettes with [SetTheme].
//
// See the package-level example for a minimal app and Example (custom widget)
// for composing a new widget from a VisualComponent.
package tv
