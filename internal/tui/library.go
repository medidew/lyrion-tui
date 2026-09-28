package tui

import (
	"fmt"
	"strconv"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/medidew/lyrion-tui/internal/lyrionapi"
)

type itemKind int

const (
	kindMenu itemKind = iota
	kindGenre
	kindArtist
	kindAlbum
	kindTrack
	kindPlaylist
	// kindQueueItem is a row in the active player's live queue (see
	// libraryPanel.openQueue), addressed by playlist position rather than a
	// database ID.
	kindQueueItem
)

// libraryItem is one row in the browse list. id holds whatever the LMS API
// needs to act on the item: a genre/artist/album/track ID, a playlist
// position (for kindQueueItem), or (for playlists) the playlist's URL. name
// only matters for playlists, where PlaySong/AddSong also want a display
// title. playlistID additionally holds a playlist's own ID (as opposed to
// its URL, held in id), needed to look up its track listing.
type libraryItem struct {
	kind       itemKind
	title      string
	desc       string
	id         string
	name       string
	playlistID string
}

// libraryFrame is one level of the browse hierarchy: the items shown, and
// the genre/artist filter context inherited from however we got here (used
// when drilling further, and when queuing/playing an aggregate selection).
type libraryFrame struct {
	title    string
	items    []libraryItem
	genreID  string
	artistID string
	// isQueue marks this frame as the live queue view (see openQueue), so a
	// background cache refresh knows whether to re-render it in place.
	isQueue bool
}

// libraryPanel is the drill-down browse screen: Genres -> Artists -> Albums,
// a Playlists tab, and Search - see runOrDrill for exactly what each kind of
// row does on Enter vs. 'z'.
type libraryPanel struct {
	app   *App
	root  *tview.Flex
	title *tview.TextView
	list  *tview.List

	stack []libraryFrame
}

func newLibraryPanel(app *App) *libraryPanel {
	panel := &libraryPanel{app: app}

	panel.title = tview.NewTextView().SetDynamicColors(true)
	panel.list = tview.NewList().ShowSecondaryText(true)
	panel.list.SetBorder(true).SetTitle(" Library ")
	panel.list.SetInputCapture(panel.listInput)

	panel.root = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(panel.title, 1, 0, false).
		AddItem(panel.list, 0, 1, true)

	panel.openRootMenu()

	return panel
}

func (panel *libraryPanel) listInput(event *tcell.EventKey) *tcell.EventKey {
	if event.Key() != tcell.KeyRune {
		if event.Key() == tcell.KeyBackspace || event.Key() == tcell.KeyBackspace2 {
			panel.popFrame()
			return nil
		}
		return event
	}

	switch event.Rune() {
	case 'j':
		panel.moveSelection(1)
	case 'k':
		panel.moveSelection(-1)
	case 'h':
		panel.popFrame()
	case 'l':
		panel.activateCurrent()
	case 'z':
		panel.actOnCurrent(false)
	case 'p':
		panel.previewCurrent()
	case '/':
		panel.openSearch()
	default:
		return event
	}
	return nil
}

func (panel *libraryPanel) moveSelection(delta int) {
	count := panel.list.GetItemCount()
	if count == 0 {
		return
	}
	next := panel.list.GetCurrentItem() + delta
	if next < 0 {
		next = 0
	}
	if next >= count {
		next = count - 1
	}
	panel.list.SetCurrentItem(next)
}

func (panel *libraryPanel) activateCurrent() {
	panel.actOnCurrent(true)
}

func (panel *libraryPanel) actOnCurrent(playNow bool) {
	frame := panel.current()
	index := panel.list.GetCurrentItem()
	if index < 0 || index >= len(frame.items) {
		return
	}
	panel.runOrDrill(frame.items[index], playNow)
}

func (panel *libraryPanel) current() libraryFrame {
	return panel.stack[len(panel.stack)-1]
}

func (panel *libraryPanel) pushFrame(frame libraryFrame) {
	panel.stack = append(panel.stack, frame)
	panel.render()
}

func (panel *libraryPanel) popFrame() {
	if len(panel.stack) <= 1 {
		return
	}
	panel.stack = panel.stack[:len(panel.stack)-1]
	panel.render()
}

func (panel *libraryPanel) render() {
	frame := panel.current()
	panel.list.Clear()
	panel.title.SetText("[::b]" + tview.Escape(frame.title) + "[::-]")

	showDesc := false
	for _, item := range frame.items {
		if item.desc != "" {
			showDesc = true
			break
		}
	}
	panel.list.ShowSecondaryText(showDesc)

	for _, item := range frame.items {
		item := item // capture for the closure below
		panel.list.AddItem(item.title, item.desc, 0, func() {
			panel.runOrDrill(item, true)
		})
	}
}

func (panel *libraryPanel) showError(err error) {
	panel.title.SetText("[red::b]error: " + tview.Escape(err.Error()) + "[::-]")
}

func (panel *libraryPanel) showStatus(text string) {
	panel.title.SetText(text)
}

