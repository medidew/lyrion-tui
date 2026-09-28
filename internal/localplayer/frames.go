package localplayer

import (
	"bufio"
	"io"

	"github.com/hajimehoshi/go-mp3"
)

// The LMS stream is one continuous MP3 bitstream, but it's built track by
// track - some tracks passed through as-is, others transcoded - so its sample
// rate can change at a track boundary (e.g. a 22.05 kHz MP3 passed through
// between 44.1 kHz tracks). go-mp3 fixes the rate from a stream's first frame, so
// feeding it the whole stream would play everything after a change at the
// wrong speed. Instead the stream is walked frame by frame and cut into
// segments of constant sample rate, each decoded by a fresh decoder.

// Layer III bitrates in kbps, by bitrate index, for MPEG-1 and MPEG-2/2.5.
var (
	bitratesV1 = [15]int{0, 32, 40, 48, 56, 64, 80, 96, 112, 128, 160, 192, 224, 256, 320}
	bitratesV2 = [15]int{0, 8, 16, 24, 32, 40, 48, 56, 64, 80, 96, 112, 128, 144, 160}
)

// parseFrameHeader returns a Layer III frame's sample rate and total length
// in bytes, or ok=false if header isn't a valid Layer III frame header.
func parseFrameHeader(header []byte) (rate, length int, ok bool) {
	if header[0] != 0xFF || header[1]&0xE0 != 0xE0 {
		return 0, 0, false
	}
	version := (header[1] >> 3) & 3 // 3: MPEG-1, 2: MPEG-2, 0: MPEG-2.5, 1: reserved
	layer := (header[1] >> 1) & 3   // 1: Layer III
	bitrateIndex := header[2] >> 4
	rateIndex := (header[2] >> 2) & 3
	padding := int((header[2] >> 1) & 1)
	// Bitrate index 0 is "free format", which has no computable length.
	if version == 1 || layer != 1 || bitrateIndex == 0 || bitrateIndex == 15 || rateIndex == 3 {
		return 0, 0, false
	}

	rate = [3]int{44100, 48000, 32000}[rateIndex]
	if version == 3 {
		return rate, 144*bitratesV1[bitrateIndex]*1000/rate + padding, true
	}
	if version == 2 {
		rate /= 2
	} else {
		rate /= 4
	}
	return rate, 72*bitratesV2[bitrateIndex]*1000/rate + padding, true
}

// frameReader yields whole MP3 frames, skipping anything between them
// (tags, or junk after a glitch) by scanning for the next valid header.
type frameReader struct {
	r           *bufio.Reader
	pending     []byte // a frame read ahead by a segment, handed out next
	pendingRate int
}

func newFrameReader(r io.Reader) *frameReader {
	return &frameReader{r: bufio.NewReaderSize(r, 16*1024)}
}

func (f *frameReader) next() (frame []byte, rate int, err error) {
	if f.pending != nil {
		frame, rate = f.pending, f.pendingRate
		f.pending = nil
		return frame, rate, nil
	}
	for {
		header, err := f.r.Peek(4)
		if err != nil {
			return nil, 0, err
		}
		rate, length, ok := parseFrameHeader(header)
		if !ok {
			f.r.Discard(1)
			continue
		}
		frame = make([]byte, length)
		if _, err := io.ReadFull(f.r, frame); err != nil {
			if err == io.ErrUnexpectedEOF {
				err = io.EOF // the stream ended mid-frame
			}
			return nil, 0, err
		}
		return frame, rate, nil
	}
}

func (f *frameReader) unread(frame []byte, rate int) {
	f.pending, f.pendingRate = frame, rate
}

// segment reads frames from a frameReader for as long as they share one
// sample rate, then reports io.EOF - to the decoder, end of stream. err
// records why it ended: nil for a rate change (more segments follow), or
// the underlying stream's own error (io.EOF when the stream closed).
type segment struct {
	frames *frameReader
	rate   int
	buf    []byte
	err    error
	ended  bool
}

func (s *segment) done() bool {
	return s.ended && len(s.buf) == 0
}

// dropFrame discards what's left of the frame being read, so a decoder that
// failed on it can be restarted from the next frame.
func (s *segment) dropFrame() {
	s.buf = nil
}

func (s *segment) Read(p []byte) (int, error) {
	for len(s.buf) == 0 {
		if s.ended {
			return 0, io.EOF
		}
		frame, rate, err := s.frames.next()
		if err != nil {
			s.err, s.ended = err, true
			return 0, io.EOF
		}
		if rate != s.rate {
			s.frames.unread(frame, rate)
			s.ended = true
			return 0, io.EOF
		}
		s.buf = frame
	}
	n := copy(p, s.buf)
	s.buf = s.buf[n:]
	return n, nil
}

// decodeStream decodes an MP3 stream into 16-bit stereo PCM at sampleRate,
// written to out, one constant-rate segment at a time. It returns the error
// that ended the stream - io.EOF if it simply ended - or out's write error.
func decodeStream(stream io.Reader, out io.Writer) error {
	frames := newFrameReader(stream)
	chunk := make([]byte, 16*1024)

	for {
		first, rate, err := frames.next()
		if err != nil {
			return err
		}
		frames.unread(first, rate)
		seg := &segment{frames: frames, rate: rate}

		if err := decodeSegment(seg, rate, out, chunk); err != nil {
			return err
		}
		if seg.err != nil {
			return seg.err
		}
		// Otherwise the sample rate changed: carry on with a new segment.
	}
}

// decodeSegment decodes one constant-rate segment into out. If go-mp3 fails
// on a frame - it rejects MPEG-2.5 (8-12 kHz) outright, and a glitch could
// corrupt any frame - the rest of that frame is dropped and decoding
// restarts from the next one, rather than failing the whole stream (which
// would only reconnect into the same audio). An unplayable track is thereby
// passed over silently. Only a write error on out is returned.
func decodeSegment(seg *segment, rate int, out io.Writer, chunk []byte) error {
	for !seg.done() {
		decoder, err := mp3.NewDecoder(seg)
		if err != nil {
			seg.dropFrame()
			continue
		}
		var source io.Reader = decoder
		if rate != sampleRate {
			source = newResampler(decoder, rate, sampleRate)
		}

		for {
			n, err := source.Read(chunk)
			if n > 0 {
				if _, err := out.Write(chunk[:n]); err != nil {
					return err
				}
			}
			if err != nil {
				seg.dropFrame() // a no-op at the segment's end
				break
			}
		}
	}
	return nil
}
