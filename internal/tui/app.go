// Package tui is the tview-based front-end for lyrion-tui. It is kept
// separate from internal/lyrionapi (the LMS client) - this package only
// consumes that API's exported surface, never its internals.
package tui

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/medidew/lyrion-tui/internal/lyrionapi"
)

// App wires together the tview widgets and holds the single "active player"
// that playback actions target - see players.go for how it's selected.
type App struct {
	server *lyrionapi.LyrionServer
	tview  *tview.Application
	pages  *tview.Pages

	active        *lyrionapi.LyrionPlayer
	activeSummary lyrionapi.PlayerSummary

	players    *playersPanel
	library    *libraryPanel
	nowPlaying *nowPlayingPanel
	queueCache *queueCache
	local      *localPlayer
	hintBar    *tview.TextView

	focusables []tview.Primitive

	// helpReturnFocus is what had focus before the help overlay opened, so
	// closing it can put focus back.
	helpReturnFocus tview.Primitive
}

func NewApp(server *lyrionapi.LyrionServer, local LocalPlayerConfig) *App {
	app := &App{
		server: server,
		tview:  tview.NewApplication(),
	}
	app.local = &localPlayer{app: app, config: local}

	app.players = newPlayersPanel(app)
	app.library = newLibraryPanel(app)
	app.nowPlaying = newNowPlayingPanel(app)
	app.queueCache = newQueueCache(app)
	app.hintBar = newHintBar()

	app.pages = tview.NewPages().
		AddPage("library", app.library.root, true, true).
		AddPage("players", app.players.root, true, false)

	root := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(app.pages, 0, 1, true).
		AddItem(app.nowPlaying.root, 5, 0, false). // 3 lines + border
		AddItem(app.hintBar, 1, 0, false)

	app.focusables = []tview.Primitive{app.pages, app.nowPlaying.root}

	app.tview.SetInputCapture(app.globalInput)
	app.tview.SetRoot(root, true).SetFocus(app.pages)

	app.showPlayers()

	return app
}

// Run starts the event loop and blocks until the user quits.
func (app *App) Run() error {
	defer app.local.Close()
	defer app.nowPlaying.Close()
	defer app.queueCache.Close()
	return app.tview.Run()
}

func (app *App) globalInput(event *tcell.EventKey) *tcell.EventKey {
	// While a text field (currently just the library search box) has focus,
	// let it see every key - including letters that are otherwise global
	// shortcuts (s, d, q, space, +, -) - so the user can actually type them
	// into their search term. Only Ctrl+C remains global in that case; Tab/
	// Backtab/Enter/Escape are left to the field's own SetDoneFunc handler.
	if _, ok := app.tview.GetFocus().(*tview.InputField); ok {
		if event.Key() == tcell.KeyCtrlC {
			app.tview.Stop()
			return nil
		}
		return event
	}

	// While the help overlay is open it's modal: only '?' (close) and Ctrl+C
	// (quit) are handled here; everything else goes to the overlay, which
	// closes on Esc/Enter and swallows the rest.
	if app.pages.HasPage("help") {
		switch {
		case event.Key() == tcell.KeyCtrlC:
			app.tview.Stop()
			return nil
		case event.Key() == tcell.KeyRune && event.Rune() == '?':
			app.closeHelp()
			return nil
		}
		return event
	}

	switch {
	case event.Key() == tcell.KeyTab:
		app.cycleFocus(1)
		return nil
	case event.Key() == tcell.KeyBacktab:
		app.cycleFocus(-1)
		return nil
	case event.Key() == tcell.KeyCtrlC:
		app.tview.Stop()
		return nil
	case event.Key() == tcell.KeyRune && event.Rune() == 'q':
		app.tview.Stop()
		return nil
	case event.Key() == tcell.KeyRune && event.Rune() == 'd':
		app.showPlayers()
		return nil
	case event.Key() == tcell.KeyRune && event.Rune() == '?':
		app.toggleHelp()
		return nil
	case event.Key() == tcell.KeyRune && event.Rune() == ' ':
		app.togglePlayPause()
		return nil
	case event.Key() == tcell.KeyRune && event.Rune() == 's':
		app.stopActive()
		return nil
	case event.Key() == tcell.KeyRune && event.Rune() == '+':
		app.adjustVolume(5)
		return nil
	case event.Key() == tcell.KeyRune && event.Rune() == '-':
		app.adjustVolume(-5)
		return nil
	case event.Key() == tcell.KeyRune && event.Rune() == ']':
		app.nextTrack()
		return nil
	case event.Key() == tcell.KeyRune && event.Rune() == '[':
		app.previousTrack()
		return nil
	}
	return event
}

