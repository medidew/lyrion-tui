# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

A terminal UI (TUI) client for a Lyrion Music Server (LMS, formerly Logitech Media Server / Squeezebox Server), written in Go. It talks to LMS over its plaintext Telnet-style CLI protocol (default port 9090). The TUI itself (`tview`) is not wired up yet — `main.go` currently just exercises the `internal` package directly.

## Commands

- Build: `go build ./...`
- Run: `go run .`
- Test: `go test ./...` (single test: `go test ./internal -run TestConnection`)
- Format: `gofmt -l .` / `gofmt -w .`
- Vet: `go vet ./...`

`internal/protocol_test.go` is a plain unit test suite (fixture strings, no network) and always runs. `internal/lms_test.go`'s `TestConnection` additionally requires a live LMS server reachable at the hardcoded address `192.168.1.4:9090` — there is no mock/fake server, so that one test will fail without one on the network. This same address is hardcoded in `main.go`.

## Architecture

The `internal` package (import path `lyrionapi "github.com/medidew/lyrion-tui/internal"`) is split by concern, one file per LMS CLI command category:

- `internal/lms.go` — `LyrionServer` struct, `Connect`/`Close`, and the two query primitives (`Query`, `queryTagged`) everything else is built on.
- `internal/protocol.go` — pure parsing: raw line reading, tokenizing, and the tagged-response record parser. No I/O/concurrency.
- `internal/notify.go` — connection lifecycle: the request/response correlation layer behind `Query`/`queryTagged`, plus `Listen`/`Subscribe` and the `Notification` type.
- `internal/library.go` — genres/artists/albums/years/titles/songinfo/search/musicfolder, plus `TotalGenres`/`TotalArtists`/`TotalAlbums`/`TotalSongs`.
- `internal/player.go` — `LyrionPlayer` struct, `GetPlayer`/`GetPlayerCount`, and per-player identity/hardware/mixer/sync control.
- `internal/playlist.go` — all playback and playlist manipulation on `*LyrionPlayer` (play/stop/pause, current song, shuffle/repeat, saved playlists, `playlistcontrol`).
- `internal/favorites.go`, `internal/randomplay.go`, `internal/alarms.go` — those command groups.
- `internal/status.go` — `Players`/`SyncGroups`/`Libraries`/`Version`/`Can`/`ReadDirectory`, plus `ServerStatus`/`Status` (see caveat below).

Deliberately out of scope: server-admin/maintenance commands (`rescan`, `abortscan`, `wipecache`, `pragma`, `debug`, `logging`, `login`, `pref`, `artworkspec`, `getstring`, `stopserver`, `restartserver`) — this is a player client, not a server admin tool.

### Query/response model

Two connections are maintained per `LyrionServer`: `conn` for request/response commands, and a lazily-opened `notifyConn` used only for push notifications (see below). LMS notification lines are textually indistinguishable from ordinary command echoes, and the server only ever pushes them to a connection that has sent `listen`/`subscribe` — keeping `conn` free of that means a response on `conn` can never be misread as an unrelated notification.

`Query`/`queryTagged` share a request/response correlation layer (`doQueryRaw` in `notify.go`): writes are serialized under a mutex that also enqueues a per-request channel in a FIFO, and a single reader goroutine drains `conn` and delivers each line to the oldest still-pending request. This makes `Query`/`queryTagged` **safe to call concurrently** from multiple goroutines on the same `LyrionServer`.

LMS echoes the request as a literal prefix of the response (e.g. querying `"player count ?"` returns `"player count 7"`). Two parsing styles exist depending on the response shape:

- **Scalar responses** (`Query`): decode the whole line, then slice off the known-length echoed prefix.
  ```go
  request := fmt.Sprintf("%v mode ?", player.id)
  state, err := player.server.Query(request)
  state = state[len(request)-1:]
  ```
  `player.queryField`/`queryIntField`/`queryFloatField`/`queryBoolField` in `playlist.go` wrap this pattern and handle the case where LMS returns a response *shorter* than the echoed prefix (nothing to report, e.g. querying the current title while stopped) as an empty/zero value rather than a panic — any new scalar accessor should go through them rather than slicing by hand.
- **Tagged, possibly multi-item responses** (`queryTagged`, in `lms.go`): count is `N` + a flat run of repeated `tag:value` groups, one group per item, with a *repeated key* (not an explicit separator) marking where the next record starts. `queryTagged` tokenizes the **raw, still percent-encoded** line on whitespace and decodes each token individually — decoding the whole line first (as `Query` does) would turn an encoded space (`%20`) inside a value into something indistinguishable from a real token separator, corrupting record boundaries. Domain files add typed struct mappers on top (e.g. `albumFromRecord` in `library.go`) using the `tagString`/`tagInt`/`tagFloat`/`tagBool` helpers in `protocol.go`.

  **Field-naming gotcha, confirmed against a live LMS 9.1.1 server**: the reference docs describe *response* fields using single-letter codes (e.g. `l` for album title, `a` for artist). In practice the server labels returned fields with full descriptive names regardless (`album`, `artist`, `artwork_track_id`, `tracknum`, ...) — the letter codes are only used in the *request's* `tags:` string to select which fields come back. Verify against a raw capture before trusting a docs-only field name; `search`'s response mixes four different id/name field-name pairs by category rather than one consistent shape, so it's parsed by hand in `library.go` (`Search`) instead of through the generic tagged-record parser.

### Notifications

`Listen()`/`Subscribe(verbs...)` open `notifyConn` on first use, send `listen 1`, and discard that command's own echo before starting a dedicated reader goroutine (otherwise every subscriber would see a spurious first "notification" that's really just LMS echoing `listen 1` back). Every line read after that is unambiguously a real notification, parsed into a `Notification{PlayerID, Verb, Args, Raw}` and fanned out non-blockingly (drop, not block, on a full subscriber channel — a slow consumer must never stall the shared reader). `Subscribe` always requests everything (`listen 1`) and filters per-subscriber client-side, rather than using the CLI's own `subscribe <list>`, since that's connection-scoped ("last one wins") and can't support multiple independent Go subscribers with different filters over one shared connection.

### Known gaps

- `ServerStatus`/`Status` (`status.go`) compose several already-verified single-purpose queries rather than parsing the compound `serverstatus`/`status` CLI commands directly — those commands' own nested response shape (a top-level block, then a per-player loop, then for `status` a further nested playlist-track loop) wasn't confirmed against a live server and doesn't fit the flat tagged-record parser. A future revision could fold this into one round trip once that's verified.
- `DirEntry` (`ReadDirectory`) and parts of `FolderItem` (`GetMusicFolder`) use field names extrapolated from the confirmed naming convention above but not directly verified against a live response.
- The `tview` UI in `main.go` is commented out; there is no actual TUI screen implemented.
- The LMS test server address should eventually move to config/env instead of being hardcoded in two places.