// runOrDrill is what both Enter and 'z' funnel through. Genres/artists drill
// into the next level on Enter (playNow=true); everything else - albums,
// tracks (search-only, since browsing doesn't go deeper than albums),
// playlists, and genres/artists too when triggered via 'z' - is a play-now
// or queue action against the active player.
func (panel *libraryPanel) runOrDrill(item libraryItem, playNow bool) {
	frame := panel.current()

	if playNow {
		switch item.kind {
		case kindMenu:
			panel.openMenu(item.name)
			return
		case kindGenre:
			panel.openArtists(item.id)
			return
		case kindArtist:
			panel.openAlbums(frame.genreID, item.id)
			return
		}
	}

	panel.performAction(item, playNow)
}

func (panel *libraryPanel) performAction(item libraryItem, playNow bool) {
	player := panel.app.active
	if player == nil {
		panel.showStatus("[red]select a player first - press d[-]")
		return
	}

	var action func() error
	switch item.kind {
	case kindPlaylist:
		action = func() error {
			if playNow {
				return player.PlaySong(item.id, item.name, 0)
			}
			return player.AddSong(item.id, item.name)
		}
	case kindTrack:
		action = func() error { return applyFilter(player, playNow, lyrionapi.PlaylistControlOpts{TrackID: item.id}) }
	case kindAlbum:
		action = func() error { return applyFilter(player, playNow, lyrionapi.PlaylistControlOpts{AlbumID: item.id}) }
	case kindArtist:
		action = func() error { return applyFilter(player, playNow, lyrionapi.PlaylistControlOpts{ArtistID: item.id}) }
	case kindGenre:
		action = func() error { return applyFilter(player, playNow, lyrionapi.PlaylistControlOpts{GenreID: item.id}) }
	case kindQueueItem:
		if !playNow {
			return // already in the queue - nothing to do for 'z'
		}
		index, err := strconv.Atoi(item.id)
		if err != nil {
			return
		}
		action = func() error { return player.SetIndex(index) }
	default:
		return
	}

	done := func(err error) {
		if err != nil {
			panel.showError(err)
			return
		}
		verb := "queued"
		if playNow {
			verb = "playing"
		}
		panel.showStatus(fmt.Sprintf("[green]%s: %s[-]", verb, tview.Escape(item.title)))
	}

	if playNow {
		panel.app.changeSong(action, done)
	} else {
		done(action())
	}
}

// previewCurrent shows the track listing a genre/artist/album/playlist
// would play or queue, without acting on it - browsing it is Backspace/'h'
// away, same as any other frame.
func (panel *libraryPanel) previewCurrent() {
	frame := panel.current()
	index := panel.list.GetCurrentItem()
	if index < 0 || index >= len(frame.items) {
		return
	}
	item := frame.items[index]

	var (
		tracks []lyrionapi.Track
		err    error
	)
	switch item.kind {
	case kindPlaylist:
		tracks, err = panel.app.server.GetPlaylistTracks(item.playlistID, 0, 300)
	case kindGenre:
		tracks, err = panel.app.server.GetTitles(0, 300, lyrionapi.TrackQueryOpts{GenreID: item.id})
	case kindArtist:
		tracks, err = panel.app.server.GetTitles(0, 300, lyrionapi.TrackQueryOpts{GenreID: frame.genreID, ArtistID: item.id})
	case kindAlbum:
		tracks, err = panel.app.server.GetTitles(0, 300, lyrionapi.TrackQueryOpts{AlbumID: item.id})
	default:
		return // nothing to preview for a track or queue entry
	}
	if err != nil {
		panel.showError(err)
		return
	}

	items := make([]libraryItem, len(tracks))
	for i, t := range tracks {
		items[i] = libraryItem{kind: kindTrack, title: t.Title, desc: t.Artist, id: strconv.Itoa(t.ID)}
	}
	panel.pushFrame(libraryFrame{title: "Preview: " + item.title, items: items})
}

// openQueue shows the active player's current playlist, so it can be
// browsed (and jumped into via Enter) without disturbing playback. Reachable
// by focusing the Now Playing panel (Tab) and pressing Enter or 'p'. Reads
// from app.queueCache (kept fresh in the background by notifications)
// instead of fetching live, so this is instant even for long playlists.
func (panel *libraryPanel) openQueue() {
	if panel.app.active == nil {
		return
	}

	panel.pushFrame(libraryFrame{title: "Queue", items: queueItems(panel.app.queueCache.Tracks()), isQueue: true})
	panel.app.pages.SwitchToPage("library")
	panel.app.tview.SetFocus(panel.list)
}

// refreshQueueViewIfOpen re-renders the queue view in place from the cache,
// if it's the frame currently being shown, preserving the selected row
// where possible. Called after every background cache refresh.
func (panel *libraryPanel) refreshQueueViewIfOpen() {
	if len(panel.stack) == 0 || !panel.current().isQueue {
		return
	}

	selected := panel.list.GetCurrentItem()
	panel.stack[len(panel.stack)-1].items = queueItems(panel.app.queueCache.Tracks())
	panel.render()

	if count := panel.list.GetItemCount(); count > 0 {
		if selected >= count {
			selected = count - 1
		}
		if selected < 0 {
			selected = 0
		}
		panel.list.SetCurrentItem(selected)
	}
}

