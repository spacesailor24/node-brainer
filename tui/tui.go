package tui

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

type TUI struct {
	// View components.
	App      *tview.Application
	Grid     *tview.Grid
	Networks *tview.List
	Clients  *tview.List
	Info     *tview.Table
}

func foo() {
	
}

func (tui *TUI) queueUpdateDraw(f func()) {
	go func() {
		tui.App.QueueUpdateDraw(f)
	}()
}

func (tui *TUI) setAfterDrawFunc(screen tcell.Screen) {
	tui.queueUpdateDraw(func() {
		p := tui.App.GetFocus()

		tui.Networks.SetBorderColor(tcell.ColorWhite)
		tui.Clients.SetBorderColor(tcell.ColorWhite)
		tui.Info.SetBorderColor(tcell.ColorWhite)

		switch p {
		case tui.Networks:
			tui.Networks.SetBorderColor(tcell.ColorGreen)
		case tui.Clients:
			tui.Clients.SetBorderColor(tcell.ColorGreen)
		case tui.Info:
			tui.Info.SetBorderColor(tcell.ColorGreen)
		}
	})
}

func setupKeyboard(tui *TUI) {
	focusMapping := map[tview.Primitive]struct{ next, prev tview.Primitive}{
		tui.Networks: {tui.Clients, tui.Info},
		tui.Clients: {tui.Info, tui.Networks},
		tui.Info: {tui.Networks, tui.Clients},
	}

	tui.App.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
			case tcell.KeyRight:
				if focusMap, ok := focusMapping[tui.App.GetFocus()]; ok {
					tui.App.SetFocus(focusMap.next)
				}
				return nil
			case tcell.KeyLeft:
				if focusMap, ok := focusMapping[tui.App.GetFocus()]; ok {
					tui.App.SetFocus(focusMap.prev)
				}
				return nil
			}
		return event
	})
}

func NewTUI() *TUI {
	t := TUI{}
	t.App = tview.NewApplication()

	t.Networks = tview.NewList()
	t.Clients = tview.NewList()
	t.Info = tview.NewTable()

	t.Networks.AddItem("Mainnet", "", 0, foo).ShowSecondaryText(false)
	t.Networks.AddItem("Sepolia", "", 0, foo).ShowSecondaryText(false)
	t.Networks.AddItem("OP Mainnet", "", 0, foo).ShowSecondaryText(false)
	t.Networks.AddItem("OP Sepolia", "", 0, foo).ShowSecondaryText(false)

	t.Clients.AddItem("Execution (Geth)", "", 0, foo).ShowSecondaryText(false)
	t.Clients.AddItem("Consensus (Lighthouse)", "", 0, foo).ShowSecondaryText(false)
	t.Clients.AddItem("Validator (Lighthouse)", "", 0, foo).ShowSecondaryText(false)

	t.Networks.SetTitle("Networks").SetBorder(true)
	t.Clients.SetTitle("Clients").SetBorder(true)
	t.Info.SetBorder(true)

	t.App.SetAfterDrawFunc(t.setAfterDrawFunc)
	setupKeyboard(&t)

	navigate := tview.NewGrid().SetRows(0, 0).
		AddItem(t.Networks, 0, 0, 1, 1, 0, 0, true).
		AddItem(t.Clients, 1, 0, 1, 1, 0, 0, false)
	info := tview.NewGrid().SetRows(0).
		AddItem(t.Info, 0, 0, 1, 1, 0, 0, false)
	t.Grid = tview.NewGrid().
		SetRows(0, 1).
		SetColumns(40, 0).
		SetBorders(false).
		AddItem(navigate, 0, 0, 2, 1, 0, 0, true).
		AddItem(info, 0, 1, 2, 1, 0, 0, false)

	return &t
}

// Start starts terminal user interface application.
func (tui *TUI) Start() error {
	return tui.App.SetRoot(tui.Grid, true).EnableMouse(true).Run()
}
