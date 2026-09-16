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
)

// libraryItem is one row in the browse list. id holds whatever the LMS API
// needs to act on the item: a genre/artist/album/track ID, or (for
// playlists) the playlist's URL. name only matters for playlists, where
// PlaySong/AddSong also want a display title.
type libraryItem struct {
	kind  itemKind
	title string
	desc  string
	id    string
	name  string
}

// libraryFrame is one level of the browse hierarchy: the items shown, and
// the genre/artist filter context inherited from however we got here (used
// when drilling further, and when queuing/playing an aggregate selection).
type libraryFrame struct {
	title    string
	items    []libraryItem
	genreID  string
	artistID string
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

	var err error
	switch item.kind {
	case kindPlaylist:
		if playNow {
			err = player.PlaySong(item.id, item.name, 0)
		} else {
			err = player.AddSong(item.id, item.name)
		}
	case kindTrack:
		err = applyFilter(player, playNow, lyrionapi.PlaylistControlOpts{TrackID: item.id})
	case kindAlbum:
		err = applyFilter(player, playNow, lyrionapi.PlaylistControlOpts{AlbumID: item.id})
	case kindArtist:
		err = applyFilter(player, playNow, lyrionapi.PlaylistControlOpts{ArtistID: item.id})
	case kindGenre:
		err = applyFilter(player, playNow, lyrionapi.PlaylistControlOpts{GenreID: item.id})
	default:
		return
	}

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
		items[i] = libraryItem{kind: kindPlaylist, title: p.Name, id: p.URL, name: p.Name}
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
