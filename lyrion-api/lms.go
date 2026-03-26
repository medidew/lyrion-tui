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
	conn    net.Conn
	players []*LyrionPlayer
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

	return &LyrionServer{
		conn:    lyrion_connection,
		players: []*LyrionPlayer{},
	}, nil
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

// Returns the number
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

	return &LyrionPlayer{
		id:     id,
		name:   name,
		server: server,
	}, nil
}
