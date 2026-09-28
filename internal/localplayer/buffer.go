package localplayer

import (
	"errors"
	"io"
	"sync"
)

// errInterrupted is what a Write blocked in an interrupted buffer returns.
var errInterrupted = errors.New("localplayer: buffer interrupted")

// pcmBuffer decouples the network/decoder side from oto. oto's mixer calls
// every source's Read synchronously on one shared loop, so a source that
// blocks on a stalled network read (e.g. while the server is paused) would
// stall the mixer and make Close hang. Instead the decoder goroutine writes
// here - blocking when the buffer is full, which applies TCP backpressure to
// the server - and oto reads here without ever blocking: an empty buffer
// reads as 0 bytes, which oto treats as "nothing yet", not end of stream.
type pcmBuffer struct {
	mu          sync.Mutex
	notFull     *sync.Cond
	data        []byte
	capacity    int
	closed      bool
	interrupted bool  // Writes fail until the next Reset
	consumed    int64 // bytes handed to oto since the last Reset
}

func newPCMBuffer(capacity int) *pcmBuffer {
	b := &pcmBuffer{capacity: capacity}
	b.notFull = sync.NewCond(&b.mu)
	return b
}

// Write blocks until all of p fits or the buffer is closed or interrupted.
func (b *pcmBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	written := 0
	for written < len(p) {
		for len(b.data) >= b.capacity && !b.closed && !b.interrupted {
			b.notFull.Wait()
		}
		if b.closed {
			return written, io.ErrClosedPipe
		}
		if b.interrupted {
			return written, errInterrupted
		}
		n := min(b.capacity-len(b.data), len(p)-written)
		b.data = append(b.data, p[written:written+n]...)
		written += n
	}
	return written, nil
}

// Read never blocks. It only hands out whole 4-byte frames (16-bit stereo),
// so a short read can't leave oto misaligned mid-sample.
func (b *pcmBuffer) Read(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	n := min(len(p), len(b.data))
	n -= n % bytesPerFrame
	copy(p, b.data[:n])
	b.data = b.data[n:]
	b.consumed += int64(n)
	if n > 0 {
		b.notFull.Broadcast()
	}
	return n, nil
}

// Flush discards everything buffered but not yet handed to oto.
func (b *pcmBuffer) Flush() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.data = b.data[:0]
	b.notFull.Broadcast()
}

// Interrupt makes any pending or future Write fail until the next Reset, so
// a writer blocked on a full buffer (e.g. while output is paused) can be
// stopped.
func (b *pcmBuffer) Interrupt() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.interrupted = true
	b.notFull.Broadcast()
}

// Reset flushes the buffer, clears an Interrupt, and restarts the Consumed
// count.
func (b *pcmBuffer) Reset() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.data = b.data[:0]
	b.interrupted = false
	b.consumed = 0
	b.notFull.Broadcast()
}

// Consumed returns how many bytes have been read since the last Reset.
func (b *pcmBuffer) Consumed() int64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.consumed
}

// Close unblocks any pending Write, permanently.
func (b *pcmBuffer) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.closed = true
	b.notFull.Broadcast()
}
