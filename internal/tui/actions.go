package tui

import "github.com/medidew/lyrion-tui/internal/lyrionapi"

// applyFilter runs filter as a play-now (replace playlist) or queue (append)
// action, depending on playNow. filter carries whichever id field (genre,
// artist, album or track) identifies the selection; PlaylistControl accepts
// any of them on their own.
func applyFilter(player *lyrionapi.LyrionPlayer, playNow bool, filter lyrionapi.PlaylistControlOpts) error {
	if playNow {
		filter.Cmd = "load"
	} else {
		filter.Cmd = "add"
	}
	return player.PlaylistControl(filter)
}
