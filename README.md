# lyrion-tui

A terminal user interface client for a [Lyrion Music Server](https://lyrion.org/) (LMS, formerly Logitech Media Server / Squeezebox Server). It talks to LMS directly over its plaintext CLI protocol (default port 9090) and allows you to browse your library, pick a player, and control playback, all from the terminal and using only a keyboard. It can also be a player itself: the "Local" player plays the server's audio stream through your computer's speakers.

The project has two parts:

- **`internal/lyrionapi`**: a Go API for the [LMS command-line interface](https://lyrion.org/reference/cli/using-the-cli/), covering library browsing (genres/artists/albums/tracks/playlists/search), playback and playlist control, favorites, random mixes, alarms, player status, and live server-push notifications.
- **`internal/localplayer`**: plays the server's HTTP audio stream locally, in pure Go.
- **`internal/tui`**: the [tview](https://github.com/rivo/tview)-based terminal front-end built on those.

## Installing

```bash
./install.sh
```

Run this as your normal user (not with `sudo`). It builds the binary, installs it to `/usr/local/bin` (prompting for your sudo password for that step only), and copies `config.template.json` to `~/.config/lyrion-tui/config.json` unless a config already exists there. Edit that file to point at your LMS server's CLI port:

```json
{
  "server_address": "192.168.1.10:9090"
}
```

Optionally, `web_port` (default `9000`) is your LMS web port, used by the Local player, and `local_player_name` sets what the Local player is called on the server.

Then run `lyrion-tui`. Press `?` inside the app for the full list of keybindings.

## Development

`go run .` and `go test ./...` read `config.json` from the repository root instead of `~/.config`, so development never touches an installed config. Copy `config.template.json` to `config.json` and set your server's address; `config.json` is gitignored.

## Dependencies

- **[Go](https://go.dev/)** (1.25+)
- **[tview](https://github.com/rivo/tview)**
- **[tcell](https://github.com/gdamore/tcell)**
- **[go-mp3](https://github.com/hajimehoshi/go-mp3)** and **[oto](https://github.com/ebitengine/oto)**, for the Local player (which needs PulseAudio/PipeWire or ALSA at runtime)