func (app *App) cycleFocus(direction int) {
	current := app.tview.GetFocus()
	index := 0
	for i, p := range app.focusables {
		if p == current {
			index = i
			break
		}
	}
	next := (index + direction + len(app.focusables)) % len(app.focusables)
	app.tview.SetFocus(app.focusables[next])
}

func (app *App) showPlayers() {
	app.players.Refresh()
	app.pages.SwitchToPage("players")
	app.tview.SetFocus(app.players.root)
}

// SetActivePlayer makes player the target of all playback actions and
// switches back to the library browser.
func (app *App) SetActivePlayer(player *lyrionapi.LyrionPlayer, summary lyrionapi.PlayerSummary) {
	app.active = player
	app.activeSummary = summary
	app.nowPlaying.SetActivePlayer(player, summary.ID, summary.Name)
	app.queueCache.SetActivePlayer(player, summary.ID)
	app.pages.SwitchToPage("library")
	app.tview.SetFocus(app.library.root)
}

// changeSong runs action, a command that changes which song the active
// player is playing, and passes its error to onDone (if set) on the UI
// goroutine. For the local player it goes through a stream restart (see
// localPlayer.changeSong), so the new song is heard straight away and from
// its start rather than after the audio already queued up.
func (app *App) changeSong(action func() error, onDone func(error)) {
	if app.local.isActive() {
		app.local.changeSong(action, onDone)
		return
	}
	err := action()
	if onDone != nil {
		onDone(err)
	}
}

func (app *App) nextTrack() {
	if app.active == nil {
		return
	}
	app.changeSong(app.active.Next, nil)
}

func (app *App) previousTrack() {
	if app.active == nil {
		return
	}
	app.changeSong(app.active.Previous, nil)
}

func (app *App) togglePlayPause() {
	if app.active == nil {
		return
	}
	mode, err := app.active.Mode()
	if err != nil {
		return
	}
	switch mode {
	case "play":
		app.active.Pause()
	case "stop":
		// Unpausing a stopped player does nothing. Starting playback is a
		// song change as far as the local stream is concerned: it's been
		// streaming silence.
		player := app.active
		app.changeSong(func() error { return player.Play(0) }, nil)
	default:
		app.active.Unpause(0)
	}
}

func (app *App) stopActive() {
	if app.active == nil {
		return
	}
	// A stop goes through changeSong too: otherwise the local player would
	// keep playing the audio LMS still has queued for it.
	app.changeSong(app.active.Stop, nil)
}

func (app *App) adjustVolume(delta int) {
	if app.active == nil {
		return
	}
	// LMS's mixer is a no-op for HTTP stream players, so the local player's
	// volume is applied locally instead.
	if app.local.isActive() {
		app.local.stream.SetVolume(app.local.stream.Volume() + delta)
		app.nowPlaying.render()
		return
	}
	volume, err := app.active.Volume()
	if err != nil {
		return
	}
	volume += delta
	if volume < 0 {
		volume = 0
	}
	if volume > 100 {
		volume = 100
	}
	app.active.SetVolume(volume)
}

func (app *App) toggleHelp() {
	if app.pages.HasPage("help") {
		app.closeHelp()
		return
	}

	app.helpReturnFocus = app.tview.GetFocus()
	overlay := newHelpOverlay(app.closeHelp)
	app.pages.AddPage("help", overlay, true, true)
	// Focus it explicitly: Pages only hands focus to a new page if Pages
	// itself had focus, which isn't the case when opened from Now Playing.
	app.tview.SetFocus(overlay)
}

func (app *App) closeHelp() {
	app.pages.RemovePage("help")
	if app.helpReturnFocus != nil {
		app.tview.SetFocus(app.helpReturnFocus)
		app.helpReturnFocus = nil
	}
}
