package tui

import "github.com/rivo/tview"

const hintText = "Tab: switch panel  Enter: play/open  z: queue  p: preview  Space: play/pause  s: stop  [/]: prev/next track  +/-: volume  d: players  /: search  ?: help  q: quit"

const helpText = `Lyrion TUI

Tab / Shift+Tab    switch panel
arrows / hjkl      move selection
Enter              drill in, or play the selection
Backspace / h      go up a level
z                  add the selection to the queue
p                  preview contents (genre/artist/album/playlist),
                   or (on Now Playing) preview the player's queue
Space              play/pause the active player - works no matter
                   which panel is focused
s                  stop the active player
[ / ]              previous / next track
+ / -              volume up/down
d                  jump to the player-select panel
/                  search the library
?                  toggle this help
q / Ctrl+C         quit`

func newHintBar() *tview.TextView {
	tv := tview.NewTextView().SetText(hintText).SetTextAlign(tview.AlignCenter)
	return tv
}
