package lyrionapi

import (
	"reflect"
	"strings"
	"testing"
)

func TestReadRawLine(t *testing.T) {
	line, err := readRawLine(strings.NewReader("player count 7\nnext line"))
	if err != nil {
		t.Fatalf("readRawLine returned error: %v", err)
	}
	if line != "player count 7" {
		t.Errorf("got %q, want %q", line, "player count 7")
	}
}

func TestTokenizeDecodesEachTokenIndividually(t *testing.T) {
	// "Living%20Room" must stay one token even though decoding it produces
	// an embedded space; a whole-line decode would have split it in two.
	tokens, err := tokenize("player name 0 Living%20Room")
	if err != nil {
		t.Fatalf("tokenize returned error: %v", err)
	}
	want := []string{"player", "name", "0", "Living Room"}
	if !reflect.DeepEqual(tokens, want) {
		t.Errorf("got %v, want %v", tokens, want)
	}
}

func TestCountTokens(t *testing.T) {
	if got := countTokens("genres 0 10 search:foo"); got != 4 {
		t.Errorf("got %d, want 4", got)
	}
}

func TestParseTaggedSeparatesMetaFromItems(t *testing.T) {
	tokens, err := tokenize("rescan:0 count:2 id:1 genre:Rock id:2 genre:Pop")
	if err != nil {
		t.Fatalf("tokenize returned error: %v", err)
	}

	meta, items := parseTagged(tokens, map[string]bool{"rescan": true, "count": true})

	if meta["rescan"] != "0" || meta["count"] != "2" {
		t.Errorf("unexpected meta: %v", meta)
	}
	want := []map[string]string{
		{"id": "1", "genre": "Rock"},
		{"id": "2", "genre": "Pop"},
	}
	if !reflect.DeepEqual(items, want) {
		t.Errorf("got %v, want %v", items, want)
	}
}

func TestParseTaggedRepeatedKeyStartsNewRecord(t *testing.T) {
	// No top-level keys at all this time: playerindex repeats to delimit records.
	tokens := []string{"playerindex:0", "name:Kitchen", "playerindex:1", "name:Living Room"}
	_, items := parseTagged(tokens, nil)

	want := []map[string]string{
		{"playerindex": "0", "name": "Kitchen"},
		{"playerindex": "1", "name": "Living Room"},
	}
	if !reflect.DeepEqual(items, want) {
		t.Errorf("got %v, want %v", items, want)
	}
}

func TestParseTaggedIgnoresTokensWithoutColon(t *testing.T) {
	// Simulates leftover positional tokens (start/itemsPerResponse) that a
	// caller forgot to skip - they should be ignored rather than corrupting
	// the first record.
	tokens := []string{"0", "10", "id:1", "genre:Rock"}
	meta, items := parseTagged(tokens, map[string]bool{"count": true})

	if len(meta) != 0 {
		t.Errorf("expected no meta, got %v", meta)
	}
	want := []map[string]string{{"id": "1", "genre": "Rock"}}
	if !reflect.DeepEqual(items, want) {
		t.Errorf("got %v, want %v", items, want)
	}
}

func TestTagHelpers(t *testing.T) {
	record := map[string]string{"id": "42", "name": "Kitchen", "y": "2004.5", "enabled": "1", "missing_bool": "0"}

	if got := tagString(record, "name"); got != "Kitchen" {
		t.Errorf("tagString: got %q", got)
	}
	if got := tagInt(record, "id"); got != 42 {
		t.Errorf("tagInt: got %d", got)
	}
	if got := tagInt(record, "name"); got != 0 {
		t.Errorf("tagInt on non-numeric: got %d, want 0", got)
	}
	if got := tagFloat(record, "y"); got != 2004.5 {
		t.Errorf("tagFloat: got %v", got)
	}
	if got := tagBool(record, "enabled"); got != true {
		t.Errorf("tagBool: got %v, want true", got)
	}
	if got := tagBool(record, "missing_bool"); got != false {
		t.Errorf("tagBool: got %v, want false", got)
	}
}

func TestEncodeArg(t *testing.T) {
	if got := encodeArg("Living Room"); got != "Living%20Room" {
		t.Errorf("got %q, want %q", got, "Living%20Room")
	}
}
