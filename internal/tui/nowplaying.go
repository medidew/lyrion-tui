package tui

import (
	"fmt"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/medidew/lyrion-tui/internal/localplayer"
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

	// Position tracking for the local player. LMS's elapsed time for it runs
	// well ahead of what's heard (it counts audio as played once sent, and
	// seconds of it are queued on the server's side), and LMS moves on to the
	// next song while the end of the last is still queued. So after a song
	// change made here (localRestarted) the song and position shown are the
	// ones actually being heard, counted from the audio played locally.
	// localStream is nil when not tracking; the server's view is shown then.
	localStream *localplayer.Player
	heardSong   lyrionapi.Song
	heardIndex  int           // heardSong's position in the queue
	heardStart  time.Duration // localStream.Played() when heardSong began
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
	panel.localStream = nil

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
				panel.applyStatus(status)
			}
			panel.render()
		})
	}
}

// refresh fetches a fresh status in the background.
func (panel *nowPlayingPanel) refresh() {
	player, playerID := panel.player, panel.playerID
	go func() {
		status, err := player.Status()
		panel.app.tview.QueueUpdateDraw(func() {
			if panel.playerID != playerID || err != nil {
				return
			}
			panel.applyStatus(status)
			panel.render()
		})
	}()
}

// localRestarted starts (or restarts) tracking the heard position after the
// local player's stream was restarted for a song change: the new song
// starts at the stream's Played() == 0.
func (panel *nowPlayingPanel) localRestarted(stream *localplayer.Player) {
	if panel.playerID != panel.app.local.config.ID {
		return
	}
	panel.localStream = stream
	panel.heardSong = lyrionapi.Song{}
	panel.heardStart = 0
	panel.refresh()
}

func (panel *nowPlayingPanel) applyStatus(status lyrionapi.PlayerStatus) {
	panel.status = status
	if panel.localStream == nil {
		return
	}

	current := status.CurrentSong
	remaining := panel.heardSong.Duration - panel.localElapsed()
	switch {
	case status.Mode == "stop":
		// At the end of the playlist LMS stops once it has sent the last
		// song, before its end is heard: keep tracking until it plays out
		// (advanceHeard). Any other stop (this app's goes through a restart,
		// which clears heardSong) ends tracking; the next play restarts it.
		if panel.heardSong.Path == "" || remaining > maxQueuedSeconds {
			panel.localStream = nil
		}
	case panel.heardSong.Path == "":
		panel.heardSong, panel.heardIndex = current, status.Index
	case current.Path != panel.heardSong.Path:
		// LMS has moved on. Doing so by itself happens while the end of the
		// heard song is still queued: keep showing that until it has played
		// out (advanceHeard). Anything else is a jump made from another
		// controller, which isn't heard promptly anyway - show LMS's view.
		if panel.heardSong.Duration <= 0 || remaining > maxQueuedSeconds {
			panel.localStream = nil
		}
	}
}

// maxQueuedSeconds bounds how far ahead of the speakers LMS can plausibly
// be, for telling a natural song change from a jump (see applyStatus).
const maxQueuedSeconds = 60

// advanceHeard moves on to the next song once the heard one has played out
// locally.
func (panel *nowPlayingPanel) advanceHeard() {
	if panel.localStream == nil || panel.heardSong.Duration <= 0 || panel.localElapsed() < panel.heardSong.Duration {
		return
	}
	song, index, ok := panel.nextHeard()
	if !ok || panel.status.Mode == "stop" && index <= panel.heardIndex {
		panel.localStream = nil // the end of the playlist has been heard
		return
	}
	panel.heardStart += time.Duration(panel.heardSong.Duration * float64(time.Second))
	panel.heardSong, panel.heardIndex = song, index
}

// nextHeard picks the song after the heard one from the cached queue rather
// than trusting LMS's current song: for an HTTP player LMS misreports that
// once it starts sending the playlist's last track (index 0 and the first
// track's title, while the last track is what's playing).
func (panel *nowPlayingPanel) nextHeard() (lyrionapi.Song, int, bool) {
	tracks := panel.app.queueCache.Tracks()
	next := panel.heardIndex + 1
	switch panel.status.Repeat {
	case 1: // repeat song
		next = panel.heardIndex
	case 2: // repeat playlist
		if next >= len(tracks) {
			next = 0
		}
	}
	if next < len(tracks) {
		return tracks[next], next, true
	}
	if len(tracks) > 0 {
		return lyrionapi.Song{}, 0, false // past the end of the playlist
	}
	// The queue isn't cached (yet): fall back to LMS's view.
	if current := panel.status.CurrentSong; current.Path != panel.heardSong.Path {
		return current, panel.status.Index, true
	}
	return lyrionapi.Song{}, 0, false
}

func (panel *nowPlayingPanel) localElapsed() float64 {
	return max(0, (panel.localStream.Played() - panel.heardStart).Seconds())
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
				panel.advanceHeard()
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

	song, elapsed, mode := panel.status.CurrentSong, panel.elapsed, modeLabel(panel.status)
	if panel.localStream != nil && panel.heardSong.Path != "" {
		song, elapsed = panel.heardSong, panel.localElapsed()
		if panel.status.Mode == "stop" {
			mode = "play" // the end of the playlist is still being heard
		}
	}
	title := tview.Escape(song.Title)
	if title == "" {
		title = "(nothing playing)"
	}
	artist := tview.Escape(song.Artist)

	volume := panel.status.Volume
	if panel.app.local.isActive() && panel.playerID == panel.app.local.config.ID {
		volume = panel.app.local.stream.Volume()
	}
	volumeBar := renderMeter(volume, 100, 20)
	progress := ""
	if song.Duration > 0 {
		progress = fmt.Sprintf("  %s / %s", formatDuration(min(elapsed, song.Duration)), formatDuration(song.Duration))
	}

	text := fmt.Sprintf(
		"[yellow]%s[-]  [gray](%s)[-]\n%s - %s%s\nvol %s %d%%",
		tview.Escape(panel.name), mode,
		title, artist, progress,
		tview.Escape(volumeBar), volume,
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
