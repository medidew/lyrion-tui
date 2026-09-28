# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

A keyboard-driven terminal UI (TUI) client for a Lyrion Music Server (LMS, formerly Logitech Media Server / Squeezebox Server), written in Go. It talks to LMS over its plaintext Telnet-style CLI protocol (default port 9090), and can also act as a player itself by playing the server's HTTP audio stream through this machine's speakers (the "Local" player). `main.go` loads the config, connects, and hands the connection to the `tview` front-end.

## Commands

- Build: `go build ./...`
- Run: `go run .` (needs a config file, see below)
- Test: `go test ./...` (single test: `go test ./internal/lyrionapi -run TestConnection`)
- Format: `gofmt -l .` / `gofmt -w .`
- Vet: `go vet ./...`
- Install: `./install.sh` (as the normal user, not root — builds, `sudo install`s the binary to `/usr/local/bin`, and copies `config.template.json` to `~/.config/lyrion-tui/config.json` only if no config exists yet)

Settings come from a JSON config file: `server_address` (required, `host:9090`), plus optional `web_port` (LMS's web port on the same host, default 9000, which serves the local player's stream) and `local_player_name` (default `lyrion-tui on <hostname>`); `Load` fills in the defaults. *Which* file is read depends on how the binary was built (`internal/config`):

- `install.sh` builds with `-ldflags "-X github.com/medidew/lyrion-tui/internal/config.installed=true"`; such binaries read `<os.UserConfigDir()>/lyrion-tui/config.json`.
- Everything else (`go run`, `go test`, plain `go build`) reads `config.json` at the **repository root**, located from the source file's compile-time path (`runtime.Caller`), so it works regardless of the working directory — including `go test`, which runs in each package's directory. That file is gitignored (it holds a personal server address); `config.template.json` is the committed template.

A missing/invalid config is a plain error naming the expected path. `Connect` gives up after `lyrionapi.ConnectTimeout` (5s) with a clear error rather than hanging.

`internal/lyrionapi/protocol_test.go` and `internal/localplayer`'s tests are plain unit tests (fixture strings / `testdata/mixed-rates.mp3`, no network or audio device) and always run. `internal/lyrionapi/lms_test.go`'s `TestConnection` additionally requires a live LMS server: it reads the same (repo) config as `go run`, skips if none exists, and fails if the configured server is unreachable. There is no mock/fake server and no tests for `internal/tui` — UI changes have to be checked by running the app.

## Layout

Four packages under `internal/`, kept strictly separate — `tui` only uses the others' exported surfaces, `lyrionapi` knows nothing about config files or the UI, and `localplayer` knows nothing about the LMS CLI or the UI:

- `internal/config` — locates (repo vs. per-user, see above) and loads/validates the JSON config file.
- `internal/lyrionapi` — the LMS client (package `lyrionapi`).
- `internal/localplayer` — plays an LMS HTTP audio stream on this machine.
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

## `internal/localplayer`

Plays `http://<host>:<web_port>/stream.mp3?player=<id>` through the speakers, in pure Go: `go-mp3` decodes, `oto` v3 outputs (PulseAudio/PipeWire, falling back to ALSA via purego — no cgo, no system headers needed to build). Facts about the LMS side, from LMS 9.1's `Slim/Web/HTTP.pm` / `Slim/Player/HTTP.pm` and live probes:

- Each connection becomes a `Slim::Player::HTTP` player (model `http`, `isplayer:0`). Its ID is the `player` query param if given, **else the client's IP** — so `ID()` (a stable, locally-administered fake MAC hashed from the hostname) must always be passed, or the app would take over any browser on this machine already using `/stream.mp3`. `/stream` and `/stream.mp3` are the same code path.
- It's then controlled over the CLI like any player, but **LMS's volume is a stub** for it (`mixer volume` does nothing), so volume is applied locally (`Player.SetVolume`).
- On connect, LMS auto-plays the player's existing playlist; the TUI stops it right after connecting.
- LMS **doesn't pace the stream**: it sends as fast as the client reads (hundreds of MB/min to a greedy reader) and counts a track as played once sent, and streams MP3 silence while idle. Pacing comes only from backpressure, so every buffer between LMS and the speakers is lag between the server's state and what's heard. Our side is kept small (`SO_RCVBUF` via `dialSmallBuffer`, the bounded `pcmBuffer`, oto's shortened buffer: under a second), but **LMS keeps several seconds queued on its own side** (measured 4–7s at 320 kbps, ~20s at 64 kbps) that the client can't limit — and its `clearOutputBuffer` is a no-op (it deletes `$outbuf{$client->id}`, but the queue is keyed by socket), so a song change doesn't discard it either.
- So song changes go through **`Player.Restart`**: drop the connection, wait for LMS to report the player disconnected (`connected ?` → 0), send the command, reconnect. The new connection starts exactly at the new song. The order is load-bearing, verified by matching streamed frames against the track files: sending the command before LMS has noticed the disconnect loses the new song's first seconds into the dying socket, and reconnecting without waiting at all leaves LMS feeding chunks to both sockets (audible skips). Pause/resume does *not* restart — the buffered audio is the correct continuation.
- LMS's `time ?` for an HTTP player is unreliable (it ran 10–20s ahead of what had even been delivered), and when it starts sending the playlist's **last** track it misreports `playlist index`/title as index 0 / the first track.
- The stream is one MP3 bitstream whose **sample rate changes between tracks** (passthrough MP3s keep theirs, e.g. 22.05 kHz; FLAC/etc. are transcoded to 44.1/48 kHz). go-mp3 fixes the rate from the first frame, so `frames.go` walks the stream frame by frame and cuts it into constant-rate `segment`s, each decoded by a fresh decoder and resampled (`resample.go`, linear) to oto's fixed 44.1 kHz.

