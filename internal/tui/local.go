package tui

import (
	"errors"
	"sync"
	"time"

	"github.com/medidew/lyrion-tui/internal/localplayer"
	"github.com/medidew/lyrion-tui/internal/lyrionapi"
)

// LocalPlayerConfig describes the local player: this machine playing the
// server's HTTP audio stream, which makes it an LMS player like any other.
type LocalPlayerConfig struct {
	ID        string // the player ID it registers under (localplayer.ID)
	Name      string // what to name it on the server
	StreamURL string // localplayer.StreamURL for that ID
}

// localPlayer owns the local audio stream. The stream only starts the first
// time the Local row is selected; after that it keeps playing (even while
// another player is active) until the app quits. Its fields are only touched
// on the UI goroutine.
type localPlayer struct {
	app         *App
	config      LocalPlayerConfig
	stream      *localplayer.Player // nil until started
	starting    bool
	unsubscribe func()

	// changeMu keeps song changes (which run in the background) in order.
	changeMu sync.Mutex
}

// isActive reports whether the local player is the app's active player.
func (local *localPlayer) isActive() bool {
	return local.stream != nil && local.app.active != nil && local.app.activeSummary.ID == local.config.ID
}

// activate makes the local player the active one, starting its stream first
// if needed. Starting runs in the background (it waits on the network), so
// this returns immediately; errors are shown on the players panel.
func (local *localPlayer) activate() {
	if local.stream != nil {
		local.app.SetActivePlayer(local.app.server.PlayerByID(local.config.ID), local.summary())
		return
	}
	if local.starting {
		return
	}
	local.starting = true
	local.app.players.showStatus("[yellow]starting local player...[-]")

	go func() {
		stream, err := local.start()
		local.app.tview.QueueUpdateDraw(func() {
			local.starting = false
			if err != nil {
				local.app.players.showError(err)
				return
			}
			local.stream = stream
			if ch, unsubscribe, err := local.app.server.Subscribe("playlist", "play", "pause", "stop", "mode"); err == nil {
				local.unsubscribe = unsubscribe
				go local.followMode(ch, stream)
			}
			local.app.players.showStatus("")
			local.app.SetActivePlayer(local.app.server.PlayerByID(local.config.ID), local.summary())
		})
	}()
}

// start connects the stream, waits for the server to register it as a
// player, and names it. Called off the UI goroutine.
func (local *localPlayer) start() (*localplayer.Player, error) {
	stream, err := localplayer.Start(local.config.StreamURL)
	if err != nil {
		return nil, err
	}

	if !local.waitForRegistration() {
		stream.Close()
		return nil, errors.New("local player didn't register with the server")
	}

	player := local.app.server.PlayerByID(local.config.ID)
	player.SetName(local.config.Name)
	// LMS starts an HTTP player's existing playlist as soon as it connects;
	// selecting Local shouldn't suddenly start music from a past session.
	// Stopping inside a restart also discards what LMS already sent of it.
	var stopErr error
	if err := stream.Restart(func() {
		waitForDisconnect(player)
		stopErr = player.Stop()
	}); err != nil || stopErr != nil {
		stream.Close()
		return nil, errors.Join(err, stopErr)
	}
	return stream, nil
}

func (local *localPlayer) waitForRegistration() bool {
	deadline := time.Now().Add(lyrionapi.ConnectTimeout)
	for time.Now().Before(deadline) {
		players, err := local.app.server.Players(0, 200)
		if err == nil {
			for _, summary := range players {
				if summary.ID == local.config.ID && summary.Connected {
					return true
				}
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	return false
}

// followMode mirrors the server's play/pause state onto the local output,
// whoever changed it (this app or another controller). The server stops
// sending when paused, but without this the audio already buffered locally
// would play on for a moment - and pausing only the local output from this
// app would leave it silent if playback were resumed from somewhere else.
func (local *localPlayer) followMode(ch <-chan lyrionapi.Notification, stream *localplayer.Player) {
	player := local.app.server.PlayerByID(local.config.ID)
	lastMode := ""
	for notification := range ch {
		if notification.PlayerID != local.config.ID {
			continue
		}
		mode, err := player.Mode()
		if err != nil || mode == lastMode {
			continue
		}
		// A stop straight after a pause would otherwise leave the paused
		// track's buffered tail to play when the output next resumes. (A stop
		// after playing isn't flushed: at the end of a playlist the server
		// stops while the last second is still buffered here.)
		if mode == "stop" && lastMode == "pause" {
			stream.Flush()
		}
		stream.SetPaused(mode == "pause")
		lastMode = mode
	}
}

// summary is the PlayerSummary the rest of the TUI sees for the local player.
func (local *localPlayer) summary() lyrionapi.PlayerSummary {
	return lyrionapi.PlayerSummary{
		ID:        local.config.ID,
		Name:      local.config.Name,
		Model:     "http",
		ModelName: "this machine",
		Power:     true,
		Connected: local.stream != nil,
	}
}

// changeSong runs action - a command that changes the song - inside a stream
// restart: the connection is dropped (discarding the seconds of old audio
// LMS keeps queued on its side), action runs once LMS has noticed, and the
// stream reconnects, so the new song is heard straight away and from its
// start. It runs in the background (it waits on the network); onDone gets
// the result on the UI goroutine.
func (local *localPlayer) changeSong(action func() error, onDone func(error)) {
	stream := local.stream
	player := local.app.server.PlayerByID(local.config.ID)
	go func() {
		local.changeMu.Lock()
		defer local.changeMu.Unlock()

		var actionErr error
		err := stream.Restart(func() {
			waitForDisconnect(player)
			actionErr = action()
		})
		if actionErr != nil {
			err = actionErr
		}
		local.app.tview.QueueUpdateDraw(func() {
			local.app.nowPlaying.localRestarted(stream)
			if onDone != nil {
				onDone(err)
			}
		})
	}()
}

// waitForDisconnect waits (briefly) until LMS reports player's stream
// connection as closed. Until then LMS may still send audio into it, and
// whatever it sends there is lost.
func waitForDisconnect(player *lyrionapi.LyrionPlayer) {
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if connected, err := player.Connected(); err == nil && !connected {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func (local *localPlayer) Close() {
	if local.unsubscribe != nil {
		local.unsubscribe()
	}
	if local.stream != nil {
		local.stream.Close()
	}
}
