package lyrionapi

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"time"
)

type LyrionServer struct {
	conn net.Conn
}

type Genre struct {
	Name string
	ID   int
}

type Song struct {
	Title    string
	Artist   string
	Album    string
	Genre    string
	Duration float64
	Path     string
}

// Initiates Telnet connection to the music server, which remains open until close() is called.
func Connect(address string) (*LyrionServer, error) {
	var lyrion_dialer net.Dialer
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()

	lyrion_connection, err := lyrion_dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return nil, err
	}

	server := &LyrionServer{
		conn: lyrion_connection,
	}

	return server, err
}

// Sends a command across the Telnet connection and returns the response.
func (server *LyrionServer) Query(command string) (string, error) {
	command += "\n"
	_, err := server.conn.Write([]byte(command))
	if err != nil {
		return "", err
	}

	response := ""
	buffer := make([]byte, 1)
	for buffer[0] != '\n' {
		_, err := server.conn.Read(buffer)
		if err != nil {
			return "", err
		}
		response += string(buffer[0])
	}

	response, err = url.QueryUnescape(response)
	if err != nil {
		return "", err
	}

	return response[:len(response)-1], nil
}

// Closes the connection to the music server.
func (server *LyrionServer) Close() error {
	return server.conn.Close()
}

func (server *LyrionServer) TotalGenres() (int, error) {
	genres, err := server.Query("info total genres ?")
	if err != nil {
		return -1, err
	}
	num_genres, err := strconv.ParseInt(genres[17:], 10, 0)
	return int(num_genres), nil
}

func (server *LyrionServer) TotalArtists() (int, error) {
	artists, err := server.Query("info total artists ?")
	if err != nil {
		return -1, err
	}
	num_artists, err := strconv.ParseInt(artists[18:], 10, 0)
	return int(num_artists), nil
}

func (server *LyrionServer) TotalAlbums() (int, error) {
	albums, err := server.Query("info total albums ?")
	if err != nil {
		return -1, err
	}
	num_albums, err := strconv.ParseInt(albums[17:], 10, 0)
	return int(num_albums), nil
}

func (server *LyrionServer) TotalSongs() (int, error) {
	songs, err := server.Query("info total songs ?")
	if err != nil {
		return -1, err
	}
	num_songs, err := strconv.ParseInt(songs[16:], 10, 0)
	return int(num_songs), nil
}

func (server *LyrionServer) GetGenres(from_index int, to_index int) ([]Genre, error) {
	request := fmt.Sprintf("genres %v %v", from_index, to_index)
	genres, err := server.Query(request)

	fmt.Printf("genres: %v\n", genres) // TODO: parse response into slice

	if err != nil {
		return nil, err
	}
	return nil, nil
}

// TODO: implement the rest of the basic DB queries + search function
