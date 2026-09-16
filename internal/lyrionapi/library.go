package lyrionapi

import (
	"fmt"
	"strconv"
	"strings"
)

type Genre struct {
	Name string
	ID   int
}

type Artist struct {
	Name string
	ID   int
}

type Album struct {
	ID             int
	Title          string
	Year           int
	ArtworkTrackID string
	ArtistName     string
	TextKey        string
}

type Track struct {
	ID       int
	Title    string
	Artist   string
	Album    string
	Genre    string
	Duration float64
	TrackNum int
	Year     int
	URL      string
}

// Song is a snapshot of a single track's playback-relevant metadata, as
// returned by LyrionPlayer.CurrentSong.
type Song struct {
	Title    string
	Artist   string
	Album    string
	Genre    string
	Duration float64
	Path     string
}

type FolderItem struct {
	ID       int
	Name     string
	Type     string // "track", "folder", "playlist", or "unknown"
	CoverID  string
	Duration float64
	TextKey  string
	URL      string
}

// Playlist is a saved playlist (distinct from a player's live/current
// playlist, which LyrionPlayer's methods in playlist.go operate on). Its URL
// can be passed directly to LyrionPlayer.PlaySong/AddSong/InsertSong, which
// are generic over songs, playlists and directories.
type Playlist struct {
	ID     string
	Name   string
	URL    string
	Remote bool
}

type SearchResults struct {
	ArtistsCount int
	AlbumsCount  int
	GenresCount  int
	TracksCount  int
	Artists      []Artist
	Albums       []Album
	Genres       []Genre
	Tracks       []Track
}

func (server *LyrionServer) totalInfoField(field string) (int, error) {
	request := "info total " + field + " ?"
	response, err := server.Query(request)
	if err != nil {
		return -1, err
	}
	n, err := strconv.ParseInt(sliceOrEmpty(response, len(request)-1), 10, 0)
	return int(n), err
}

func (server *LyrionServer) TotalGenres() (int, error) {
	return server.totalInfoField("genres")
}

func (server *LyrionServer) TotalArtists() (int, error) {
	return server.totalInfoField("artists")
}

func (server *LyrionServer) TotalAlbums() (int, error) {
	return server.totalInfoField("albums")
}

func (server *LyrionServer) TotalSongs() (int, error) {
	return server.totalInfoField("songs")
}

type GenreQueryOpts struct {
	Search   string
	ArtistID string
	AlbumID  string
	GenreID  string
	Year     string
}

func (opts GenreQueryOpts) args() []string {
	var args []string
	if opts.Search != "" {
		args = append(args, "search:"+encodeArg(opts.Search))
	}
	if opts.ArtistID != "" {
		args = append(args, "artist_id:"+opts.ArtistID)
	}
	if opts.AlbumID != "" {
		args = append(args, "album_id:"+opts.AlbumID)
	}
	if opts.GenreID != "" {
		args = append(args, "genre_id:"+opts.GenreID)
	}
	if opts.Year != "" {
		args = append(args, "year:"+opts.Year)
	}
	return args
}

type ArtistQueryOpts struct {
	Search   string
	GenreID  string
	AlbumID  string
	ArtistID string
}

func (opts ArtistQueryOpts) args() []string {
	var args []string
	if opts.Search != "" {
		args = append(args, "search:"+encodeArg(opts.Search))
	}
	if opts.GenreID != "" {
		args = append(args, "genre_id:"+opts.GenreID)
	}
	if opts.AlbumID != "" {
		args = append(args, "album_id:"+opts.AlbumID)
	}
	if opts.ArtistID != "" {
		args = append(args, "artist_id:"+opts.ArtistID)
	}
	return args
}

type AlbumQueryOpts struct {
	Search   string
	GenreID  string
	ArtistID string
	AlbumID  string
	Year     string
	Tags     string // defaults to "layjs" (title, artist, year, artwork id, textkey)
}

func (opts AlbumQueryOpts) args() []string {
	args := []string{"tags:" + orDefault(opts.Tags, "layjs")}
	if opts.Search != "" {
		args = append(args, "search:"+encodeArg(opts.Search))
	}
	if opts.GenreID != "" {
		args = append(args, "genre_id:"+opts.GenreID)
	}
	if opts.ArtistID != "" {
		args = append(args, "artist_id:"+opts.ArtistID)
	}
	if opts.AlbumID != "" {
		args = append(args, "album_id:"+opts.AlbumID)
	}
	if opts.Year != "" {
		args = append(args, "year:"+opts.Year)
	}
	return args
}

type TrackQueryOpts struct {
	Search   string
	GenreID  string
	ArtistID string
	AlbumID  string
	Year     string
	Tags     string // defaults to "galdyu" (genre, artist, album, duration, year, url)
}

