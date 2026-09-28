package tui

import (
	"fmt"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/medidew/lyrion-tui/internal/lyrionapi"
)

// playersPanel is the player-select screen (bound to 'd', like spotify-tui's
// device switcher): it lists every known player with its live status and
// lets the user pick which one becomes the App's active player. The first
// row is always the local player (this machine; see local.go).
type playersPanel struct {
	app  *App
	root *tview.Table
	rows []playerRow
}

// playerRow is one selectable row: either a server-side player or the local
// player.
type playerRow struct {
	local   bool
	summary lyrionapi.PlayerSummary
}

const playersTitle = " Players (d)  -  Enter to select "

func newPlayersPanel(app *App) *playersPanel {
	table := tview.NewTable().SetSelectable(true, false).SetFixed(1, 0)
	table.SetBorder(true).SetTitle(playersTitle)

	panel := &playersPanel{app: app, root: table}
	table.SetSelectedFunc(func(row, column int) {
		panel.selectRow(row)
	})

	return panel
}

// Refresh re-fetches the player list and each player's signal strength.
// Signal strength isn't part of the bulk "players" query, so it costs one
// extra round trip per player - acceptable here since this panel is only
// populated on demand (opening it, not on every frame).
func (panel *playersPanel) Refresh() {
	panel.root.Clear()
	panel.setHeader()

	players, err := panel.app.server.Players(0, 200)
	if err != nil {
		panel.root.SetCell(1, 0, tview.NewTableCell(fmt.Sprintf("error: %v", err)).SetSelectable(false))
		return
	}

	panel.rows = []playerRow{{local: true}}
	panel.setLocalRow(players)

	for _, summary := range players {
		if summary.ID == panel.app.local.config.ID {
			continue // shown as the local row instead
		}
		panel.rows = append(panel.rows, playerRow{summary: summary})
		row := len(panel.rows)
		signal := "-"
		if lp, err := panel.app.server.GetPlayer(summary.Index); err == nil {
			if strength, err := lp.SignalStrength(); err == nil && strength > 0 {
				signal = fmt.Sprintf("%d%%", strength)
			}
		}

		active := ""
		if panel.app.active != nil && summary.ID == panel.app.activeSummary.ID {
			active = "> "
		}

		panel.root.SetCell(row, 0, tview.NewTableCell(active+summary.Name))
		panel.root.SetCell(row, 1, tview.NewTableCell(summary.ModelName))
		panel.root.SetCell(row, 2, tview.NewTableCell(signal))
		panel.root.SetCell(row, 3, tview.NewTableCell(onOff(summary.Power)))
		panel.root.SetCell(row, 4, tview.NewTableCell(connectionState(summary)))
	}

	panel.root.Select(1, 0)
}

// setLocalRow renders the local player's row, using the server's view of it
// (players) once its stream is running.
func (panel *playersPanel) setLocalRow(players []lyrionapi.PlayerSummary) {
	local := panel.app.local
	state := "not started"
	if local.starting {
		state = "starting"
	}
	if local.stream != nil {
		state = "disconnected"
		for _, summary := range players {
			if summary.ID == local.config.ID {
				state = connectionState(summary)
			}
		}
	}

	active := ""
	if local.isActive() {
		active = "> "
	}
	panel.root.SetCell(1, 0, tview.NewTableCell(active+tview.Escape(local.config.Name)))
	panel.root.SetCell(1, 1, tview.NewTableCell("this machine"))
	panel.root.SetCell(1, 2, tview.NewTableCell("-"))
	panel.root.SetCell(1, 3, tview.NewTableCell("-"))
	panel.root.SetCell(1, 4, tview.NewTableCell(state))
}

// showStatus puts a short message in the panel's title ("" restores it).
func (panel *playersPanel) showStatus(message string) {
	if message == "" {
		panel.root.SetTitle(playersTitle)
		return
	}
	panel.root.SetTitle(playersTitle + " " + message + " ")
}

func (panel *playersPanel) showError(err error) {
	panel.showStatus("[red]error: " + tview.Escape(err.Error()) + "[-]")
}

func (panel *playersPanel) setHeader() {
	headers := []string{"Name", "Model", "Signal", "Power", "State"}
	for col, text := range headers {
		panel.root.SetCell(0, col, tview.NewTableCell(text).
			SetSelectable(false).
			SetAttributes(tcell.AttrBold).
			SetTextColor(tcell.ColorYellow))
	}
}

func (panel *playersPanel) selectRow(row int) {
	index := row - 1
	if index < 0 || index >= len(panel.rows) {
		return
	}
	if panel.rows[index].local {
		panel.app.local.activate()
		return
	}
	summary := panel.rows[index].summary

	player, err := panel.app.server.GetPlayer(summary.Index)
	if err != nil {
		return
	}
	panel.app.SetActivePlayer(player, summary)
}

func onOff(on bool) string {
	if on {
		return "on"
	}
	return "off"
}

func connectionState(summary lyrionapi.PlayerSummary) string {
	if !summary.Connected {
		return "disconnected"
	}
	if summary.IsPlaying {
		return "playing"
	}
	return "idle"
}
