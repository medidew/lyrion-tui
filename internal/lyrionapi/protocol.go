package lyrionapi

import (
	"io"
	"net/url"
	"strconv"
	"strings"
)

// readRawLine reads bytes from r until (and excluding) a trailing newline.
// The returned line is not percent-decoded.
func readRawLine(r io.Reader) (string, error) {
	var line strings.Builder
	buffer := make([]byte, 1)
	for {
		if _, err := r.Read(buffer); err != nil {
			return "", err
		}
		if buffer[0] == '\n' {
			return line.String(), nil
		}
		line.WriteByte(buffer[0])
	}
}

// tokenize splits a raw (still percent-encoded) response line on whitespace
// and decodes each token individually. Decoding tokens individually, rather
// than decoding the whole line before splitting, matters: an encoded space
// (%20) inside a value would otherwise become indistinguishable from a real
// token separator once decoded.
func tokenize(raw string) ([]string, error) {
	fields := strings.Fields(raw)
	tokens := make([]string, len(fields))
	for i, field := range fields {
		decoded, err := url.QueryUnescape(field)
		if err != nil {
			return nil, err
		}
		tokens[i] = decoded
	}
	return tokens, nil
}

// countTokens returns how many whitespace-delimited tokens a command has, so
// that the same number of leading tokens (the echoed request) can be skipped
// in its response.
func countTokens(command string) int {
	return len(strings.Fields(command))
}

// sliceOrEmpty returns response[from:], or "" if response is shorter than
// from - guarding against LMS returning a response shorter than the echoed
// prefix (nothing to report), the same edge case queryField in playlist.go
// handles for player-scoped scalar queries.
func sliceOrEmpty(response string, from int) string {
	if from < 0 || from > len(response) {
		return ""
	}
	return response[from:]
}

// splitTag splits a decoded "key:value" token on the first colon. A token
// with no colon (shouldn't occur among the tag tokens passed to parseTagged)
// is reported via ok=false.
func splitTag(token string) (key string, value string, ok bool) {
	index := strings.Index(token, ":")
	if index < 0 {
		return "", "", false
	}
	return token[:index], token[index+1:], true
}

// parseTagged splits decoded tag tokens (as produced by tokenize, with the
// echoed request prefix already removed) into top-level metadata and a list
// of per-item records.
//
// Tokens whose key is in topLevelKeys are treated as metadata as long as no
// item record has started yet. Once a key outside topLevelKeys is seen, item
// parsing begins: a key repeating within the current record marks the start
// of a new record (this is how LMS delimits items in a tagged list, since
// there is no explicit item separator).
func parseTagged(tokens []string, topLevelKeys map[string]bool) (map[string]string, []map[string]string) {
	meta := map[string]string{}
	var items []map[string]string
	var current map[string]string

	for _, token := range tokens {
		key, value, ok := splitTag(token)
		if !ok {
			continue
		}

		if current == nil && topLevelKeys[key] {
			meta[key] = value
			continue
		}

		if _, seen := current[key]; current == nil || seen {
			current = map[string]string{}
			items = append(items, current)
		}
		current[key] = value
	}

	return meta, items
}

func tagString(record map[string]string, key string) string {
	return record[key]
}

func tagInt(record map[string]string, key string) int {
	value, ok := record[key]
	if !ok || value == "" {
		return 0
	}
	n, err := strconv.Atoi(value)
	if err != nil {
		return 0
	}
	return n
}

func tagFloat(record map[string]string, key string) float64 {
	value, ok := record[key]
	if !ok || value == "" {
		return 0
	}
	f, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0
	}
	return f
}

func tagBool(record map[string]string, key string) bool {
	return record[key] == "1"
}

// encodeArg percent-encodes a free-text argument for inclusion in an
// outgoing LMS command. LMS uses %20 for spaces (as seen in its own
// responses), not "+" as url.QueryEscape would produce for a form body.
func encodeArg(s string) string {
	return strings.ReplaceAll(url.QueryEscape(s), "+", "%20")
}
