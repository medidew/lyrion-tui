package lyrionapi

import (
	"fmt"
	"strings"
)

type PlayerSummary struct {
	Index       int
	ID          string
	UUID        string
	IP          string
	Name        string
	Model       string
	ModelName   string
	Power       bool
	IsPlaying   bool
	DisplayType string
	Connected   bool
}

func playerSummaryFromRecord(record map[string]string) PlayerSummary {
	return PlayerSummary{
		Index:       tagInt(record, "playerindex"),
		ID:          tagString(record, "playerid"),
		UUID:        tagString(record, "uuid"),
		IP:          tagString(record, "ip"),
		Name:        tagString(record, "name"),
		Model:       tagString(record, "model"),
		ModelName:   tagString(record, "modelname"),
		Power:       tagBool(record, "power"),
		IsPlaying:   tagBool(record, "isplaying"),
		DisplayType: tagString(record, "displaytype"),
		Connected:   tagBool(record, "connected"),
	}
}

// Players lists every player known to the server.
func (server *LyrionServer) Players(from, to int) ([]PlayerSummary, error) {
	items, err := server.queryTaggedList(listCommand("players", from, to, nil))
	if err != nil {
		return nil, err
	}
	players := make([]PlayerSummary, len(items))
	for i, item := range items {
		players[i] = playerSummaryFromRecord(item)
	}
	return players, nil
}

type SyncGroup struct {
	MemberIDs   []string
	MemberNames []string
}

func (server *LyrionServer) SyncGroups() ([]SyncGroup, error) {
	_, items, err := server.queryTagged("syncgroups ?", nil)
	if err != nil {
		return nil, err
	}
	groups := make([]SyncGroup, len(items))
	for i, item := range items {
		groups[i] = SyncGroup{
			MemberIDs:   splitNonEmpty(tagString(item, "sync_members"), ","),
			MemberNames: splitNonEmpty(tagString(item, "sync_member_names"), ","),
		}
	}
	return groups, nil
}

func splitNonEmpty(s, sep string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(s, sep)
}

type Library struct {
	ID   string
	Name string
}

func (server *LyrionServer) Libraries() ([]Library, error) {
	_, items, err := server.queryTagged("libraries", nil)
	if err != nil {
		return nil, err
	}
	libraries := make([]Library, len(items))
	for i, item := range items {
		libraries[i] = Library{ID: tagString(item, "id"), Name: tagString(item, "name")}
	}
	return libraries, nil
}

// LibraryID returns the ID of the virtual library active for this player
// ("0" if none is active).
func (player *LyrionPlayer) LibraryID() (string, error) {
	command := fmt.Sprintf("%v libraries getid", player.id)
	meta, _, err := player.server.queryTagged(command, map[string]bool{"id": true, "name": true})
	if err != nil {
		return "", err
	}
	return meta["id"], nil
}

func (server *LyrionServer) Version() (string, error) {
	request := "version ?"
	response, err := server.Query(request)
	if err != nil {
		return "", err
	}
	return response[len(request)-1:], nil
}

// Can reports whether the server recognizes the given command/query terms.
func (server *LyrionServer) Can(requestTerms string) (bool, error) {
	request := fmt.Sprintf("can %v ?", requestTerms)
	response, err := server.Query(request)
	if err != nil {
		return false, err
	}
	return response[len(request)-1:] == "1", nil
}

type ReadDirOpts struct {
	Filter string // "foldersonly", "filesonly", "musicfiles", "filetype:xyz", or a custom regex
}

// DirEntry's field names (path/isfolder) are a best-effort reading of the
// reference docs' prose description ("item paths and folder/file flags") -
// unlike the rest of this file's tag names, these were not confirmed
// against a raw server response and may need correcting.
type DirEntry struct {
	Path     string
	IsFolder bool
}

func (server *LyrionServer) ReadDirectory(folder string, from, to int, opts ReadDirOpts) ([]DirEntry, error) {
	args := []string{"folder:" + encodeArg(folder)}
	if opts.Filter != "" {
		args = append(args, "filter:"+opts.Filter)
	}

	items, err := server.queryTaggedList(listCommand("readdirectory", from, to, args))
	if err != nil {
		return nil, err
	}
	entries := make([]DirEntry, len(items))
	for i, item := range items {
		entries[i] = DirEntry{Path: tagString(item, "path"), IsFolder: tagBool(item, "isfolder")}
	}
	return entries, nil
}

type ServerStatus struct {
	TotalGenres  int
	TotalArtists int
	TotalAlbums  int
	TotalSongs   int
	PlayerCount  int
	Players      []PlayerSummary
}

// ServerStatus assembles a snapshot of overall server state by composing
// several already-verified single-purpose queries (TotalGenres, Players,
// ...), rather than parsing the compound "serverstatus" CLI command's own
// response directly: that command's top-level tag vocabulary (version, uuid,
// ip, ...) isn't confirmed against a live server the way the narrower
// commands used here are, and getting a tag name wrong there would silently
// return zero-valued fields. This can be folded into a single round trip
// once that's verified.
func (server *LyrionServer) ServerStatus() (ServerStatus, error) {
	var status ServerStatus
	var err error

	if status.TotalGenres, err = server.TotalGenres(); err != nil {
		return status, err
	}
	if status.TotalArtists, err = server.TotalArtists(); err != nil {
		return status, err
	}
	if status.TotalAlbums, err = server.TotalAlbums(); err != nil {
		return status, err
	}
	if status.TotalSongs, err = server.TotalSongs(); err != nil {
		return status, err
	}
	if status.PlayerCount, err = server.GetPlayerCount(); err != nil {
		return status, err
	}
	if status.Players, err = server.Players(0, status.PlayerCount); err != nil {
		return status, err
	}

	return status, nil
}

type PlayerStatus struct {
	Mode        string
	Power       bool
	Volume      int
	Shuffle     int
	Repeat      int
	TrackCount  int
	Index       int
	CurrentSong Song
}

// Status assembles a snapshot of this player's current state, composing
// several already-verified single-purpose queries rather than parsing the
// compound "status" CLI command's own response - see ServerStatus for why.
func (player *LyrionPlayer) Status() (PlayerStatus, error) {
	var status PlayerStatus
	var err error

	if status.Mode, err = player.Mode(); err != nil {
		return status, err
	}
	if status.Power, err = player.Power(); err != nil {
		return status, err
	}
	if status.Volume, err = player.Volume(); err != nil {
		return status, err
	}
	if status.Shuffle, err = player.Shuffle(); err != nil {
		return status, err
	}
	if status.Repeat, err = player.Repeat(); err != nil {
		return status, err
	}
	if status.TrackCount, err = player.TrackCount(); err != nil {
		return status, err
	}
	if status.Index, err = player.Index(); err != nil {
		return status, err
	}
	if status.CurrentSong, err = player.CurrentSong(); err != nil {
		return status, err
	}

	return status, nil
}
