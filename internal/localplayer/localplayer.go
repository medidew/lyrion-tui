// Package localplayer plays an LMS HTTP audio stream (/stream.mp3) through
// this machine's speakers, which makes the process an LMS "player" in its own
// right. It knows nothing about the LMS CLI or the TUI: callers give it a
// stream URL and control playback on the server as with any other player.
package localplayer

import (
	"context"
	"crypto/sha1"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"sync"
	"time"

	"github.com/ebitengine/oto/v3"
)

const (
	// sampleRate is the one rate oto's (process-wide, single) context runs
	// at; streams at other rates go through a resampler.
	sampleRate    = 44100
	bytesPerFrame = 4 // 16-bit little-endian stereo, which go-mp3 always emits

	// bufferDuration bounds how much decoded audio sits between the network
	// and the speakers. Smaller means server-side skips/stops are heard
	// sooner; larger rides out more network jitter.
	bufferDuration = 500 * time.Millisecond

	// socketBufferSize caps the TCP receive buffer (see dialSmallBuffer):
	// 16 KiB, which the kernel doubles, is ~1s of 256 kbps MP3.
	socketBufferSize = 16 * 1024

	connectTimeout   = 5 * time.Second
	reconnectBackoff = time.Second
)

// ID returns a stable player ID for this machine, shaped like a MAC address
// (as LMS player IDs are). The first octet 02 marks it as locally
// administered, so it can't collide with real hardware. It must be passed to
// the stream explicitly: without one, LMS keys the player by client IP,
// which would take over any browser on this machine that's already using
// /stream.mp3.
func ID() string {
	host, err := os.Hostname()
	if err != nil {
		host = "localhost"
	}
	sum := sha1.Sum([]byte("lyrion-tui:" + host))
	return fmt.Sprintf("02:%02x:%02x:%02x:%02x:%02x", sum[0], sum[1], sum[2], sum[3], sum[4])
}

// StreamURL builds the /stream.mp3 URL for playerID on the LMS web server at
// host:webPort.
func StreamURL(host string, webPort int, playerID string) string {
	u := url.URL{
		Scheme:   "http",
		Host:     net.JoinHostPort(host, fmt.Sprint(webPort)),
		Path:     "/stream.mp3",
		RawQuery: url.Values{"player": {playerID}}.Encode(),
	}
	return u.String()
}

var (
	otoOnce    sync.Once
	otoContext *oto.Context
	otoErr     error
)

// audioContext lazily creates the process-wide oto context (oto supports
// only one) and waits for the audio device to be ready.
func audioContext() (*oto.Context, error) {
	otoOnce.Do(func() {
		ctx, ready, err := oto.NewContext(&oto.NewContextOptions{
			SampleRate:      sampleRate,
			ChannelCount:    2,
			Format:          oto.FormatSignedInt16LE,
			ApplicationName: "lyrion-tui",
		})
		if err != nil {
			otoErr = err
			return
		}
		<-ready
		if err := ctx.Err(); err != nil {
			otoErr = err
			return
		}
		otoContext = ctx
	})
	if otoErr != nil {
		return nil, fmt.Errorf("localplayer: no audio output: %w", otoErr)
	}
	return otoContext, nil
}

// dialSmallBuffer dials with a small socket receive buffer. LMS doesn't pace
// the stream - it sends as fast as the client reads - so every buffer between
// it and the speakers is audio the server has already moved past. Linux
// would autotune this one to megabytes (minutes of audio). The server's own
// send-side buffering (several seconds, more at low bitrates) can't be
// limited from here; Restart discards it when that matters.
func dialSmallBuffer(ctx context.Context, network, address string) (net.Conn, error) {
	conn, err := (&net.Dialer{Timeout: connectTimeout}).DialContext(ctx, network, address)
	if err != nil {
		return nil, err
	}
	if tcp, ok := conn.(*net.TCPConn); ok {
		tcp.SetReadBuffer(socketBufferSize)
	}
	return conn, nil
}

// Player streams one URL to the speakers until Close, reconnecting if the
// stream drops.
type Player struct {
	url    string
	client *http.Client
	buffer *pcmBuffer
	output *oto.Player

	ctx      context.Context
	cancel   context.CancelFunc
	done     chan struct{}
	restarts chan restartRequest

	mu         sync.Mutex
	connCancel context.CancelFunc // cancels the current connection
	volume     int
	lastErr    error
}

type restartRequest struct {
	whileDisconnected func()
	done              chan error
}

// Start opens the audio device and connects to streamURL, returning an error
// if either fails. After that, dropped connections are retried in the
// background until Close.
func Start(streamURL string) (*Player, error) {
	audio, err := audioContext()
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())
	player := &Player{
		url: streamURL,
		client: &http.Client{Transport: &http.Transport{
			DialContext:           dialSmallBuffer,
			ResponseHeaderTimeout: connectTimeout,
		}},
		buffer:   newPCMBuffer(int(bufferDuration.Seconds() * sampleRate * bytesPerFrame)),
		ctx:      ctx,
		cancel:   cancel,
		done:     make(chan struct{}),
		restarts: make(chan restartRequest),
		volume:   100,
	}

	first, err := player.connect()
	if err != nil {
		cancel()
		return nil, err
	}

	player.output = audio.NewPlayer(player.buffer)
	// oto's own buffer sits after ours; keep it short too.
	player.output.SetBufferSize(int(0.1 * sampleRate * bytesPerFrame))
	player.output.Play()

	go player.run(first)
	return player, nil
}

