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
	hintBar    *tview.TextView

	focusables []tview.Primitive
}

func NewApp(server *lyrionapi.LyrionServer) *App {
	app := &App{
		server: server,
		tview:  tview.NewApplication(),
	}

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
		AddItem(app.nowPlaying.root, 4, 0, false).
		AddItem(app.hintBar, 1, 0, false)

	app.focusables = []tview.Primitive{app.pages, app.nowPlaying.root}

	app.tview.SetInputCapture(app.globalInput)
	app.tview.SetRoot(root, true).SetFocus(app.pages)

	app.showPlayers()

	return app
}

// Run starts the event loop and blocks until the user quits.
func (app *App) Run() error {
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

func (app *App) nextTrack() {
	if app.active == nil {
		return
	}
	app.active.Next()
}

func (app *App) previousTrack() {
	if app.active == nil {
		return
	}
	app.active.Previous()
}

func (app *App) togglePlayPause() {
	if app.active == nil {
		return
	}
	mode, err := app.active.Mode()
	if err != nil {
		return
	}
	if mode == "play" {
		app.active.Pause()
	} else {
		app.active.Unpause(0)
	}
}

func (app *App) stopActive() {
	if app.active == nil {
		return
	}
	app.active.Stop()
}

func (app *App) adjustVolume(delta int) {
	if app.active == nil {
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
		app.pages.RemovePage("help")
		return
	}

	overlay := newHelpOverlay(func() {
		app.pages.RemovePage("help")
	})
	app.pages.AddPage("help", overlay, true, true)
}
