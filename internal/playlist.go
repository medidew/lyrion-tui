package lyrionapi

import (
	"fmt"
	"strconv"
	"strings"
)

// queryField sends command (expected to end in "?") and strips the echoed
// request prefix from the response, leaving just the value. When there's
// nothing to report (e.g. querying the current title while stopped), LMS
// can return a response shorter than the echoed prefix itself (the trailing
// "?" simply isn't replaced with anything) - that's treated as an empty
// value rather than an error.
func (player *LyrionPlayer) queryField(command string) (string, error) {
	response, err := player.server.Query(command)
	if err != nil {
		return "", err
	}
	if len(response) < len(command)-1 {
		return "", nil
	}
	return response[len(command)-1:], nil
}

func (player *LyrionPlayer) queryIntField(command string) (int, error) {
	value, err := player.queryField(command)
	if err != nil || value == "" {
		return 0, err
	}
	n, err := strconv.ParseInt(value, 10, 0)
	if err != nil {
		return 0, err
	}
	return int(n), nil
}

func (player *LyrionPlayer) queryBoolField(command string) (bool, error) {
	value, err := player.queryField(command)
	if err != nil {
		return false, err
	}
	return value == "1", nil
}

func (player *LyrionPlayer) Play(fadein_duration int) error {
	request := fmt.Sprintf("%v play %v", player.id, fadein_duration)
	_, err := player.server.Query(request)
	return err
}

func (player *LyrionPlayer) Stop() error {
	request := fmt.Sprintf("%v stop", player.id)
	_, err := player.server.Query(request)
	return err
}

func (player *LyrionPlayer) Pause() error {
	request := fmt.Sprintf("%v pause 1", player.id)
	_, err := player.server.Query(request)
	return err
}

func (player *LyrionPlayer) Unpause(fadein_duration int) error {
	request := fmt.Sprintf("%v pause 0 %v", player.id, fadein_duration)
	_, err := player.server.Query(request)
	return err
}

func (player *LyrionPlayer) Mode() (string, error) {
	return player.queryField(fmt.Sprintf("%v mode ?", player.id))
}

func (player *LyrionPlayer) Time() (float64, error) {
	value, err := player.queryField(fmt.Sprintf("%v time ?", player.id))
	if err != nil {
		return 0, err
	}
	return strconv.ParseFloat(value, 64)
}

// CurrentTitle returns the current title for remote streams, or the song
// title formatted as it appears on the player's display.
func (player *LyrionPlayer) CurrentTitle() (string, error) {
	return player.queryField(fmt.Sprintf("%v current_title ?", player.id))
}

// Remote reports whether the current track is a remote stream.
func (player *LyrionPlayer) Remote() (bool, error) {
	return player.queryBoolField(fmt.Sprintf("%v remote ?", player.id))
}

func (player *LyrionPlayer) CurrentSong() (Song, error) {
	song := Song{}
	var err error

	if song.Title, err = player.queryField(fmt.Sprintf("%v title ?", player.id)); err != nil {
		return song, err
	}
	if song.Genre, err = player.queryField(fmt.Sprintf("%v genre ?", player.id)); err != nil {
		return song, err
	}
	if song.Artist, err = player.queryField(fmt.Sprintf("%v artist ?", player.id)); err != nil {
		return song, err
	}
	if song.Album, err = player.queryField(fmt.Sprintf("%v album ?", player.id)); err != nil {
		return song, err
	}
	if song.Duration, err = player.queryFloatField(fmt.Sprintf("%v duration ?", player.id)); err != nil {
		return song, err
	}
	if song.Path, err = player.queryField(fmt.Sprintf("%v path ?", player.id)); err != nil {
		return song, err
	}

	return song, nil
}

func (player *LyrionPlayer) queryFloatField(command string) (float64, error) {
	value, err := player.queryField(command)
	if err != nil || value == "" {
		return 0, err
	}
	return strconv.ParseFloat(value, 64)
}

// Can also be used for playlists.
func (player *LyrionPlayer) PlaySong(song string, song_title string, fadein_duration int) error {
	request := fmt.Sprintf("%v playlist play %v %v %v", player.id, song, song_title, fadein_duration)
	_, err := player.server.Query(request)
	return err
}