func (opts TrackQueryOpts) args() []string {
	args := []string{"tags:" + orDefault(opts.Tags, "galdyu")}
	if opts.Search != "" {
		args = append(args, "search:"+encodeArg(opts.Search))
	}
	if opts.GenreID != "" {
		args = append(args, "genre_id:"+opts.GenreID)
	}
	if opts.ArtistID != "" {
		args = append(args, "artist_id:"+opts.ArtistID)
	}
	if opts.AlbumID != "" {
		args = append(args, "album_id:"+opts.AlbumID)
	}
	if opts.Year != "" {
		args = append(args, "year:"+opts.Year)
	}
	return args
}

type MusicFolderOpts struct {
	FolderID  string
	URL       string
	Recursive bool
}

func (opts MusicFolderOpts) args() []string {
	var args []string
	if opts.FolderID != "" {
		args = append(args, "folder_id:"+opts.FolderID)
	}
	if opts.URL != "" {
		args = append(args, "url:"+encodeArg(opts.URL))
	}
	if opts.Recursive {
		args = append(args, "recursive:1")
	}
	return args
}

type PlaylistQueryOpts struct {
	Search string
}

func (opts PlaylistQueryOpts) args() []string {
	args := []string{"tags:ux"}
	if opts.Search != "" {
		args = append(args, "search:"+encodeArg(opts.Search))
	}
	return args
}

