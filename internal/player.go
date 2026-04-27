package lyrionapi

import (
	"fmt"
	"strconv"
)

type LyrionPlayer struct {
	id     string
	name   string
	model  string
	server *LyrionServer
}

func (server *LyrionServer) GetPlayerCount() (int, error) {
	response, err := server.Query("player count ?")
	if err != nil {
		return -1, err
	}

	// expected response is 14 characters in
	count, err := strconv.ParseInt(response[13:], 10, 0)
	if err != nil {
		return -1, err
	}

	return int(count), nil
}

func (server *LyrionServer) GetPlayer(index int) (*LyrionPlayer, error) {
	request := fmt.Sprintf("player id %v ?", index)
	id, err := server.Query(request)
	if err != nil {
		return nil, err
	}
	id = id[len(request)-1:]

	request = fmt.Sprintf("%v name ?", id)
	name, err := server.Query(request)
	if err != nil {
		return nil, err
	}
	name = name[len(request)-1:]

	request = fmt.Sprintf("player model %v ?", id)
	model, err := server.Query(request)
	if err != nil {
		return nil, err
	}
	model = model[len(request)-1:]

	return &LyrionPlayer{
		id:     id,
		name:   name,
		model:  model,
		server: server,
	}, nil
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
	request := fmt.Sprintf("%v mode ?", player.id)
	state, err := player.server.Query(request)
	if err != nil {
		return "", err
	}
	state = state[len(request)-1:]
	return state, nil
}

func (player *LyrionPlayer) Time() (float64, error) {
	request := fmt.Sprintf("%v time ?", player.id)
	time, err := player.server.Query(request)
	if err != nil {
		return 0, err
	}
	time = time[len(request)-1:]

	time_f, err := strconv.ParseFloat(time, 64)
	if err != nil {
		return 0, err
	}

	return time_f, nil
}

func (player *LyrionPlayer) CurrentSong() (Song, error) {
	song := Song{}

	request := fmt.Sprintf("%v title ?", player.id)
	title, err := player.server.Query(request)
	if err != nil {
		return song, err
	}
	song.Title = title[len(request)-1:]

	request = fmt.Sprintf("%v genre ?", player.id)
	genre, err := player.server.Query(request)
	if err != nil {
		return song, err
	}
	song.Genre = genre[len(request)-1:]

	request = fmt.Sprintf("%v artist ?", player.id)
	artist, err := player.server.Query(request)
	if err != nil {
		return song, err
	}
	song.Artist = artist[len(request)-1:]

	request = fmt.Sprintf("%v album ?", player.id)
	album, err := player.server.Query(request)
	if err != nil {
		return song, err
	}
	song.Album = album[len(request)-1:]

	request = fmt.Sprintf("%v duration ?", player.id)
	duration, err := player.server.Query(request)
	if err != nil {
		return song, err
	}
	duration = duration[len(request)-1:]

	song.Duration, err = strconv.ParseFloat(duration, 64)
	if err != nil {
		return song, err
	}

	request = fmt.Sprintf("%v path ?", player.id)
	path, err := player.server.Query(request)
	if err != nil {
		return song, err
	}
	song.Path = path[len(request)-1:]

	return song, nil
}
