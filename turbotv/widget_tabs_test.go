package tv

import (
	"bytes"
	"reflect"
	"strings"
	"testing"

	tui "github.com/hobbestherat/turbotui"
)

func screenText(app *tui.App) string {
	var b strings.Builder
	for y := 0; y < app.Height(); y++ {
		for x := 0; x < app.Width(); x++ {
			ch := app.ReadCell(x, y).Ch
			if ch == 0 {
				ch = ' '
			}
			b.WriteRune(ch)
		}
		b.WriteByte('\n')
	}
	return b.String()
}

func newTabsTestDesktop(w, h int) (*tui.App, *Desktop) {
	app := tui.NewWithSize(w, h, &bytes.Buffer{})
	desktop := NewDesktop(app)
	return app, desktop
}

func TestTabsRenderStripAndOnlyActiveContent(t *testing.T) {
	app, desktop := newTabsTestDesktop(50, 8)
	tabs := NewTabs(desktop, Rect{X: 2, Y: 1, W: 40, H: 5})
	tabs.AddTab("Scope", NewLabel("scope panel", Rect{W: 20, H: 1}))
	tabs.AddTab("Tech", NewLabel("tech panel", Rect{W: 20, H: 1}))
	desktop.AddLayer(NewLayer("tabs", tabs, true, false))

	desktop.Redraw()
	screen := screenText(app)
	for _, want := range []string{"Scope", "Tech", "scope panel"} {
		if !strings.Contains(screen, want) {
			t.Fatalf("expected rendered screen to contain %q:\n%s", want, screen)
		}
	}
	if strings.Contains(screen, "tech panel") {
		t.Fatalf("inactive tab content was rendered:\n%s", screen)
	}

	tabs.SetActive(1)
	desktop.Redraw()
	screen = screenText(app)
	if !strings.Contains(screen, "tech panel") {
		t.Fatalf("expected active tab content to render after switch:\n%s", screen)
	}
	if strings.Contains(screen, "scope panel") {
		t.Fatalf("previous tab content remained visible after switch:\n%s", screen)
	}
}

func TestTabsActiveLabelUsesActiveStyle(t *testing.T) {
	app, desktop := newTabsTestDesktop(24, 4)
	tabs := NewTabs(desktop, Rect{X: 0, Y: 0, W: 20, H: 3})
	tabs.FG = tui.ANSIColor(1)
	tabs.BG = tui.ANSIColor(2)
	tabs.ActiveFG = tui.ANSIColor(3)
	tabs.ActiveBG = tui.ANSIColor(4)
	tabs.AddTab("One", NewLabel("first", Rect{W: 10, H: 1}))
	tabs.AddTab("Two", NewLabel("second", Rect{W: 10, H: 1}))
	desktop.AddLayer(NewLayer("tabs", tabs, true, false))

	desktop.Redraw()

	active := app.ReadCell(1, 0)
	if active.FG != tabs.ActiveFG || active.BG != tabs.ActiveBG || !active.Bold {
		t.Fatalf("active tab cell style = fg %v bg %v bold %v, want fg %v bg %v bold true",
			active.FG, active.BG, active.Bold, tabs.ActiveFG, tabs.ActiveBG)
	}
	inactive := app.ReadCell(6, 0)
	if inactive.FG != tabs.FG || inactive.BG != tabs.BG || inactive.Bold {
		t.Fatalf("inactive tab cell style = fg %v bg %v bold %v, want fg %v bg %v bold false",
			inactive.FG, inactive.BG, inactive.Bold, tabs.FG, tabs.BG)
	}
}

func TestTabsKeyboardSwitchesAndFiresOnTabChange(t *testing.T) {
	_, desktop := newTabsTestDesktop(60, 10)
	tabs := NewTabs(desktop, Rect{X: 0, Y: 0, W: 50, H: 5})
	first := NewTextBox("", Rect{W: 12, H: 1})
	second := NewTextBox("", Rect{W: 12, H: 1})
	third := NewTextBox("", Rect{W: 12, H: 1})
	tabs.AddTab("One", first)
	tabs.AddTab("Two", second)
	tabs.AddTab("Three", third)
	desktop.AddLayer(NewLayer("tabs", tabs, true, false))
	desktop.SetFocus(first)

	var changes []int
	tabs.OnTabChange = func(index int) {
		changes = append(changes, index)
	}

	desktop.handleType(tui.TypeEvent{Key: tui.KeyRight, Alt: true})
	if tabs.Active() != 1 {
		t.Fatalf("Alt+Right active tab = %d, want 1", tabs.Active())
	}
	if !second.Component.Focused() {
		t.Fatalf("focus did not move into the newly active tab")
	}

	desktop.handleType(tui.TypeEvent{Key: tui.KeyTab, Ctrl: true})
	if tabs.Active() != 2 {
		t.Fatalf("Ctrl+Tab active tab = %d, want 2", tabs.Active())
	}

	desktop.handleType(tui.TypeEvent{Key: tui.KeyLeft, Alt: true})
	if tabs.Active() != 1 {
		t.Fatalf("Alt+Left active tab = %d, want 1", tabs.Active())
	}

	desktop.handleType(tui.TypeEvent{Key: tui.KeyTab, Ctrl: true, Shift: true})
	if tabs.Active() != 0 {
		t.Fatalf("Ctrl+Shift+Tab active tab = %d, want 0", tabs.Active())
	}

	want := []int{1, 2, 1, 0}
	if !reflect.DeepEqual(changes, want) {
		t.Fatalf("OnTabChange calls = %v, want %v", changes, want)
	}
}

