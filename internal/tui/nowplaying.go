package tui

import (
	"fmt"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/medidew/lyrion-tui/internal/lyrionapi"
)

// nowPlayingPanel shows the active player's status and keeps it live via a
// notification subscription (for song/mode/volume changes) plus a 1s ticker
// (for elapsed-time display, which LMS doesn't push). All mutation of the
// widget and of this struct's fields happens inside tview.QueueUpdateDraw
// closures, so it's always serialized onto the UI goroutine - the watch/tick
// goroutines below only compute values, never touch shared state directly.
type nowPlayingPanel struct {
	app  *App
	root *tview.TextView

	player   *lyrionapi.LyrionPlayer
	playerID string
	name     string
	status   lyrionapi.PlayerStatus
	elapsed  float64

	unsubscribe func()
	stopTicker  chan struct{}
}

func newNowPlayingPanel(app *App) *nowPlayingPanel {
	tv := tview.NewTextView().SetDynamicColors(true)
	tv.SetBorder(true).SetTitle(" Now Playing (Enter/p: preview queue) ")

	panel := &nowPlayingPanel{app: app, root: tv}
	tv.SetInputCapture(panel.input)
	panel.render()
	return panel
}

func (panel *nowPlayingPanel) input(event *tcell.EventKey) *tcell.EventKey {
	if event.Key() == tcell.KeyEnter || (event.Key() == tcell.KeyRune && event.Rune() == 'p') {
		panel.app.library.openQueue()
		return nil
	}
	return event
}

// SetActivePlayer switches which player this panel tracks, tearing down the
// previous subscription/ticker first.
func (panel *nowPlayingPanel) SetActivePlayer(player *lyrionapi.LyrionPlayer, playerID, name string) {
	panel.teardown()

	panel.player = player
	panel.playerID = playerID
	panel.name = name
	panel.elapsed = 0

	status, err := player.Status()
	if err != nil {
		panel.status = lyrionapi.PlayerStatus{}
	} else {
		panel.status = status
	}
	panel.render()

	if ch, unsubscribe, err := panel.app.server.Subscribe("playlist", "mixer", "power"); err == nil {
		panel.unsubscribe = unsubscribe
		go panel.watch(ch, player, playerID)
	}

	stop := make(chan struct{})
	panel.stopTicker = stop
	go panel.tick(player, playerID, stop)
}

// watch relays notifications for playerID into a fresh Status() snapshot.
// It never touches panel fields directly - only inside the QueueUpdateDraw
// closure, which runs serialized on the UI goroutine.
func (panel *nowPlayingPanel) watch(ch <-chan lyrionapi.Notification, player *lyrionapi.LyrionPlayer, playerID string) {
	for notification := range ch {
		if notification.PlayerID != playerID {
			continue
		}
		status, err := player.Status()
		panel.app.tview.QueueUpdateDraw(func() {
			if panel.playerID != playerID {
				return // player was switched while this fetch was in flight
			}
			if err == nil {
				panel.status = status
			}
			panel.render()
		})
	}
}

func (panel *nowPlayingPanel) tick(player *lyrionapi.LyrionPlayer, playerID string, stop <-chan struct{}) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			elapsed, err := player.Time()
			panel.app.tview.QueueUpdateDraw(func() {
				if panel.playerID != playerID {
					return
				}
				if err == nil {
					panel.elapsed = elapsed
				}
				panel.render()
			})
		}
	}
}

func (panel *nowPlayingPanel) teardown() {
	if panel.unsubscribe != nil {
		panel.unsubscribe()
		panel.unsubscribe = nil
	}
	if panel.stopTicker != nil {
		close(panel.stopTicker)
		panel.stopTicker = nil
	}
}

// Close releases the panel's subscription/ticker; called once on quit.
func (panel *nowPlayingPanel) Close() {
	panel.teardown()
}

func (panel *nowPlayingPanel) render() {
	if panel.player == nil {
		panel.root.SetText("[gray]No player selected - press 'd' to choose one[-]")
		return
	}

	song := panel.status.CurrentSong
	title := tview.Escape(song.Title)
	if title == "" {
		title = "(nothing playing)"
	}
	artist := tview.Escape(song.Artist)

	volumeBar := renderMeter(panel.status.Volume, 100, 20)
	progress := ""
	if song.Duration > 0 {
		progress = fmt.Sprintf("  %s / %s", formatDuration(panel.elapsed), formatDuration(song.Duration))
	}

	text := fmt.Sprintf(
		"[yellow]%s[-]  [gray](%s)[-]\n%s - %s%s\nvol %s %d%%",
		tview.Escape(panel.name), modeLabel(panel.status),
		title, artist, progress,
		tview.Escape(volumeBar), panel.status.Volume,
	)
	panel.root.SetText(text)
}

func modeLabel(status lyrionapi.PlayerStatus) string {
	if !status.Power {
		return "off"
	}
	return status.Mode
}

func renderMeter(value, max, width int) string {
	if max <= 0 {
		max = 1
	}
	filled := value * width / max
	if filled > width {
		filled = width
	}
	if filled < 0 {
		filled = 0
	}
	bar := ""
	for i := 0; i < width; i++ {
		if i < filled {
			bar += "="
		} else {
			bar += "-"
		}
	}
	return bar
}

func formatDuration(seconds float64) string {
	total := int(seconds)
	return fmt.Sprintf("%d:%02d", total/60, total%60)
}
