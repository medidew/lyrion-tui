package localplayer

import (
	"bytes"
	"encoding/binary"
	"io"
	"os"
	"regexp"
	"testing"
	"time"
)

func TestID(t *testing.T) {
	id := ID()
	if id != ID() {
		t.Fatal("ID is not stable")
	}
	if !regexp.MustCompile(`^02(:[0-9a-f]{2}){5}$`).MatchString(id) {
		t.Fatalf("ID %q is not a locally-administered MAC-style address", id)
	}
}

func TestStreamURL(t *testing.T) {
	got := StreamURL("192.168.1.4", 9000, "02:aa:bb:cc:dd:ee")
	want := "http://192.168.1.4:9000/stream.mp3?player=02%3Aaa%3Abb%3Acc%3Add%3Aee"
	if got != want {
		t.Fatalf("got %s, want %s", got, want)
	}
}

// fakeFrame builds a Layer III frame (header + zero padding) of the length
// its header implies.
func fakeFrame(t *testing.T, header [4]byte) []byte {
	t.Helper()
	_, length, ok := parseFrameHeader(header[:])
	if !ok {
		t.Fatalf("invalid test header % x", header)
	}
	frame := make([]byte, length)
	copy(frame, header[:])
	return frame
}

var (
	header44k = [4]byte{0xFF, 0xFB, 0x90, 0x00} // MPEG-1, 128 kbps, 44.1 kHz
	header48k = [4]byte{0xFF, 0xFB, 0x94, 0x00} // MPEG-1, 128 kbps, 48 kHz
	header22k = [4]byte{0xFF, 0xF3, 0x90, 0x00} // MPEG-2, 80 kbps, 22.05 kHz
)

func TestParseFrameHeader(t *testing.T) {
	cases := []struct {
		header       [4]byte
		rate, length int
	}{
		{header44k, 44100, 417},
		{header48k, 48000, 384},
		{header22k, 22050, 261},
	}
	for _, c := range cases {
		rate, length, ok := parseFrameHeader(c.header[:])
		if !ok || rate != c.rate || length != c.length {
			t.Errorf("% x: got rate %d length %d ok %v, want %d %d", c.header, rate, length, ok, c.rate, c.length)
		}
	}
	if _, _, ok := parseFrameHeader([]byte{0xFF, 0xFD, 0x90, 0x00}); ok { // Layer II
		t.Error("accepted a Layer II header")
	}
}

func TestSegmentsSplitOnRateChange(t *testing.T) {
	var stream bytes.Buffer
	stream.Write([]byte("junk"))
	for _, header := range [][4]byte{header44k, header44k, header48k, header22k, header22k} {
		stream.Write(fakeFrame(t, header))
	}

	frames := newFrameReader(&stream)
	var rates []int
	var sizes []int
	for {
		first, rate, err := frames.next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		frames.unread(first, rate)
		seg := &segment{frames: frames, rate: rate}
		data, _ := io.ReadAll(seg)
		rates = append(rates, rate)
		sizes = append(sizes, len(data))
		if seg.err != nil {
			break
		}
	}

	wantRates := []int{44100, 48000, 22050}
	wantSizes := []int{2 * 417, 384, 2 * 261}
	if len(rates) != 3 {
		t.Fatalf("got segments with rates %v, want %v", rates, wantRates)
	}
	for i := range wantRates {
		if rates[i] != wantRates[i] || sizes[i] != wantSizes[i] {
			t.Fatalf("got rates %v sizes %v, want %v %v", rates, sizes, wantRates, wantSizes)
		}
	}
}

func pcm(frames int, value func(i int) int16) []byte {
	buf := make([]byte, frames*bytesPerFrame)
	for i := range frames {
		binary.LittleEndian.PutUint16(buf[i*4:], uint16(value(i)))
		binary.LittleEndian.PutUint16(buf[i*4+2:], uint16(-value(i)))
	}
	return buf
}

func TestResamplerLengthAndInterpolation(t *testing.T) {
	// 480 frames at 48 kHz is 10ms, which is 441 frames at 44.1 kHz.
	in := pcm(480, func(i int) int16 { return int16(i * 10) })
	out, err := io.ReadAll(newResampler(bytes.NewReader(in), 48000, 44100))
	if err != nil {
		t.Fatal(err)
	}
	if frames := len(out) / bytesPerFrame; frames < 440 || frames > 441 {
		t.Fatalf("got %d output frames, want ~441", frames)
	}
	// A linear ramp stays a linear ramp: output frame k sits at source
	// position k*48000/44100.
	for _, k := range []int{0, 100, 300} {
		left := int16(binary.LittleEndian.Uint16(out[k*4:]))
		want := int16(k * 48000 * 10 / 44100)
		if diff := left - want; diff < -1 || diff > 1 {
			t.Errorf("frame %d: got %d, want %d", k, left, want)
		}
		right := int16(binary.LittleEndian.Uint16(out[k*4+2:]))
		if diff := right + want; diff < -1 || diff > 1 {
			t.Errorf("frame %d right channel: got %d, want %d", k, right, -want)
		}
	}
}