func TestTabsAltArrowSwitchesFromMultiLineInput(t *testing.T) {
	_, desktop := newTabsTestDesktop(60, 12)
	tabs := NewTabs(desktop, Rect{X: 0, Y: 0, W: 50, H: 6})
	first := NewMultiLineInput("line one\nline two", Rect{W: 20, H: 3})
	second := NewTextBox("", Rect{W: 12, H: 1})
	tabs.AddTab("Notes", first)
	tabs.AddTab("Next", second)
	desktop.AddLayer(NewLayer("tabs", tabs, true, false))
	desktop.SetFocus(first)

	desktop.handleType(tui.TypeEvent{Key: tui.KeyRight, Alt: true})

	if tabs.Active() != 1 {
		t.Fatalf("Alt+Right from multiline input active tab = %d, want 1", tabs.Active())
	}
	if !second.Component.Focused() {
		t.Fatalf("Alt+Right from multiline input should focus first field in new tab")
	}
}

func TestTabsAltArrowSwitchTakesPriorityOverChildArrowHandler(t *testing.T) {
	_, desktop := newTabsTestDesktop(60, 10)
	tabs := NewTabs(desktop, Rect{X: 0, Y: 0, W: 50, H: 5})
	greedy := NewComponent(Rect{W: 12, H: 1})
	greedy.Focusable = true
	greedy.OnTypeFn = func(_ *VisualComponent, event tui.TypeEvent) bool {
		return event.Key == tui.KeyRight && event.Alt
	}
	next := NewTextBox("", Rect{W: 12, H: 1})
	tabs.AddTab("Greedy", greedy)
	tabs.AddTab("Next", next)
	desktop.AddLayer(NewLayer("tabs", tabs, true, false))
	desktop.SetFocus(greedy)

	desktop.handleType(tui.TypeEvent{Key: tui.KeyRight, Alt: true})

	if tabs.Active() != 1 {
		t.Fatalf("Alt+Right should switch tabs before active content handles it; active tab = %d, want 1", tabs.Active())
	}
	if !next.Component.Focused() {
		t.Fatalf("Alt+Right should focus the first focusable in the newly active tab")
	}
}

func TestTabsPlainTabCyclesFocusWithinActiveTab(t *testing.T) {
	_, desktop := newTabsTestDesktop(70, 10)
	root := NewComponent(Rect{X: 0, Y: 0, W: 70, H: 10})
	tabs := NewTabs(desktop, Rect{X: 0, Y: 0, W: 50, H: 5})
	activeBoxA := NewTextBox("", Rect{W: 10, H: 1})
	activeBoxB := NewTextBox("", Rect{Y: 1, W: 10, H: 1})
	activePanel := NewComponent(Rect{W: 20, H: 3})
	activePanel.AddChild(activeBoxA)
	activePanel.AddChild(activeBoxB)
	inactiveBox := NewTextBox("", Rect{W: 10, H: 1})
	outside := NewTextBox("", Rect{X: 55, Y: 0, W: 10, H: 1})

	tabs.AddTab("Active", activePanel)
	tabs.AddTab("Hidden", inactiveBox)
	root.AddChild(tabs)
	root.AddChild(outside)
	desktop.AddLayer(NewLayer("root", root, true, false))
	desktop.SetFocus(activeBoxA)

	desktop.handleType(tui.TypeEvent{Key: tui.KeyTab})
	if !activeBoxB.Component.Focused() {
		t.Fatalf("plain Tab should move to the next focusable in the active tab")
	}
	if outside.Component.Focused() || inactiveBox.Component.Focused() {
		t.Fatalf("plain Tab escaped the active tab")
	}

	desktop.handleType(tui.TypeEvent{Key: tui.KeyTab})
	if !activeBoxA.Component.Focused() {
		t.Fatalf("plain Tab should wrap within the active tab")
	}
	if outside.Component.Focused() || inactiveBox.Component.Focused() {
		t.Fatalf("plain Tab escaped the active tab after wrapping")
	}

	desktop.handleType(tui.TypeEvent{Key: tui.KeyBackTab})
	if !activeBoxB.Component.Focused() {
		t.Fatalf("Shift+Tab should wrap backward within the active tab")
	}
}

