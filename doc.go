// Package tui is the low-level terminal-UI engine of turbotui, in the spirit of
// classic Turbo Vision. It gives you a double-buffered cell grid, an input
// parser (keyboard with Alt/Ctrl/Shift, mouse click/scroll, resize and
// bracketed paste), box/line drawing, the alternate-screen lifecycle and a Run
// event loop. Use it directly when you want to paint cells yourself; reach for
// the sibling package turbotui/turbotv when you want ready-made widgets, focus
// management and layering.
//
// # The cell grid
//
// Every screen position is a [Cell] holding a rune plus foreground/background
// [Color] and Bold/Underline flags. Colours come from [ANSIColor] (0..255),
// [RGBColor] or [DefaultColor]. Drawing calls — [App.WriteCell],
// [App.WriteString], [App.DrawBox], [App.Clear] — mutate an off-screen buffer;
// [App.Apply] diffs that buffer against the visible frame and writes only the
// cells that changed, so redraws are cheap.
//
// # The event loop
//
// Construct an App, register the handlers you need, then call [App.Run]:
//
//	app := tui.New()
//	app.OnType(func(e tui.TypeEvent) {
//		if e.Key == tui.KeyRune && e.Rune == 'q' {
//			app.Close()
//		}
//	})
//	app.Run(ctx) // enters the alternate screen and pumps input until ctx is cancelled
//
// [App.OnType], [App.OnPaste], [App.OnClick], [App.OnScroll] and [App.OnResize]
// register the callbacks. [TypeEvent] carries a [KeyCode] (such as [KeyEnter],
// [KeyUp] or [KeyRune]), a Rune and Alt/Ctrl/Shift modifiers; [ClickEvent] and
// [ScrollEvent] carry coordinates and a button/direction; [PasteEvent] carries
// the literal Text of a bracketed paste as one block, so embedded newlines never
// look like Enter presses. Run is single-threaded: call the App's methods from a
// handler, or from a background goroutine schedule them with [App.Post].
//
// # Construction
//
// [New] auto-sizes to the current terminal and never fails (an unreadable size
// falls back to 80×25); [App.Validate] then reports whether stdin/stdout are a
// real terminal. [NewWithIO] takes explicit files and a size (e.g. a PTY).
// [NewWithSize] is buffer-backed with no real terminal — ideal for tests that
// render a frame and inspect it with [App.ReadCell], which is exactly how the
// runnable examples in this package work.
//
// See the package-level example for a minimal program, and turbotui/turbotv for
// the widget layer.
package tui
