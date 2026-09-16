package lyrionapi

import (
	"bufio"
	"context"
	"net"
	"net/url"
	"sync"
	"sync/atomic"
	"time"
)

type LyrionServer struct {
	conn       net.Conn
	connReader *bufio.Reader
	address    string

	mu              sync.Mutex
	pending         []*pendingRequest
	startReaderOnce sync.Once
	closeOnce       sync.Once
	closed          atomic.Bool

	notifyMu    sync.Mutex
	notifyConn  net.Conn
	subscribers []*subscriber
}

// Initiates Telnet connection to the music server, which remains open until close() is called.
func Connect(address string) (*LyrionServer, error) {
	var dialer net.Dialer
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()

	conn, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return nil, err
	}

	server := &LyrionServer{
		conn:       conn,
		connReader: bufio.NewReader(conn),
		address:    address,
	}

	return server, err
}

// Sends a command across the Telnet connection and returns the response.
func (server *LyrionServer) Query(command string) (string, error) {
	line, err := server.doQueryRaw(command)
	if err != nil {
		return "", err
	}
	return url.QueryUnescape(line)
}

// queryTagged sends command and parses its response as a tagged, possibly
// multi-item response: topLevelKeys names the tags (e.g. "count", "rescan")
// that appear once before any items rather than as part of an item record.
func (server *LyrionServer) queryTagged(command string, topLevelKeys map[string]bool) (map[string]string, []map[string]string, error) {
	line, err := server.doQueryRaw(command)
	if err != nil {
		return nil, nil, err
	}

	tokens, err := tokenize(line)
	if err != nil {
		return nil, nil, err
	}

	skip := countTokens(command)
	if skip > len(tokens) {
		skip = len(tokens)
	}

	meta, items := parseTagged(tokens[skip:], topLevelKeys)
	return meta, items, nil
}

// Closes the connection(s) to the music server. Safe to call more than once.
func (server *LyrionServer) Close() error {
	var err error
	server.closeOnce.Do(func() {
		server.closed.Store(true)
		err = server.conn.Close()
		server.failPending(ErrClosed)

		server.notifyMu.Lock()
		notifyConn := server.notifyConn
		server.notifyMu.Unlock()
		if notifyConn != nil {
			notifyConn.Close()
		}
	})
	return err
}