func TestTabsClickingTabLabelFocusesNewTabFromOutside(t *testing.T) {
	_, desktop := newTabsTestDesktop(50, 8)
	root := NewComponent(Rect{X: 0, Y: 0, W: 50, H: 8})
	tabs := NewTabs(desktop, Rect{X: 0, Y: 0, W: 35, H: 4})
	first := NewTextBox("", Rect{W: 10, H: 1})
	second := NewTextBox("", Rect{W: 10, H: 1})
	outside := NewTextBox("", Rect{X: 40, Y: 0, W: 8, H: 1})
	tabs.AddTab("First", first)
	tabs.AddTab("Second", second)
	root.AddChild(tabs)
	root.AddChild(outside)
	desktop.AddLayer(NewLayer("root", root, true, false))
	desktop.SetFocus(outside)

	// " First " occupies x=[0,7), so x=8 is inside " Second ".
	desktop.handleClick(tui.ClickEvent{X: 8, Y: 0, Button: tui.MouseLeft, Down: true})
	desktop.handleClick(tui.ClickEvent{X: 8, Y: 0, Button: tui.MouseLeft, Down: false})

	if tabs.Active() != 1 {
		t.Fatalf("clicking second tab active tab = %d, want 1", tabs.Active())
	}
	if !second.Component.Focused() {
		t.Fatalf("clicking a tab from outside should move focus into the newly active tab")
	}
}

func TestTabsClickingTabLabelActivatesTab(t *testing.T) {
	_, desktop := newTabsTestDesktop(40, 6)
	tabs := NewTabs(desktop, Rect{X: 2, Y: 1, W: 30, H: 4})
	tabs.AddTab("Alpha", NewLabel("alpha content", Rect{W: 20, H: 1}))
	tabs.AddTab("Beta", NewLabel("beta content", Rect{W: 20, H: 1}))
	desktop.AddLayer(NewLayer("tabs", tabs, true, false))

	var changes []int
	tabs.OnTabChange = func(index int) {
		changes = append(changes, index)
	}

	// " Alpha " occupies x=[2,9), so x=10 is inside " Beta ".
	desktop.handleClick(tui.ClickEvent{X: 10, Y: 1, Button: tui.MouseLeft, Down: true})
	desktop.handleClick(tui.ClickEvent{X: 10, Y: 1, Button: tui.MouseLeft, Down: false})

	if tabs.Active() != 1 {
		t.Fatalf("clicking Beta active tab = %d, want 1", tabs.Active())
	}
	if !reflect.DeepEqual(changes, []int{1}) {
		t.Fatalf("OnTabChange calls = %v, want [1]", changes)
	}
}

func TestTabsEmptyAndOutOfRangeAreNoOps(t *testing.T) {
	_, desktop := newTabsTestDesktop(20, 5)
	tabs := NewTabs(desktop, Rect{W: 20, H: 4})
	if tabs.Count() != 0 {
		t.Fatalf("empty tabs Count = %d, want 0", tabs.Count())
	}
	if tabs.ActiveContent() != nil {
		t.Fatalf("empty tabs ActiveContent should be nil")
	}
	tabs.AddTab("Nil", nil)
	if tabs.Count() != 0 {
		t.Fatalf("nil content should not add a tab, got count %d", tabs.Count())
	}
	if handled := tabs.HandleType(tui.TypeEvent{Key: tui.KeyRight, Alt: true}); handled {
		t.Fatalf("empty tabs should not consume Alt+Right")
	}

	tabs.AddTab("Only", NewLabel("only", Rect{W: 10, H: 1}))
	calls := 0
	tabs.OnTabChange = func(int) { calls++ }
	tabs.SetActive(-1)
	tabs.SetActive(1)
	if tabs.Active() != 0 {
		t.Fatalf("out-of-range SetActive changed active tab to %d", tabs.Active())
	}
	if calls != 0 {
		t.Fatalf("out-of-range SetActive fired OnTabChange %d times", calls)
	}
}

