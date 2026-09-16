package tui

import (
	"fmt"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/medidew/lyrion-tui/internal/lyrionapi"
)

// playersPanel is the player-select screen (bound to 'd', like spotify-tui's
// device switcher): it lists every known player with its live status and
// lets the user pick which one becomes the App's active player.
type playersPanel struct {
	app  *App
	root *tview.Table
	rows []lyrionapi.PlayerSummary
}

func newPlayersPanel(app *App) *playersPanel {
	table := tview.NewTable().SetSelectable(true, false).SetFixed(1, 0)
	table.SetBorder(true).SetTitle(" Players (d)  -  Enter to select ")

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
	panel.rows = players

	for i, summary := range players {
		row := i + 1
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

	if len(players) > 0 {
		panel.root.Select(1, 0)
	}
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
	summary := panel.rows[index]

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
