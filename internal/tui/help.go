package tui

import (
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

const hintText = "input '?' for keybindings"

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

// newHelpOverlay renders helpText as-is in a left-aligned, fixed-size box
// centered on screen. A tview.Modal (used previously) centers and can wrap
// each line individually, which destroys helpText's hand-aligned columns -
// a plain left-aligned TextView preserves them.
func newHelpOverlay(close func()) tview.Primitive {
	tv := tview.NewTextView().
		SetText(helpText).
		SetTextAlign(tview.AlignLeft).
		SetDynamicColors(true)
	tv.SetBorder(true).
		SetTitle(" Help (Esc / Enter / ? to close) ").
		SetBorderPadding(1, 1, 2, 2)
	tv.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		// '?' also closes it, but that's already handled a level up: the
		// global handler toggles the page before this capture ever sees it.
		if event.Key() == tcell.KeyEscape || event.Key() == tcell.KeyEnter {
			close()
		}
		return nil // swallow everything else while help is open
	})

	width, height := helpDimensions()

	return tview.NewFlex().
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().SetDirection(tview.FlexRow).
			AddItem(nil, 0, 1, false).
			AddItem(tv, height, 0, true).
			AddItem(nil, 0, 1, false), width, 0, true).
		AddItem(nil, 0, 1, false)
}

// helpDimensions sizes the help box to helpText's actual content, so it
// stays correct if the text changes rather than relying on hardcoded numbers.
func helpDimensions() (width, height int) {
	lines := strings.Split(helpText, "\n")
	height = len(lines) + 4 // border (2) + vertical padding (2)
	for _, line := range lines {
		if len(line) > width {
			width = len(line)
		}
	}
	width += 6 // border (2) + horizontal padding (4)
	return width, height
}
