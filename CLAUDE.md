# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

A keyboard-driven terminal UI (TUI) client for a Lyrion Music Server (LMS, formerly Logitech Media Server / Squeezebox Server), written in Go. It talks to LMS over its plaintext Telnet-style CLI protocol (default port 9090). `main.go` loads the config, connects, and hands the connection to the `tview` front-end.

## Commands

- Build: `go build ./...`
- Run: `go run .` (needs a config file, see below)
- Test: `go test ./...` (single test: `go test ./internal/lyrionapi -run TestConnection`)
- Format: `gofmt -l .` / `gofmt -w .`
- Vet: `go vet ./...`
- Install: `./install.sh` (as the normal user, not root — builds, `sudo install`s the binary to `/usr/local/bin`, and copies `config.template.json` to `~/.config/lyrion-tui/config.json` only if no config exists yet)

The server address comes from a JSON config file (`{"server_address": "host:9090"}`), and *which* file depends on how the binary was built (`internal/config`):

- `install.sh` builds with `-ldflags "-X github.com/medidew/lyrion-tui/internal/config.installed=true"`; such binaries read `<os.UserConfigDir()>/lyrion-tui/config.json`.
- Everything else (`go run`, `go test`, plain `go build`) reads `config.json` at the **repository root**, located from the source file's compile-time path (`runtime.Caller`), so it works regardless of the working directory — including `go test`, which runs in each package's directory. That file is gitignored (it holds a personal server address); `config.template.json` is the committed template.

A missing/invalid config is a plain error naming the expected path. `Connect` gives up after `lyrionapi.ConnectTimeout` (5s) with a clear error rather than hanging.

`internal/lyrionapi/protocol_test.go` is a plain unit test suite (fixture strings, no network) and always runs. `internal/lyrionapi/lms_test.go`'s `TestConnection` additionally requires a live LMS server: it reads the same (repo) config as `go run`, skips if none exists, and fails if the configured server is unreachable. There is no mock/fake server and no tests for `internal/tui` — UI changes have to be checked by running the app.

## Layout

Three packages under `internal/`, kept strictly separate — `tui` only uses `lyrionapi`'s exported surface, and `lyrionapi` knows nothing about config files or the UI:

- `internal/config` — locates (repo vs. per-user, see above) and loads/validates the JSON config file.
- `internal/lyrionapi` — the LMS client (package `lyrionapi`).
- `internal/tui` — the `tview` front-end.

## `internal/lyrionapi`

Split by concern, one file per LMS CLI command category:

- `lms.go` — `LyrionServer` struct, `Connect`/`Close`, and the two query primitives (`Query`, `queryTagged`) everything else is built on.
- `protocol.go` — pure parsing: raw line reading, tokenizing, the tagged-response record parser, typed tag getters, `sliceOrEmpty`, `encodeArg`. No I/O/concurrency.
- `notify.go` — connection lifecycle: the request/response correlation layer behind `Query`/`queryTagged`, plus `Listen`/`Subscribe` and the `Notification` type.
- `library.go` — genres/artists/albums/years/titles/songinfo/search/musicfolder, saved playlists (`GetPlaylists`, `GetPlaylistTracks`), plus `TotalGenres`/`TotalArtists`/`TotalAlbums`/`TotalSongs`.
- `player.go` — `LyrionPlayer` struct, `GetPlayer`/`GetPlayerCount`, and per-player identity/hardware/mixer/sync control.
- `playlist.go` — all playback and live-playlist manipulation on `*LyrionPlayer` (play/stop/pause, current song, `Next`/`Previous`/`SetIndex`, shuffle/repeat, `playlistcontrol`).
- `favorites.go`, `randomplay.go`, `alarms.go` — those command groups.
- `status.go` — `Players`/`SyncGroups`/`Libraries`/`Version`/`Can`/`ReadDirectory`, plus `ServerStatus`/`Status` (see caveat below).

Deliberately out of scope: server-admin/maintenance commands (`rescan`, `abortscan`, `wipecache`, `pragma`, `debug`, `logging`, `login`, `pref`, `artworkspec`, `getstring`, `stopserver`, `restartserver`) — this is a player client, not a server admin tool.

### Query/response model

Two connections are maintained per `LyrionServer`: `conn` for request/response commands, and a lazily-opened `notifyConn` used only for push notifications (see below). LMS notification lines are textually indistinguishable from ordinary command echoes, and the server only ever pushes them to a connection that has sent `listen`/`subscribe` — keeping `conn` free of that means a response on `conn` can never be misread as an unrelated notification. Both connections are read through a `bufio.Reader`; reading the raw socket a byte at a time was a measured bottleneck on large responses.

`Query`/`queryTagged` share a request/response correlation layer (`doQueryRaw` in `notify.go`): writes are serialized under a mutex that also enqueues a per-request channel in a FIFO, and a single reader goroutine drains `conn` and delivers each line to the oldest still-pending request. This makes `Query`/`queryTagged` **safe to call concurrently** from multiple goroutines on the same `LyrionServer`. Individual queries have no timeout — only `Connect` does.

LMS echoes the request as a literal prefix of the response (e.g. querying `"player count ?"` returns `"player count 7"`). Two parsing styles exist depending on the response shape:

- **Scalar responses** (`Query`): decode the whole line, then slice off the known-length echoed prefix. LMS can return a response *shorter* than the echoed prefix when there's nothing to report (e.g. querying the current title while stopped), so never slice by hand: use `player.queryField`/`queryIntField`/`queryFloatField`/`queryBoolField` (`playlist.go`) for player-scoped queries, or `sliceOrEmpty(response, len(request)-1)` for server-scoped ones.
- **Tagged, possibly multi-item responses** (`queryTagged`, in `lms.go`): count is `N` + a flat run of repeated `tag:value` groups, one group per item, with a *repeated key* (not an explicit separator) marking where the next record starts. `queryTagged` tokenizes the **raw, still percent-encoded** line on whitespace and decodes each token individually — decoding the whole line first (as `Query` does) would turn an encoded space (`%20`) inside a value into something indistinguishable from a real token separator, corrupting record boundaries. Domain files add typed struct mappers on top (e.g. `albumFromRecord` in `library.go`) using the `tagString`/`tagInt`/`tagFloat`/`tagBool` helpers in `protocol.go`.

  **Field-naming gotcha, confirmed against a live LMS 9.1.1 server**: the reference docs describe *response* fields using single-letter codes (e.g. `l` for album title, `a` for artist). In practice the server labels returned fields with full descriptive names regardless (`album`, `artist`, `artwork_track_id`, `tracknum`, `playlist`, ...) — the letter codes are only used in the *request's* `tags:` string to select which fields come back. Verify against a raw capture before trusting a docs-only field name; `search`'s response mixes four different id/name field-name pairs by category rather than one consistent shape, so it's parsed by hand in `library.go` (`Search`) instead of through the generic tagged-record parser.

Free-text arguments in outgoing commands (titles, search terms, URLs) must go through `encodeArg`, which uses `%20` for spaces as LMS does (not `+`, which `url.QueryEscape` would emit — `+` is meaningful in LMS syntax, e.g. `playlist index +1`). `LoadTracks`/`AddTracks` are the exception: they take a pre-formatted `key=value` fragment and send it as-is.

### Notifications

`Listen()`/`Subscribe(verbs...)` open `notifyConn` on first use, send `listen 1`, and discard that command's own echo before starting a dedicated reader goroutine (otherwise every subscriber would see a spurious first "notification" that's really just LMS echoing `listen 1` back). Every line read after that is unambiguously a real notification, parsed into a `Notification{PlayerID, Verb, Args, Raw}` and fanned out non-blockingly (drop, not block, on a full subscriber channel — a slow consumer must never stall the shared reader). `Subscribe` always requests everything (`listen 1`) and filters per-subscriber by verb client-side, rather than using the CLI's own `subscribe <list>`, since that's connection-scoped ("last one wins") and can't support multiple independent Go subscribers with different filters over one shared connection. Filtering by player is the subscriber's job (compare `Notification.PlayerID`).

## `internal/tui`

- `app.go` — `App`: owns the `LyrionServer`, the single **active player** that all playback actions target (chosen on the players panel), the panels, and the global key handler.
- `players.go` — player-select table (name/model/signal/power/state); `Enter` makes a row the active player.
- `library.go` — the browse panel, a stack of `libraryFrame`s (`pushFrame`/`popFrame`). Every row is a `libraryItem` with a kind; `runOrDrill` decides what `Enter`/`l` does (menu/genre/artist rows drill in, everything else plays now) and `z` queues. `p` previews a genre/artist/album/playlist's tracks as a new frame without acting on it. The player's queue is also shown as a frame here (`openQueue`, marked `isQueue`).
- `nowplaying.go` — status bar for the active player, kept live by a `Subscribe` on `playlist`/`mixer`/`power` plus a 1s ticker for elapsed time. `Enter`/`p` while it's focused opens the queue view.
- `queue.go` — `queueCache`: a local copy of the active player's queue, refetched in the background whenever a `playlist`/`playlistcontrol` notification arrives for that player (bursts are coalesced into at most one extra refetch). The queue view reads from it and re-renders in place when it changes.
- `actions.go` — `applyFilter`: play-now (`cmd:load`) or queue (`cmd:add`) via `PlaylistControl`. Saved playlists are played/queued by URL through `PlaySong`/`AddSong` instead.
- `help.go` — the one-line hint bar and the `?` help overlay. **When adding or changing a key binding, update both `hintText` and `helpText`.**

Rules that span these files:

- **Threading:** tview widgets may only be touched on the UI goroutine. Background goroutines (notification watchers, the ticker, queue refreshes) compute their data off-thread and apply it inside `app.tview.QueueUpdateDraw(...)`. They also re-check that the active player hasn't changed before applying a result.
- **Key handling:** `App.globalInput` is installed with `Application.SetInputCapture`, so it sees every key *before* the focused widget does. Global single-key shortcuts (space, `s`, `d`, `q`, `?`, `+`/`-`, `[`/`]`) are handled there; panel-specific keys (`j`/`k`/`h`/`l`, `z`, `p`, `/`) live in each panel's own `SetInputCapture`. When a `tview.InputField` (the search box) has focus, `globalInput` passes everything through except Ctrl+C so those letters can be typed — any new text field gets this for free, but any new global shortcut must not break it.

## Known gaps

- `ServerStatus`/`Status` (`status.go`) compose several already-verified single-purpose queries rather than parsing the compound `serverstatus`/`status` CLI commands directly — those commands' own nested response shape (a top-level block, then a per-player loop, then for `status` a further nested playlist-track loop) wasn't confirmed against a live server and doesn't fit the flat tagged-record parser. A future revision could fold this into one round trip once that's verified. `CurrentSong`/`TrackAt` similarly cost one round trip per field, so a `queueCache` refresh costs ~6 round trips per queued track.
- `DirEntry` (`ReadDirectory`), parts of `FolderItem` (`GetMusicFolder`), and `GetPlaylistTracks`'s fields use names extrapolated from the confirmed naming convention above but not directly verified against a live response.
- Global shortcuts still fire while the `?` help overlay is open (only text fields are exempted in `globalInput`).
