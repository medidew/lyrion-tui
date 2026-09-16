package lyrionapi

import "fmt"

type RandomGenre struct {
	Name    string
	Enabled bool
}

// RandomPlay starts a random mix of the given kind ("tracks", "albums",
// "contributors" or "year"), or stops the current mix ("disable").
func (player *LyrionPlayer) RandomPlay(kind string) error {
	_, err := player.server.Query(fmt.Sprintf("%v randomplay %v", player.id, kind))
	return err
}

// RandomPlayGenreList returns every genre and whether it's currently
// included in random mixes.
func (player *LyrionPlayer) RandomPlayGenreList() ([]RandomGenre, error) {
	command := fmt.Sprintf("%v randomplaygenrelist", player.id)
	_, records, err := player.server.queryTagged(command, map[string]bool{"count": true, "offset": true})
	if err != nil {
		return nil, err
	}

	genres := make([]RandomGenre, len(records))
	for i, record := range records {
		genres[i] = RandomGenre{Name: tagString(record, "text"), Enabled: tagBool(record, "checkbox")}
	}
	return genres, nil
}

func (player *LyrionPlayer) RandomPlayChooseGenre(genre string, enabled bool) error {
	request := fmt.Sprintf("%v randomplaychoosegenre %v %v", player.id, encodeArg(genre), boolToInt(enabled))
	_, err := player.server.Query(request)
	return err
}

func (player *LyrionPlayer) RandomPlayGenreSelectAll(enabled bool) error {
	_, err := player.server.Query(fmt.Sprintf("%v randomplaygenreselectall %v", player.id, boolToInt(enabled)))
	return err
}

// RandomPlayIsActive returns the active mix type ("tracks", "albums",
// "contributors" or "year"), or "" if random play is inactive.
func (player *LyrionPlayer) RandomPlayIsActive() (string, error) {
	response, err := player.server.Query(fmt.Sprintf("%v randomplayisactive", player.id))
	if err != nil {
		return "", err
	}

	prefix := fmt.Sprintf("%v randomplayisactive ", player.id)
	if len(response) <= len(prefix) {
		return "", nil
	}
	active := response[len(prefix):]
	if active == "0" {
		return "", nil
	}
	return active, nil
}