func TestRadioGroupPreservesExistingCheckboxCallbacks(t *testing.T) {
	var lowCalls []bool
	var highCalls []bool
	low := NewCheckbox("low", Rect{W: 8, H: 1}, func(checked bool) {
		lowCalls = append(lowCalls, checked)
	})
	high := NewCheckbox("high", Rect{W: 8, H: 1}, func(checked bool) {
		highCalls = append(highCalls, checked)
	})
	group := NewRadioGroup().Add(low).Add(high)

	high.toggle()
	if group.Selected() != 1 || !high.Checked || low.Checked {
		t.Fatalf("high toggle did not settle as exclusive selection")
	}
	if !reflect.DeepEqual(highCalls, []bool{true}) {
		t.Fatalf("high callback calls = %v, want [true]", highCalls)
	}

	high.toggle()
	if group.Selected() != 1 || !high.Checked {
		t.Fatalf("re-clicking selected radio should restore high as checked")
	}
	if !reflect.DeepEqual(highCalls, []bool{true, true}) {
		t.Fatalf("selected radio callback should see settled state, got %v", highCalls)
	}
	if len(lowCalls) != 0 {
		t.Fatalf("low callback should not fire when low is unchecked programmatically, got %v", lowCalls)
	}
}

func TestRadioGroupSelectionIsExclusive(t *testing.T) {
	low := NewCheckbox("low", Rect{W: 8, H: 1}, nil)
	normal := NewCheckbox("normal", Rect{W: 10, H: 1}, nil)
	high := NewCheckbox("high", Rect{W: 8, H: 1}, nil)
	group := NewRadioGroup().Add(low).Add(normal).Add(high)

	var changes []int
	group.OnChange = func(index int) {
		changes = append(changes, index)
	}

	normal.toggle()
	if group.Selected() != 1 || group.Value() != "normal" {
		t.Fatalf("selected = %d/%q, want 1/normal", group.Selected(), group.Value())
	}
	if low.Checked || !normal.Checked || high.Checked {
		t.Fatalf("radio selection was not exclusive: low=%v normal=%v high=%v", low.Checked, normal.Checked, high.Checked)
	}

	normal.toggle()
	if group.Selected() != 1 || !normal.Checked {
		t.Fatalf("re-clicking selected radio should keep it selected")
	}

	high.toggle()
	if group.Selected() != 2 || group.Value() != "high" {
		t.Fatalf("selected = %d/%q, want 2/high", group.Selected(), group.Value())
	}
	if low.Checked || normal.Checked || !high.Checked {
		t.Fatalf("radio selection did not move exclusively: low=%v normal=%v high=%v", low.Checked, normal.Checked, high.Checked)
	}

	if want := []int{1, 2}; !reflect.DeepEqual(changes, want) {
		t.Fatalf("OnChange calls = %v, want %v", changes, want)
	}

	group.SetSelected(-1)
	if group.Selected() != -1 || group.Value() != "" || low.Checked || normal.Checked || high.Checked {
		t.Fatalf("SetSelected(-1) should clear selection")
	}
}

func TestMultiSelectPreservesExistingCheckboxCallbacks(t *testing.T) {
	var callbackStates []bool
	api := NewCheckbox("API", Rect{W: 8, H: 1}, func(checked bool) {
		callbackStates = append(callbackStates, checked)
	})
	group := NewMultiSelect().Add(api)

	changes := 0
	group.OnChange = func() {
		changes++
	}

	api.toggle()
	api.toggle()

	if !reflect.DeepEqual(callbackStates, []bool{true, false}) {
		t.Fatalf("checkbox callback states = %v, want [true false]", callbackStates)
	}
	if changes != 2 {
		t.Fatalf("group OnChange calls = %d, want 2", changes)
	}
	if got := group.Selected(); len(got) != 0 {
		t.Fatalf("Selected after toggling off = %v, want empty", got)
	}
}

func TestMultiSelectTracksIndependentToggles(t *testing.T) {
	api := NewCheckbox("API", Rect{W: 8, H: 1}, nil)
	ui := NewCheckbox("UI", Rect{W: 8, H: 1}, nil)
	docs := NewCheckbox("Docs", Rect{W: 8, H: 1}, nil)
	group := NewMultiSelect().Add(api).Add(ui).Add(docs)

	changes := 0
	group.OnChange = func() {
		changes++
	}

	api.toggle()
	docs.toggle()
	if got, want := group.Selected(), []int{0, 2}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Selected = %v, want %v", got, want)
	}
	if got, want := group.SelectedValues(), []string{"API", "Docs"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("SelectedValues = %v, want %v", got, want)
	}

	api.toggle()
	if got, want := group.Selected(), []int{2}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Selected after untoggle = %v, want %v", got, want)
	}
	if changes != 3 {
		t.Fatalf("OnChange calls = %d, want 3", changes)
	}
}