func TestPCMBuffer(t *testing.T) {
	buffer := newPCMBuffer(8)

	// An empty buffer reads 0 bytes without blocking.
	if n, err := buffer.Read(make([]byte, 8)); n != 0 || err != nil {
		t.Fatalf("empty read: got %d, %v", n, err)
	}

	// Reads hand out whole 4-byte frames only.
	buffer.Write([]byte{1, 2, 3, 4, 5, 6})
	out := make([]byte, 8)
	if n, _ := buffer.Read(out); n != 4 {
		t.Fatalf("got %d bytes, want 4", n)
	}
	// Completing the partial frame makes it readable.
	buffer.Write([]byte{7, 8})
	if n, _ := buffer.Read(out); n != 4 || !bytes.Equal(out[:4], []byte{5, 6, 7, 8}) {
		t.Fatalf("got % x, want 05 06 07 08", out[:n])
	}

	// A write that doesn't fit blocks until a read makes room...
	written := make(chan struct{})
	go func() {
		buffer.Write(make([]byte, 12))
		close(written)
	}()
	select {
	case <-written:
		t.Fatal("write into a full buffer didn't block")
	case <-time.After(50 * time.Millisecond):
	}
	buffer.Read(make([]byte, 8))
	select {
	case <-written:
	case <-time.After(time.Second):
		t.Fatal("write stayed blocked after reads made room")
	}

	// ...and Close unblocks a pending write with an error.
	buffer.Write(make([]byte, 4)) // fills it: 4 left over + 4
	failed := make(chan error)
	go func() {
		_, err := buffer.Write(make([]byte, 4))
		failed <- err
	}()
	buffer.Close()
	if err := <-failed; err == nil {
		t.Fatal("write after Close succeeded")
	}
}

// TestDecodeStreamKeepsSpeedAcrossRateChanges decodes a fixture of 1 kHz sine
// segments at 48, 22.05 and 44.1 kHz (as an LMS stream switches rate between
// tracks), with a 0.5s MPEG-2.5 (11.025 kHz) segment - which go-mp3 can't
// decode - before the last one. The output must be a 1 kHz tone throughout,
// with the undecodable segment skipped and decoding carrying on after it.
// Decoding a segment at the wrong rate would shift its pitch (to ~919 Hz or
// ~2 kHz) and change the total length.
func TestDecodeStreamKeepsSpeedAcrossRateChanges(t *testing.T) {
	stream, err := os.Open("testdata/mixed-rates.mp3")
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()

	var out bytes.Buffer
	if err := decodeStream(stream, &out); err != io.EOF {
		t.Fatalf("decodeStream: %v", err)
	}

	samples := make([]int16, out.Len()/bytesPerFrame)
	for i := range samples {
		samples[i] = int16(binary.LittleEndian.Uint16(out.Bytes()[i*bytesPerFrame:])) // left channel
	}

	// Three 1s segments (the MPEG-2.5 one is skipped), plus up to a few
	// frames of encoder padding each.
	if seconds := float64(len(samples)) / sampleRate; seconds < 3 || seconds > 3.25 {
		t.Fatalf("decoded %.2fs of audio, want ~3s", seconds)
	}

	const window = sampleRate / 10
	checked := 0
	for start := 0; start+window <= len(samples); start += window {
		part := samples[start : start+window]
		if hasQuietGap(part) {
			continue // encoder delay/padding at a segment boundary
		}
		crossings := 0
		for i := 1; i < len(part); i++ {
			if part[i-1] < 0 && part[i] >= 0 {
				crossings++
			}
		}
		hz := float64(crossings) * sampleRate / window
		if hz < 980 || hz > 1020 {
			t.Errorf("window at %.1fs: %.0f Hz, want ~1000", float64(start)/sampleRate, hz)
		}
		checked++
	}
	if checked < 20 {
		t.Fatalf("only %d windows were loud enough to check", checked)
	}
}

// hasQuietGap reports whether any 2ms stretch of part is near-silent.
func hasQuietGap(part []int16) bool {
	const block = sampleRate / 500
	for start := 0; start+block <= len(part); start += block {
		peak := 0
		for _, s := range part[start : start+block] {
			peak = max(peak, abs(int(s)))
		}
		if peak < 1000 {
			return true
		}
	}
	return false
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func TestPCMBufferInterruptAndReset(t *testing.T) {
	buffer := newPCMBuffer(8)
	buffer.Write(make([]byte, 8))
	buffer.Read(make([]byte, 4))
	if got := buffer.Consumed(); got != 4 {
		t.Fatalf("Consumed = %d, want 4", got)
	}

	// Interrupt releases a writer blocked on a full buffer (as when output
	// is paused during a Restart)...
	failed := make(chan error)
	go func() {
		_, err := buffer.Write(make([]byte, 8))
		failed <- err
	}()
	time.Sleep(20 * time.Millisecond)
	buffer.Interrupt()
	select {
	case err := <-failed:
		if err != errInterrupted {
			t.Fatalf("blocked write returned %v, want errInterrupted", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Interrupt didn't release the blocked write")
	}

	// ...and Reset empties it, clears the interrupt and the Consumed count.
	buffer.Reset()
	if n, _ := buffer.Read(make([]byte, 8)); n != 0 || buffer.Consumed() != 0 {
		t.Fatalf("after Reset: read %d bytes, Consumed %d", n, buffer.Consumed())
	}
	if _, err := buffer.Write(make([]byte, 4)); err != nil {
		t.Fatalf("write after Reset: %v", err)
	}
}
