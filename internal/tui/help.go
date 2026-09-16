package tui

import "github.com/rivo/tview"

const hintText = "Tab: switch panel  Enter: play/open  z: queue  Space: play/pause  s: stop  +/-: volume  d: players  /: search  ?: help  q: quit"

const helpText = `Lyrion TUI

Tab / Shift+Tab    switch panel
arrows / hjkl      move selection
Enter              drill in, or play the selection
Backspace / h      go up a level
z                  add the selection to the queue
Space              play/pause the active player
s                  stop the active player
+ / -              volume up/down
d                  jump to the player-select panel
/                  search the library
?                  toggle this help
q / Ctrl+C         quit`

func newHintBar() *tview.TextView {
	tv := tview.NewTextView().SetText(hintText).SetTextAlign(tview.AlignCenter)
	return tv
}
