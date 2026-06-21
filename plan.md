# Fix gogent #206 — sidebar tree highlight follows the active session

## Problem
The "Sessions & Agents" sidebar highlight is `tv.Tree.selected`, a **private**
index mutated only by the widget's own `handleType`/`handleClick`. turbotui
exposes `Root()`/`AddRoot()`/`Selected()` but **no setter**, so gogent cannot
move the bar when the active session changes by any path other than direct
input *inside* the tree (new session, `Ctrl+]` cycle, close, Session-menu pick).

## Fix (two repos)

### 1. turbotui (this repo) — add a public selection setter
File: `turbotv/widget_tree.go`. Add programmatic selection that mirrors how
`handleClick` sets `t.selected`, clamps it valid, and scrolls it into view, but
**does NOT fire `OnSelect`** — gogent's `OnSelect → wb.Focus → sidebar sync`
path would otherwise echo back and loop.

New public API on `*Tree`:
- `SetSelected(index int)` — set the highlight to a visible-row index (the same
  index space as `Selected()`); clamps into range and scrolls into view.
- `SelectNode(node *TreeNode) bool` — move the highlight to `node` matched by
  pointer identity among the currently visible rows; returns whether it was
  found. No-op (returns false) for nil / not-visible / collapsed-subtree nodes.

Neither fires `OnSelect`/`OnActivate`. Both reuse `clampInt` + `ensureVisible`.

### 2. gogent (partner's repo, after the turbotui bump) — wire it up
- `sidebar.selectSession(id string)` helper:
  `if n := s.sessions[id]; n != nil { s.tree.SelectNode(n) }` — no-op for
  unknown ids (sub-agent-only / removed sessions).
- Call it from every window→tree path:
  - `sidebar.addSession` — select the node just added (new session).
  - `Workbench.Focus` (raise / cycle / menu pick) — select the focused session.
  - `Workbench.CloseSession` — after switching the top window, select its node.
  `s.sessions` already maps `sessionID -> *tv.TreeNode`; `node.Data` is a
  `nodeRef{sessionID,...}`, so `SelectNode` by stored node pointer is exact.

## Tests (partner writes)
After a focus change (new / cycle / close / menu), the sidebar tree's selected
node points at the active session, and `OnSelect` is NOT re-fired by the
programmatic select (no echo loop).

## Constraints
No new deps; gofmt; golangci-lint 0; Go tests without `-race`.
