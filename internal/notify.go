package lyrionapi

import (
	"errors"
	"net"
	"strings"
)

// ErrClosed is returned by Query, queryTagged, Listen and Subscribe once the
// LyrionServer has been closed.
var ErrClosed = errors.New("lyrionapi: server connection closed")

type pendingRequest struct {
	resp chan rawResult
}

type rawResult struct {
	line string
	err  error
}

// doQueryRaw writes command on server.conn and returns the raw (still
// percent-encoded), newline-stripped response line. It is safe to call
// concurrently: writes are serialized and responses are matched to requests
// strictly in send order via a FIFO queue drained by a single shared reader
// goroutine.
func (server *LyrionServer) doQueryRaw(command string) (string, error) {
	if server.closed.Load() {
		return "", ErrClosed
	}

	server.startReaderOnce.Do(server.startReader)

	req := &pendingRequest{resp: make(chan rawResult, 1)}

	server.mu.Lock()
	server.pending = append(server.pending, req)
	_, err := server.conn.Write([]byte(command + "\n"))
	if err != nil {
		server.pending = server.pending[:len(server.pending)-1]
	}
	server.mu.Unlock()

	if err != nil {
		return "", err
	}

	result := <-req.resp
	return result.line, result.err
}

func (server *LyrionServer) startReader() {
	go func() {
		for {
			line, err := readRawLine(server.conn)

			server.mu.Lock()
			var req *pendingRequest
			if len(server.pending) > 0 {
				req = server.pending[0]
				server.pending = server.pending[1:]
			}
			server.mu.Unlock()

			if req != nil {
				req.resp <- rawResult{line: line, err: err}
			}

			if err != nil {
				server.failPending(err)
				return
			}
		}
	}()
}

// failPending delivers err to every request still waiting for a response,
// e.g. once the connection has failed or been closed.
func (server *LyrionServer) failPending(err error) {
	server.mu.Lock()
	pending := server.pending
	server.pending = nil
	server.mu.Unlock()

	for _, req := range pending {
		req.resp <- rawResult{err: err}
	}
}

// Notification is a single unsolicited event pushed by the server to a
// connection that has called Listen or Subscribe.
type Notification struct {
	// PlayerID is set when the notification is scoped to a specific player
	// (its raw id, typically a MAC address); empty for server-wide events
	// such as "rescan done" or "favorites changed".
	PlayerID string
	// Verb is the notification's command word, e.g. "playlist", "mixer",
	// "client", "alarm", "prefset", "rescan", "favorites".
	Verb string
	// Args holds the remaining decoded tokens, whose meaning is verb-specific.
	Args []string
	// Raw is the original, undecoded line, for notification shapes not
	// otherwise modeled by PlayerID/Verb/Args.
	Raw string
}

type subscriber struct {
	ch    chan Notification
	verbs map[string]bool // nil means "all verbs"
}

// Listen opens the dedicated notification connection (if not already open)
// and enables server push notifications on it. It is idempotent.
func (server *LyrionServer) Listen() error {
	server.notifyMu.Lock()
	defer server.notifyMu.Unlock()
	return server.ensureNotifyConnLocked()
}

// Subscribe registers a new channel of notifications, optionally filtered to
// only the given verbs (all verbs, if none are given). The returned func
// unsubscribes and closes the channel; callers must call it to avoid leaking
// the subscription once they're done reading from the channel.
func (server *LyrionServer) Subscribe(verbs ...string) (<-chan Notification, func(), error) {
	server.notifyMu.Lock()
	if err := server.ensureNotifyConnLocked(); err != nil {
		server.notifyMu.Unlock()
		return nil, nil, err
	}

	sub := &subscriber{ch: make(chan Notification, 32)}
	if len(verbs) > 0 {
		sub.verbs = make(map[string]bool, len(verbs))
		for _, verb := range verbs {
			sub.verbs[verb] = true
		}
	}
	server.subscribers = append(server.subscribers, sub)
	server.notifyMu.Unlock()

	unsubscribe := func() {
		server.notifyMu.Lock()
		for i, s := range server.subscribers {
			if s == sub {
				server.subscribers = append(server.subscribers[:i], server.subscribers[i+1:]...)
				break
			}
		}
		server.notifyMu.Unlock()
		close(sub.ch)
	}

	return sub.ch, unsubscribe, nil
}

// ensureNotifyConnLocked must be called with notifyMu held.
func (server *LyrionServer) ensureNotifyConnLocked() error {
	if server.notifyConn != nil {
		return nil
	}
	if server.closed.Load() {
		return ErrClosed
	}

	conn, err := net.Dial("tcp", server.address)
	if err != nil {
		return err
	}
	if _, err := conn.Write([]byte("listen 1\n")); err != nil {
		conn.Close()
		return err
	}
	// Discard the echo of "listen 1" itself, so subscribers only ever see
	// genuine unsolicited notifications.
	if _, err := readRawLine(conn); err != nil {
		conn.Close()
		return err
	}

	server.notifyConn = conn
	go server.notifyReader(conn)
	return nil
}

// notifyReader is the sole reader of a notifyConn: since that connection
// never issues Query/queryTagged, every line it reads is unambiguously an
// unsolicited notification, so no request/response demuxing is needed here.
func (server *LyrionServer) notifyReader(conn net.Conn) {
	for {
		line, err := readRawLine(conn)
		if err != nil {
			server.notifyMu.Lock()
			subs := server.subscribers
			server.subscribers = nil
			if server.notifyConn == conn {
				server.notifyConn = nil
			}
			server.notifyMu.Unlock()

			for _, s := range subs {
				close(s.ch)
			}
			return
		}

		notification, err := parseNotification(line)
		if err != nil {
			continue
		}

		server.notifyMu.Lock()
		subs := server.subscribers
		server.notifyMu.Unlock()

		for _, s := range subs {
			if s.verbs != nil && !s.verbs[notification.Verb] {
				continue
			}
			select {
			case s.ch <- notification:
			default:
				// Slow subscriber: drop rather than block the shared reader.
			}
		}
	}
}

func parseNotification(raw string) (Notification, error) {
	tokens, err := tokenize(raw)
	if err != nil {
		return Notification{}, err
	}
	if len(tokens) == 0 {
		return Notification{}, errors.New("lyrionapi: empty notification")
	}

	notification := Notification{Raw: raw}
	if strings.Contains(tokens[0], ":") {
		// Player IDs are MAC addresses (or an IP for streaming players);
		// verbs are plain words, so a colon in the first token marks a
		// player-scoped notification.
		notification.PlayerID = tokens[0]
		tokens = tokens[1:]
	}
	if len(tokens) > 0 {
		notification.Verb = tokens[0]
		notification.Args = tokens[1:]
	}
	return notification, nil
}