// Can also be used for playlists.
func (player *LyrionPlayer) AddSong(song string, song_title string) error {
	request := fmt.Sprintf("%v playlist add %v %v", player.id, song, song_title)
	_, err := player.server.Query(request)
	return err
}

// Can also be used for playlists.
func (player *LyrionPlayer) InsertSong(song string, song_title string) error {
	request := fmt.Sprintf("%v playlist insert %v %v", player.id, song, song_title)
	_, err := player.server.Query(request)
	return err
}

func (player *LyrionPlayer) DeleteSongByTitle(song string) error {
	request := fmt.Sprintf("%v playlist deleteitem %v", player.id, song)
	_, err := player.server.Query(request)
	return err
}

func (player *LyrionPlayer) MoveSong(from_index int, to_index int) error {
	request := fmt.Sprintf("%v playlist move %v %v", player.id, from_index, to_index)
	_, err := player.server.Query(request)
	return err
}

func (player *LyrionPlayer) DeleteSong(index int) error {
	request := fmt.Sprintf("%v playlist delete %v", player.id, index)
	_, err := player.server.Query(request)
	return err
}

// Clear removes every song from the playlist and stops playback.
func (player *LyrionPlayer) Clear() error {
	_, err := player.server.Query(fmt.Sprintf("%v playlist clear", player.id))
	return err
}

// Zap adds the song at index to the zapped-song playlist and removes it from
// the current playlist.
func (player *LyrionPlayer) Zap(index int) error {
	_, err := player.server.Query(fmt.Sprintf("%v playlist zap %v", player.id, index))
	return err
}

// Save saves the current playlist under name in the saved-playlists directory.
func (player *LyrionPlayer) Save(name string) error {
	_, err := player.server.Query(fmt.Sprintf("%v playlist save %v", player.id, encodeArg(name)))
	return err
}

// Resume loads a saved playlist and resumes playback from the previously
// playing track.
func (player *LyrionPlayer) Resume(playlist string) error {
	_, err := player.server.Query(fmt.Sprintf("%v playlist resume %v", player.id, encodeArg(playlist)))
	return err
}

func (player *LyrionPlayer) loadAlbumCommand(verb, genre, artist, album string) error {
	request := fmt.Sprintf("%v playlist %v %v %v %v", player.id, verb, encodeArg(genre), encodeArg(artist), encodeArg(album))
	_, err := player.server.Query(request)
	return err
}

func (player *LyrionPlayer) LoadAlbum(genre, artist, album string) error {
	return player.loadAlbumCommand("loadalbum", genre, artist, album)
}

func (player *LyrionPlayer) AddAlbum(genre, artist, album string) error {
	return player.loadAlbumCommand("addalbum", genre, artist, album)
}

func (player *LyrionPlayer) InsertAlbum(genre, artist, album string) error {
	return player.loadAlbumCommand("insertalbum", genre, artist, album)
}

func (player *LyrionPlayer) DeleteAlbum(genre, artist, album string) error {
	return player.loadAlbumCommand("deletealbum", genre, artist, album)
}

// LoadTracks replaces the playlist with tracks matching search, e.g.
// "track.titlesearch=Yesterday". search is sent as-is (not percent-encoded),
// since it's a "key=value" fragment where only the value portion may need
// encoding - callers are responsible for encoding any free-text value.
func (player *LyrionPlayer) LoadTracks(search string) error {
	_, err := player.server.Query(fmt.Sprintf("%v playlist loadtracks %v", player.id, search))
	return err
}

// AddTracks appends tracks matching search to the current playlist. See
// LoadTracks for the search parameter format.
func (player *LyrionPlayer) AddTracks(search string) error {
	_, err := player.server.Query(fmt.Sprintf("%v playlist addtracks %v", player.id, search))
	return err
}

// Name returns the name of the saved playlist last loaded into the Now
// Playing playlist, if any.
func (player *LyrionPlayer) Name() (string, error) {
	return player.queryField(fmt.Sprintf("%v playlist name ?", player.id))
}

// URL returns the URL of the saved playlist last loaded into the Now
// Playing playlist, if any.
func (player *LyrionPlayer) PlaylistURL() (string, error) {
	return player.queryField(fmt.Sprintf("%v playlist url ?", player.id))
}

