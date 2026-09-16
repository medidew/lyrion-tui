package tui

import (
	"sync"

	"github.com/medidew/lyrion-tui/internal/lyrionapi"
)

// queueCache mirrors the active player's live playlist locally, refreshed
// in the background whenever LMS reports the playlist changed (rather than
// on every preview open), so the queue preview (Now Playing -> Enter/p) is
// instant even for long playlists instead of doing dozens of round trips
// (one TrackAt call per song) on demand.
type queueCache struct {
	app *App

	mu       sync.Mutex
	tracks   []lyrionapi.Song
	player   *lyrionapi.LyrionPlayer
	playerID string

	unsubscribe func()

	refreshMu      sync.Mutex
	refreshing     bool
	refreshPending bool
}

func newQueueCache(app *App) *queueCache {
	return &queueCache{app: app}
}

// SetActivePlayer switches which player's queue is cached: tears down the
// previous subscription, subscribes to this player's playlist-changing
// notifications, and kicks off a fresh (background) fetch.
func (c *queueCache) SetActivePlayer(player *lyrionapi.LyrionPlayer, playerID string) {
	c.teardown()

	c.mu.Lock()
	c.player = player
	c.playerID = playerID
	c.tracks = nil
	c.mu.Unlock()

	// "playlist" covers add/insert/delete/move/clear/newsong/shuffle/repeat
	// etc. (all issued as "<playerid> playlist <subverb> ..."); "playlistcontrol"
	// is issued as its own top-level command by PlaylistControl.
	if ch, unsubscribe, err := c.app.server.Subscribe("playlist", "playlistcontrol"); err == nil {
		c.unsubscribe = unsubscribe
		go c.watch(ch, playerID)
	}

	c.refresh(player, playerID)
}

func (c *queueCache) watch(ch <-chan lyrionapi.Notification, playerID string) {
	for notification := range ch {
		if notification.PlayerID != playerID {
			continue
		}

		c.mu.Lock()
		player, current := c.player, c.playerID
		c.mu.Unlock()
		if player == nil || current != playerID {
			continue
		}

		c.refresh(player, playerID)
	}
}

// refresh re-fetches the queue in the background, coalescing overlapping
// requests: a burst of notifications (e.g. queuing an entire album fires
// one notification per track) triggers at most one extra refresh after the
// in-flight one completes, rather than one per notification.
func (c *queueCache) refresh(player *lyrionapi.LyrionPlayer, playerID string) {
	c.refreshMu.Lock()
	if c.refreshing {
		c.refreshPending = true
		c.refreshMu.Unlock()
		return
	}
	c.refreshing = true
	c.refreshMu.Unlock()

	go func() {
		for {
			tracks := fetchQueue(player)

			c.mu.Lock()
			stillActive := c.playerID == playerID
			if stillActive {
				c.tracks = tracks
			}
			c.mu.Unlock()

			if stillActive {
				c.app.tview.QueueUpdateDraw(func() {
					c.app.library.refreshQueueViewIfOpen()
				})
			}

			c.refreshMu.Lock()
			if c.refreshPending {
				c.refreshPending = false
				c.refreshMu.Unlock()
				continue
			}
			c.refreshing = false
			c.refreshMu.Unlock()
			return
		}
	}()
}

func fetchQueue(player *lyrionapi.LyrionPlayer) []lyrionapi.Song {
	count, err := player.TrackCount()
	if err != nil {
		return nil
	}

	tracks := make([]lyrionapi.Song, 0, count)
	for i := 0; i < count; i++ {
		track, err := player.TrackAt(i)
		if err != nil {
			continue
		}
		tracks = append(tracks, track)
	}
	return tracks
}

// Tracks returns a snapshot of the cached queue.
func (c *queueCache) Tracks() []lyrionapi.Song {
	c.mu.Lock()
	defer c.mu.Unlock()
	tracks := make([]lyrionapi.Song, len(c.tracks))
	copy(tracks, c.tracks)
	return tracks
}

func (c *queueCache) teardown() {
	if c.unsubscribe != nil {
		c.unsubscribe()
		c.unsubscribe = nil
	}
}

// Close releases the cache's subscription; called once on quit.
func (c *queueCache) Close() {
	c.teardown()
}
