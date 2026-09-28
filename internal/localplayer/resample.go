package localplayer

import (
	"encoding/binary"
	"io"
)

// resampler converts 16-bit little-endian stereo PCM from one sample rate to
// another by linear interpolation. oto's context runs at one fixed rate for
// the life of the process, so a stream at any other rate must be converted
// or it would play at the wrong speed and pitch.
type resampler struct {
	src      io.Reader
	fromRate int
	toRate   int

	// pos is the output position in source frames, as a fraction:
	// frame index pos/toRate. prev/next are the two source frames that the
	// current output frame is interpolated between.
	pos        int64
	prev, next [2]int16
	nextIndex  int64 // source frame index of next
	primed     bool
	eof        bool

	inBuf []byte
}

func newResampler(src io.Reader, fromRate, toRate int) *resampler {
	return &resampler{src: src, fromRate: fromRate, toRate: toRate}
}

// readFrame reads exactly one source frame.
func (r *resampler) readFrame() ([2]int16, error) {
	if len(r.inBuf) < bytesPerFrame {
		r.inBuf = make([]byte, bytesPerFrame)
	}
	if _, err := io.ReadFull(r.src, r.inBuf[:bytesPerFrame]); err != nil {
		return [2]int16{}, err
	}
	return [2]int16{
		int16(binary.LittleEndian.Uint16(r.inBuf[0:2])),
		int16(binary.LittleEndian.Uint16(r.inBuf[2:4])),
	}, nil
}

func (r *resampler) Read(p []byte) (int, error) {
	if !r.primed {
		first, err := r.readFrame()
		if err != nil {
			return 0, err
		}
		second, err := r.readFrame()
		if err != nil {
			second = first
		}
		r.prev, r.next, r.nextIndex, r.primed = first, second, 1, true
	}

	n := 0
	for n+bytesPerFrame <= len(p) {
		// Source position of this output frame, in units of 1/toRate frames.
		srcPos := r.pos * int64(r.fromRate)
		index := srcPos / int64(r.toRate)

		// Advance the source until next is the frame just after index.
		for r.nextIndex <= index {
			if r.eof {
				if n == 0 {
					return 0, io.EOF
				}
				return n, nil
			}
			frame, err := r.readFrame()
			if err != nil {
				r.eof = true
				if n == 0 {
					return 0, err
				}
				return n, nil
			}
			r.prev, r.next = r.next, frame
			r.nextIndex++
		}

		frac := srcPos - index*int64(r.toRate) // 0 <= frac < toRate
		for ch := range 2 {
			a, b := int64(r.prev[ch]), int64(r.next[ch])
			v := a + (b-a)*frac/int64(r.toRate)
			binary.LittleEndian.PutUint16(p[n+2*ch:], uint16(int16(v)))
		}
		n += bytesPerFrame
		r.pos++
	}
	return n, nil
}