// connect issues the stream request, returning the response on success. The
// connection can be cut short with cancelConn.
func (player *Player) connect() (*http.Response, error) {
	ctx, cancel := context.WithCancel(player.ctx)
	player.mu.Lock()
	player.connCancel = cancel
	player.mu.Unlock()

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, player.url, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("User-Agent", "lyrion-tui")

	response, err := player.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("localplayer: connecting to %s: %w", player.url, err)
	}
	if response.StatusCode != http.StatusOK {
		response.Body.Close()
		return nil, fmt.Errorf("localplayer: %s returned %s", player.url, response.Status)
	}
	return response, nil
}

func (player *Player) cancelConn() {
	player.mu.Lock()
	cancel := player.connCancel
	player.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// run decodes the current connection into the buffer until it ends, then
// reconnects - at once for a Restart, else after a backoff - until Close.
func (player *Player) run(response *http.Response) {
	defer close(player.done)

	for {
		if response != nil {
			err := player.decode(response)
			response.Body.Close()
			player.setErr(err)
		}

		select {
		case <-player.ctx.Done():
			return
		case request := <-player.restarts:
			player.buffer.Reset()
			request.whileDisconnected()
			var err error
			response, err = player.connect()
			player.setErr(err)
			request.done <- err
			continue
		case <-time.After(reconnectBackoff):
		}

		var err error
		response, err = player.connect()
		player.setErr(err)
	}
}

// Restart discards everything buffered between the server and the speakers
// by dropping the connection, calls whileDisconnected, then reconnects.
//
// This is how a new song gets heard promptly: LMS keeps seconds of audio
// queued on its side of the connection that it has already moved past, so
// without this a newly selected song would only be heard once that stale
// audio had played out. The order matters: a command that changes the song
// must be sent from whileDisconnected, after LMS has noticed the old
// connection is gone - otherwise LMS starts sending the new song into the
// dying connection, and its start is lost with it.
func (player *Player) Restart(whileDisconnected func()) error {
	request := restartRequest{whileDisconnected: whileDisconnected, done: make(chan error, 1)}
	player.cancelConn()
	player.buffer.Interrupt() // in case the decoder is blocked on a full buffer
	select {
	case player.restarts <- request:
	case <-player.ctx.Done():
		return errors.New("localplayer: closed")
	}
	return <-request.done
}

// decode feeds one connection's audio into the buffer. It returns when the
// stream ends or fails; a clean end (or shutting down, or a Restart) returns
// nil.
func (player *Player) decode(response *http.Response) error {
	err := decodeStream(response.Body, player.buffer)
	if player.ctx.Err() != nil || errors.Is(err, io.EOF) || errors.Is(err, io.ErrClosedPipe) ||
		errors.Is(err, context.Canceled) || errors.Is(err, errInterrupted) {
		return nil
	}
	return fmt.Errorf("localplayer: stream: %w", err)
}

// Played returns how much audio has been played since the last Restart
// (paused time excluded).
func (player *Player) Played() time.Duration {
	bytes := player.buffer.Consumed() - int64(player.output.BufferedSize())
	return max(0, time.Duration(bytes)*time.Second/(sampleRate*bytesPerFrame))
}

func (player *Player) setErr(err error) {
	player.mu.Lock()
	defer player.mu.Unlock()
	player.lastErr = err
}

// Err reports the most recent stream error, or nil if the last connection
// attempt succeeded.
func (player *Player) Err() error {
	player.mu.Lock()
	defer player.mu.Unlock()
	return player.lastErr
}

// Volume returns the local output volume, 0-100.
func (player *Player) Volume() int {
	player.mu.Lock()
	defer player.mu.Unlock()
	return player.volume
}

// SetVolume sets the local output volume, 0-100 (clamped). LMS's own volume
// control is a no-op for HTTP stream players, so volume is applied here. The
// curve is squared so that equal steps sound roughly equally loud.
func (player *Player) SetVolume(volume int) {
	volume = max(0, min(100, volume))
	player.mu.Lock()
	player.volume = volume
	player.mu.Unlock()

	level := float64(volume) / 100
	player.output.SetVolume(level * level)
}

// SetPaused pauses or resumes local output immediately. The server should be
// paused too (it owns the playlist position) - this just stops the audio
// already buffered locally from playing out after it.
func (player *Player) SetPaused(paused bool) {
	if paused {
		player.output.Pause()
	} else {
		player.output.Play()
	}
}

// Flush drops locally buffered audio, e.g. after a stop, so it isn't heard
// when playback next starts.
func (player *Player) Flush() {
	player.buffer.Flush()
}

// Close stops playback and disconnects.
func (player *Player) Close() {
	player.cancel()
	player.buffer.Close()
	<-player.done
	player.output.Close()
}