// Modified reports whether the loaded playlist has changed since loading.
func (player *LyrionPlayer) Modified() (bool, error) {
	return player.queryBoolField(fmt.Sprintf("%v playlist modified ?", player.id))
}

// TrackCount returns the number of tracks in the current playlist.
func (player *LyrionPlayer) TrackCount() (int, error) {
	return player.queryIntField(fmt.Sprintf("%v playlist tracks ?", player.id))
}

// Index returns the position of the currently playing track in the playlist.
func (player *LyrionPlayer) Index() (int, error) {
	return player.queryIntField(fmt.Sprintf("%v playlist index ?", player.id))
}

// SetIndex jumps to the given playlist position.
func (player *LyrionPlayer) SetIndex(index int) error {
	_, err := player.server.Query(fmt.Sprintf("%v playlist index %v", player.id, index))
	return err
}

// Shuffle returns the current shuffle mode: 0 (off), 1 (by song) or 2 (by album).
func (player *LyrionPlayer) Shuffle() (int, error) {
	return player.queryIntField(fmt.Sprintf("%v playlist shuffle ?", player.id))
}

func (player *LyrionPlayer) SetShuffle(mode int) error {
	_, err := player.server.Query(fmt.Sprintf("%v playlist shuffle %v", player.id, mode))
	return err
}

// Repeat returns the current repeat mode: 0 (off), 1 (song) or 2 (playlist).
func (player *LyrionPlayer) Repeat() (int, error) {
	return player.queryIntField(fmt.Sprintf("%v playlist repeat ?", player.id))
}

func (player *LyrionPlayer) SetRepeat(mode int) error {
	_, err := player.server.Query(fmt.Sprintf("%v playlist repeat %v", player.id, mode))
	return err
}

// TrackAt returns metadata for the playlist entry at index, without
// affecting what's currently playing.
func (player *LyrionPlayer) TrackAt(index int) (Song, error) {
	song := Song{}
	var err error

	field := func(name string) (string, error) {
		return player.queryField(fmt.Sprintf("%v playlist %v %v ?", player.id, name, index))
	}

	if song.Title, err = field("title"); err != nil {
		return song, err
	}
	if song.Genre, err = field("genre"); err != nil {
		return song, err
	}
	if song.Artist, err = field("artist"); err != nil {
		return song, err
	}
	if song.Album, err = field("album"); err != nil {
		return song, err
	}
	durationStr, err := field("duration")
	if err != nil {
		return song, err
	}
	if durationStr == "" {
		durationStr = "0"
	}
	if song.Duration, err = strconv.ParseFloat(durationStr, 64); err != nil {
		return song, err
	}
	if song.Path, err = field("path"); err != nil {
		return song, err
	}

	return song, nil
}

type PlaylistControlOpts struct {
	Cmd        string // mandatory: "load", "add", "insert", or "delete"
	GenreID    string
	ArtistID   string
	AlbumID    string
	TrackID    string
	Year       string
	PlaylistID string
	FolderID   string
	PlayIndex  string
}

// PlaylistControl performs bulk playlist operations using IDs as returned by
// the tagged database queries (GetGenres, GetArtists, GetAlbums, GetTitles).
func (player *LyrionPlayer) PlaylistControl(opts PlaylistControlOpts) error {
	args := []string{"cmd:" + opts.Cmd}
	if opts.GenreID != "" {
		args = append(args, "genre_id:"+opts.GenreID)
	}
	if opts.ArtistID != "" {
		args = append(args, "artist_id:"+opts.ArtistID)
	}
	if opts.AlbumID != "" {
		args = append(args, "album_id:"+opts.AlbumID)
	}
	if opts.TrackID != "" {
		args = append(args, "track_id:"+opts.TrackID)
	}
	if opts.Year != "" {
		args = append(args, "year:"+opts.Year)
	}
	if opts.PlaylistID != "" {
		args = append(args, "playlist_id:"+opts.PlaylistID)
	}
	if opts.FolderID != "" {
		args = append(args, "folder_id:"+opts.FolderID)
	}
	if opts.PlayIndex != "" {
		args = append(args, "play_index:"+opts.PlayIndex)
	}

	request := fmt.Sprintf("%v playlistcontrol %v", player.id, strings.Join(args, " "))
	_, err := player.server.Query(request)
	return err
}