func queueItems(tracks []lyrionapi.Song) []libraryItem {
	items := make([]libraryItem, len(tracks))
	for i, track := range tracks {
		title := track.Title
		if title == "" {
			title = fmt.Sprintf("Track %d", i+1)
		}
		items[i] = libraryItem{kind: kindQueueItem, title: title, desc: track.Artist, id: strconv.Itoa(i)}
	}
	return items
}

func (panel *libraryPanel) openRootMenu() {
	panel.stack = nil
	panel.pushFrame(libraryFrame{
		title: "Library",
		items: []libraryItem{
			{kind: kindMenu, title: "Genres", desc: "browse by genre", name: "genres"},
			{kind: kindMenu, title: "Artists", desc: "browse by artist", name: "artists"},
			{kind: kindMenu, title: "Albums", desc: "browse by album", name: "albums"},
			{kind: kindMenu, title: "Playlists", desc: "browse saved playlists", name: "playlists"},
		},
	})
}

func (panel *libraryPanel) openMenu(name string) {
	switch name {
	case "genres":
		panel.openGenres()
	case "artists":
		panel.openArtists("")
	case "albums":
		panel.openAlbums("", "")
	case "playlists":
		panel.openPlaylists()
	}
}

func (panel *libraryPanel) openGenres() {
	genres, err := panel.app.server.GetGenres(0, 300, lyrionapi.GenreQueryOpts{})
	if err != nil {
		panel.showError(err)
		return
	}
	items := make([]libraryItem, len(genres))
	for i, g := range genres {
		items[i] = libraryItem{kind: kindGenre, title: g.Name, id: strconv.Itoa(g.ID)}
	}
	panel.pushFrame(libraryFrame{title: "Genres", items: items})
}

func (panel *libraryPanel) openArtists(genreID string) {
	artists, err := panel.app.server.GetArtists(0, 500, lyrionapi.ArtistQueryOpts{GenreID: genreID})
	if err != nil {
		panel.showError(err)
		return
	}
	items := make([]libraryItem, len(artists))
	for i, a := range artists {
		items[i] = libraryItem{kind: kindArtist, title: a.Name, id: strconv.Itoa(a.ID)}
	}
	panel.pushFrame(libraryFrame{title: "Artists", items: items, genreID: genreID})
}

func (panel *libraryPanel) openAlbums(genreID, artistID string) {
	albums, err := panel.app.server.GetAlbums(0, 500, lyrionapi.AlbumQueryOpts{GenreID: genreID, ArtistID: artistID})
	if err != nil {
		panel.showError(err)
		return
	}
	items := make([]libraryItem, len(albums))
	for i, a := range albums {
		desc := a.ArtistName
		if a.Year > 0 {
			desc = fmt.Sprintf("%s (%d)", desc, a.Year)
		}
		items[i] = libraryItem{kind: kindAlbum, title: a.Title, desc: desc, id: strconv.Itoa(a.ID)}
	}
	panel.pushFrame(libraryFrame{title: "Albums", items: items, genreID: genreID, artistID: artistID})
}

func (panel *libraryPanel) openPlaylists() {
	playlists, err := panel.app.server.GetPlaylists(0, 300, lyrionapi.PlaylistQueryOpts{})
	if err != nil {
		panel.showError(err)
		return
	}
	items := make([]libraryItem, len(playlists))
	for i, p := range playlists {
		items[i] = libraryItem{kind: kindPlaylist, title: p.Name, id: p.URL, name: p.Name, playlistID: p.ID}
	}
	panel.pushFrame(libraryFrame{title: "Playlists", items: items})
}

func (panel *libraryPanel) openSearch() {
	input := tview.NewInputField().SetLabel("Search: ")
	input.SetDoneFunc(func(key tcell.Key) {
		term := input.GetText()
		panel.root.RemoveItem(input)
		panel.app.tview.SetFocus(panel.list)
		if key == tcell.KeyEnter && term != "" {
			panel.runSearch(term)
		}
	})

	panel.root.AddItem(input, 1, 0, true)
	panel.app.tview.SetFocus(input)
}

func (panel *libraryPanel) runSearch(term string) {
	results, err := panel.app.server.Search(term, 0, 30)
	if err != nil {
		panel.showError(err)
		return
	}

	var items []libraryItem
	for _, a := range results.Artists {
		items = append(items, libraryItem{kind: kindArtist, title: a.Name, desc: "artist", id: strconv.Itoa(a.ID)})
	}
	for _, a := range results.Albums {
		items = append(items, libraryItem{kind: kindAlbum, title: a.Title, desc: "album", id: strconv.Itoa(a.ID)})
	}
	for _, g := range results.Genres {
		items = append(items, libraryItem{kind: kindGenre, title: g.Name, desc: "genre", id: strconv.Itoa(g.ID)})
	}
	for _, t := range results.Tracks {
		items = append(items, libraryItem{kind: kindTrack, title: t.Title, desc: "song", id: strconv.Itoa(t.ID)})
	}

	panel.pushFrame(libraryFrame{title: fmt.Sprintf("Search results for %q", term), items: items})
}
