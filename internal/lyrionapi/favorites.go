package lyrionapi

import (
	"fmt"
	"strings"
)

type Favorite struct {
	ID       string
	Name     string
	HasItems bool
	URL      string
}

func favoriteFromRecord(record map[string]string) Favorite {
	return Favorite{
		ID:       tagString(record, "id"),
		Name:     tagString(record, "name"),
		HasItems: tagBool(record, "hasitems"),
		URL:      tagString(record, "url"),
	}
}

type FavoriteItemsOpts struct {
	ItemID string // dotted-hierarchy ID; empty means the root
	Search string
}

// FavoriteItems lists favorites (and favorite folders) under opts.ItemID.
func (server *LyrionServer) FavoriteItems(from, to int, opts FavoriteItemsOpts) ([]Favorite, error) {
	var args []string
	if opts.ItemID != "" {
		args = append(args, "item_id:"+opts.ItemID)
	}
	if opts.Search != "" {
		args = append(args, "search:"+encodeArg(opts.Search))
	}
	args = append(args, "want_url:1")

	items, err := server.queryTaggedList(listCommand("favorites items", from, to, args))
	if err != nil {
		return nil, err
	}
	favorites := make([]Favorite, len(items))
	for i, item := range items {
		favorites[i] = favoriteFromRecord(item)
	}
	return favorites, nil
}

// FavoriteExists reports whether a favorite with the given ID or URL exists,
// and its index if so.
func (server *LyrionServer) FavoriteExists(idOrURL string) (bool, int, error) {
	command := fmt.Sprintf("favorites exists %v", encodeArg(idOrURL))
	meta, _, err := server.queryTagged(command, map[string]bool{"exists": true, "index": true})
	if err != nil {
		return false, 0, err
	}
	return meta["exists"] == "1", atoiSafe(meta["index"]), nil
}

type FavoriteAddOpts struct {
	ItemID string // insertion position; empty appends
	Title  string // mandatory
	URL    string // mandatory for FavoriteAdd; unused for FavoriteAddLevel
	Icon   string
}

func (opts FavoriteAddOpts) args() []string {
	args := []string{"title:" + encodeArg(opts.Title)}
	if opts.ItemID != "" {
		args = append(args, "item_id:"+opts.ItemID)
	}
	if opts.URL != "" {
		args = append(args, "url:"+encodeArg(opts.URL))
	}
	if opts.Icon != "" {
		args = append(args, "icon:"+encodeArg(opts.Icon))
	}
	return args
}

func (server *LyrionServer) FavoriteAdd(opts FavoriteAddOpts) error {
	_, err := server.Query("favorites add " + strings.Join(opts.args(), " "))
	return err
}

// FavoriteAddLevel creates a favorites folder.
func (server *LyrionServer) FavoriteAddLevel(opts FavoriteAddOpts) error {
	_, err := server.Query("favorites addlevel " + strings.Join(opts.args(), " "))
	return err
}

func (server *LyrionServer) FavoriteDelete(itemID string) error {
	_, err := server.Query("favorites delete item_id:" + itemID)
	return err
}

func (server *LyrionServer) FavoriteRename(itemID, newTitle string) error {
	_, err := server.Query(fmt.Sprintf("favorites rename item_id:%v title:%v", itemID, encodeArg(newTitle)))
	return err
}

func (server *LyrionServer) FavoriteMove(fromID, toID string) error {
	_, err := server.Query(fmt.Sprintf("favorites move from_id:%v to_id:%v", fromID, toID))
	return err
}

type FavoritePlaylistOpts struct {
	ItemID string
}

// FavoritePlaylist plays, loads, inserts or adds a favorite to the player's playlist.
func (player *LyrionPlayer) FavoritePlaylist(action string, opts FavoritePlaylistOpts) error {
	request := fmt.Sprintf("%v favorites playlist %v item_id:%v", player.id, action, opts.ItemID)
	_, err := player.server.Query(request)
	return err
}