`pcmBuffer` exists because oto's mixer calls every source's `Read` synchronously on one loop: a source blocking on the network would stall it and make `Close` hang. The decoder goroutine blocks writing into it instead; oto's reads never block (empty reads 0 bytes). `Interrupt` releases a blocked writer (needed for a `Restart` while output is paused), and its count of bytes handed to oto backs `Player.Played()`, the audio actually played since the last restart. `Start` connects once synchronously (so a bad URL is an immediate error), then a supervisor goroutine reconnects after drops until `Close`. oto allows one context per process (`audioContext`, `sync.Once`).

## `internal/tui`

- `app.go` — `App`: owns the `LyrionServer`, the single **active player** that all playback actions target (chosen on the players panel), the panels, and the global key handler.
- `players.go` — player-select table (name/model/signal/power/state); `Enter` makes a row the active player. The first row is always the local player (the server's own row for its ID is folded into it).
- `local.go` — `localPlayer`: starts the `localplayer` stream the first time Local is selected (off the UI goroutine: connect, wait for the server to register the ID, name it, stop the auto-played playlist), then keeps it running — even while another player is active — until quit. It mirrors the server's play/pause state onto the local output via a notification subscription. `App.changeSong` wraps every command that changes the song (play-now from the library/queue, `[`/`]`, stop, play from stopped): for the local player it runs the command inside a `Restart` in the background (`localPlayer.changeSong`), for other players directly. Start-up also stops LMS's auto-played playlist inside a restart. `App.adjustVolume` and the Now Playing meter use its local volume when it's active.
- For the local player, Now Playing ignores LMS's song/time after one of the app's own song changes (`localRestarted`): it shows the song actually being heard and its position from `Played()`, advancing to the next song in the *cached queue* (not LMS's current song — see the last-track misreport above) once the heard one has played out locally. A song change or stop made from another controller ends this tracking and falls back to LMS's view.
- `library.go` — the browse panel, a stack of `libraryFrame`s (`pushFrame`/`popFrame`). Every row is a `libraryItem` with a kind; `runOrDrill` decides what `Enter`/`l` does (menu/genre/artist rows drill in, everything else plays now) and `z` queues. `p` previews a genre/artist/album/playlist's tracks as a new frame without acting on it. The player's queue is also shown as a frame here (`openQueue`, marked `isQueue`).
- `nowplaying.go` — status bar for the active player, kept live by a `Subscribe` on `playlist`/`mixer`/`power` plus a 1s ticker for elapsed time. `Enter`/`p` while it's focused opens the queue view.
- `queue.go` — `queueCache`: a local copy of the active player's queue, refetched in the background whenever a `playlist`/`playlistcontrol` notification arrives for that player (bursts are coalesced into at most one extra refetch). The queue view reads from it and re-renders in place when it changes.
- `actions.go` — `applyFilter`: play-now (`cmd:load`) or queue (`cmd:add`) via `PlaylistControl`. Saved playlists are played/queued by URL through `PlaySong`/`AddSong` instead.
- `help.go` — the one-line hint bar (just points at `?`) and the `?` help overlay. **When adding or changing a key binding, update `helpText`.**

Rules that span these files:

- **Threading:** tview widgets may only be touched on the UI goroutine. Background goroutines (notification watchers, the ticker, queue refreshes) compute their data off-thread and apply it inside `app.tview.QueueUpdateDraw(...)`. They also re-check that the active player hasn't changed before applying a result.
- **Key handling:** `App.globalInput` is installed with `Application.SetInputCapture`, so it sees every key *before* the focused widget does. Global single-key shortcuts (space, `s`, `d`, `q`, `?`, `+`/`-`, `[`/`]`) are handled there; panel-specific keys (`j`/`k`/`h`/`l`, `z`, `p`, `/`) live in each panel's own `SetInputCapture`. When a `tview.InputField` (the search box) has focus, `globalInput` passes everything through except Ctrl+C so those letters can be typed — any new text field gets this for free, but any new global shortcut must not break it. The `?` help overlay is modal the same way: while it's open, `globalInput` handles only `?` (close) and Ctrl+C, and the overlay (explicitly focused by `toggleHelp`, which restores the previous focus on close) swallows everything else except Esc/Enter.

## Known gaps

- Local player: go-mp3 rejects MPEG-2.5 (8–12 kHz) — those segments are skipped silently frame by frame — and garbles some low-bitrate MPEG-2 (e.g. 32 kbps at 22.05 kHz; 64 kbps decodes identically to ffmpeg). Only passthrough MP3s at those rates are affected. Song changes made from *other* controllers are still heard only after LMS's queued audio plays out (seconds), and Now Playing shows LMS's (early) view for them. Its volume starts at 100% each run.

- `ServerStatus`/`Status` (`status.go`) compose several already-verified single-purpose queries rather than parsing the compound `serverstatus`/`status` CLI commands directly — those commands' own nested response shape (a top-level block, then a per-player loop, then for `status` a further nested playlist-track loop) wasn't confirmed against a live server and doesn't fit the flat tagged-record parser. A future revision could fold this into one round trip once that's verified. `CurrentSong`/`TrackAt` similarly cost one round trip per field, so a `queueCache` refresh costs ~6 round trips per queued track.
- `DirEntry` (`ReadDirectory`), parts of `FolderItem` (`GetMusicFolder`), and `GetPlaylistTracks`'s fields use names extrapolated from the confirmed naming convention above but not directly verified against a live response.