func orDefault(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func listCommand(verb string, from, to int, args []string) string {
	parts := append([]string{verb, strconv.Itoa(from), strconv.Itoa(to)}, args...)
	return strings.Join(parts, " ")
}

func genreFromRecord(record map[string]string) Genre {
	return Genre{ID: tagInt(record, "id"), Name: tagString(record, "genre")}
}

func artistFromRecord(record map[string]string) Artist {
	return Artist{ID: tagInt(record, "id"), Name: tagString(record, "artist")}
}

func playlistFromRecord(record map[string]string) Playlist {
	return Playlist{
		ID:     tagString(record, "id"),
		Name:   tagString(record, "playlist"),
		URL:    tagString(record, "url"),
		Remote: tagBool(record, "remote"),
	}
}

// albumFromRecord and trackFromRecord map the *response* field names
// (verified against a live LMS 9.1.1 server): the request-side "tags:"
// string still uses the reference docs' single-letter codes to select which
// fields to include, but the server labels the fields it returns with full
// descriptive names (e.g. "album", "artwork_track_id"), not those letters.
func albumFromRecord(record map[string]string) Album {
	return Album{
		ID:             tagInt(record, "id"),
		Title:          tagString(record, "album"),
		Year:           tagInt(record, "year"),
		ArtworkTrackID: tagString(record, "artwork_track_id"),
		ArtistName:     tagString(record, "artist"),
		TextKey:        tagString(record, "textkey"),
	}
}

func trackFromRecord(record map[string]string) Track {
	return Track{
		ID:       tagInt(record, "id"),
		Title:    tagString(record, "title"),
		Artist:   tagString(record, "artist"),
		Album:    tagString(record, "album"),
		Genre:    tagString(record, "genre"),
		Duration: tagFloat(record, "duration"),
		TrackNum: tagInt(record, "tracknum"),
		Year:     tagInt(record, "year"),
		URL:      tagString(record, "url"),
	}
}

// id/filename/type were confirmed live; coverid/duration/textkey/url were
// not (they only appear when explicitly requested via tags:, which
// GetMusicFolder doesn't currently do) - named here by extrapolating the
// same full-word convention confirmed for every other command.
func folderItemFromRecord(record map[string]string) FolderItem {
	return FolderItem{
		ID:       tagInt(record, "id"),
		Name:     tagString(record, "filename"),
		Type:     tagString(record, "type"),
		CoverID:  tagString(record, "coverid"),
		Duration: tagFloat(record, "duration"),
		TextKey:  tagString(record, "textkey"),
		URL:      tagString(record, "url"),
	}
}

var listMetaKeys = map[string]bool{"count": true, "rescan": true}

func (server *LyrionServer) queryTaggedList(command string) ([]map[string]string, error) {
	_, items, err := server.queryTagged(command, listMetaKeys)
	return items, err
}

func (server *LyrionServer) GetGenres(from, to int, opts GenreQueryOpts) ([]Genre, error) {
	items, err := server.queryTaggedList(listCommand("genres", from, to, opts.args()))
	if err != nil {
		return nil, err
	}
	genres := make([]Genre, len(items))
	for i, item := range items {
		genres[i] = genreFromRecord(item)
	}
	return genres, nil
}

func (server *LyrionServer) GetArtists(from, to int, opts ArtistQueryOpts) ([]Artist, error) {
	items, err := server.queryTaggedList(listCommand("artists", from, to, opts.args()))
	if err != nil {
		return nil, err
	}
	artists := make([]Artist, len(items))
	for i, item := range items {
		artists[i] = artistFromRecord(item)
	}
	return artists, nil
}

func (server *LyrionServer) GetPlaylists(from, to int, opts PlaylistQueryOpts) ([]Playlist, error) {
	items, err := server.queryTaggedList(listCommand("playlists", from, to, opts.args()))
	if err != nil {
		return nil, err
	}
	playlists := make([]Playlist, len(items))
	for i, item := range items {
		playlists[i] = playlistFromRecord(item)
	}
	return playlists, nil
}

func (server *LyrionServer) GetAlbums(from, to int, opts AlbumQueryOpts) ([]Album, error) {
	items, err := server.queryTaggedList(listCommand("albums", from, to, opts.args()))
	if err != nil {
		return nil, err
	}
	albums := make([]Album, len(items))
	for i, item := range items {
		albums[i] = albumFromRecord(item)
	}
	return albums, nil
}

func (server *LyrionServer) GetYears(from, to int) ([]int, error) {
	items, err := server.queryTaggedList(listCommand("years", from, to, nil))
	if err != nil {
		return nil, err
	}
	years := make([]int, len(items))
	for i, item := range items {
		years[i] = tagInt(item, "year")
	}
	return years, nil
}

func (server *LyrionServer) GetTitles(from, to int, opts TrackQueryOpts) ([]Track, error) {
	items, err := server.queryTaggedList(listCommand("titles", from, to, opts.args()))
	if err != nil {
		return nil, err
	}
	tracks := make([]Track, len(items))
	for i, item := range items {
		tracks[i] = trackFromRecord(item)
	}
	return tracks, nil
}

// GetSongInfo looks up a single track by track ID or by its file:// / http:// URL.
func (server *LyrionServer) GetSongInfo(trackIDOrURL string, tags string) (Track, error) {
	idArg := "track_id:" + trackIDOrURL
	if strings.Contains(trackIDOrURL, "://") {
		idArg = "url:" + encodeArg(trackIDOrURL)
	}
	command := listCommand("songinfo", 0, 100, []string{idArg, "tags:" + orDefault(tags, "galdyu")})

	items, err := server.queryTaggedList(command)
	if err != nil {
		return Track{}, err
	}
	if len(items) == 0 {
		return Track{}, fmt.Errorf("lyrionapi: no song info returned for %q", trackIDOrURL)
	}
	return trackFromRecord(items[0]), nil
}

func (server *LyrionServer) GetMusicFolder(from, to int, opts MusicFolderOpts) ([]FolderItem, error) {
	items, err := server.queryTaggedList(listCommand("musicfolder", from, to, opts.args()))
	if err != nil {
		return nil, err
	}
	folders := make([]FolderItem, len(items))
	for i, item := range items {
		folders[i] = folderItemFromRecord(item)
	}
	return folders, nil
}

// Search performs a global search across artists, albums, genres and tracks.
//
// The response mixes four different record shapes (artist_id/artist,
// album_id/album, genre_id/genre, track_id/track) one category at a time, so
// it can't use the generic repeated-key record parser in protocol.go (which
// assumes every record in a list shares the same field names) - each
// category is parsed here as fixed id/name pairs instead.
func (server *LyrionServer) Search(term string, from, to int) (SearchResults, error) {
	command := listCommand("search", from, to, []string{"term:" + encodeArg(term)})

	line, err := server.doQueryRaw(command)
	if err != nil {
		return SearchResults{}, err
	}
	tokens, err := tokenize(line)
	if err != nil {
		return SearchResults{}, err
	}

	skip := countTokens(command)
	if skip > len(tokens) {
		skip = len(tokens)
	}
	tokens = tokens[skip:]

	var results SearchResults
	for i := 0; i < len(tokens); i++ {
		key, value, ok := splitTag(tokens[i])
		if !ok {
			continue
		}

		nameOf := func() string {
			if i+1 >= len(tokens) {
				return ""
			}
			_, name, _ := splitTag(tokens[i+1])
			return name
		}

		switch key {
		case "artists_count":
			results.ArtistsCount = atoiSafe(value)
		case "albums_count":
			results.AlbumsCount = atoiSafe(value)
		case "genres_count":
			results.GenresCount = atoiSafe(value)
		case "tracks_count":
			results.TracksCount = atoiSafe(value)
		case "artist_id":
			results.Artists = append(results.Artists, Artist{ID: atoiSafe(value), Name: nameOf()})
			i++
		case "album_id":
			results.Albums = append(results.Albums, Album{ID: atoiSafe(value), Title: nameOf()})
			i++
		case "genre_id":
			results.Genres = append(results.Genres, Genre{ID: atoiSafe(value), Name: nameOf()})
			i++
		case "track_id":
			results.Tracks = append(results.Tracks, Track{ID: atoiSafe(value), Title: nameOf()})
			i++
		}
	}

	return results, nil
}

func atoiSafe(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}
