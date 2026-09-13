package radio

import (
	"fmt"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

const remoteTextWidth = 40

func (a *Application) setupRemoteModal() {
	a.remoteQR = tview.NewTextView().SetDynamicColors(true)
	a.remoteQR.SetBackgroundColor(tcell.ColorDefault)

	a.remoteText = tview.NewTextView().SetDynamicColors(true).SetWordWrap(true)
	a.remoteText.SetBackgroundColor(tcell.ColorDefault)

	a.remoteModal = tview.NewFlex().SetDirection(tview.FlexRow)
	a.remoteModal.SetBackgroundColor(tcell.ColorDefault)

	a.pages.AddPage(a.pageNames[RemotePage], a.remoteModal, true, false)
}

func (a *Application) toggleRemoteModal() {
	if a.isRemoteModalOpen() {
		a.hideRemoteModal()
		return
	}
	a.showRemoteModal()
}

func (a *Application) showRemoteModal() {
	var qr, text string
	qrWidth, qrHeight := 0, 0

	r, err := a.startRemote()
	if err != nil {
		text = fmt.Sprintf("[red]Could not start the remote control server:[-]\n\n%s", err)
	} else {
		var qrErr error
		qr, qrWidth, qrHeight, qrErr = qrText(r.URL())
		if qrErr != nil {
			log.Printf("remote: qr: %v", qrErr)
		}

		var sb strings.Builder
		sb.WriteString("[green::b]Control goradion from your phone[-::-]\n\n")
		sb.WriteString("Scan the QR code with the phone camera, or open\n\n")
		sb.WriteString(fmt.Sprintf("  [yellow]%s[-]\n\n", r.Address()))
		if r.Key() == "" {
			sb.WriteString("[red]No access code is set:[-] anyone on the network can control goradion.\n\n")
		} else {
			sb.WriteString("and enter the code\n\n")
			sb.WriteString(fmt.Sprintf("  [yellow::b]%s[-::-]\n\n", tview.Escape(r.Key())))
		}
		sb.WriteString("The phone must be on the same network.\n\n")
		sb.WriteString("[gray]Esc closes this window.[-]")
		text = sb.String()
	}

	a.remoteQR.SetText(qr)
	a.remoteText.SetText(text)

	// Empty boxes, not nil spacers: a Flex does not clear its background.
	content := tview.NewFlex().SetDirection(tview.FlexColumn).AddItem(tview.NewBox(), 1, 0, false)
	if qr != "" {
		content.AddItem(a.remoteQR, qrWidth, 0, false).
			AddItem(tview.NewBox(), 2, 0, false)
	}
	content.AddItem(a.remoteText, 0, 1, true).AddItem(tview.NewBox(), 1, 0, false)
	content.SetBorder(true).
		SetTitle(" Remote control ").SetTitleAlign(tview.AlignLeft).SetTitleColor(colorAccent).
		SetBackgroundColor(tcell.ColorDefault)

	height := max(qrHeight, 12) + 2
	width := qrWidth + 2 + remoteTextWidth + 4
	if qr == "" {
		width = remoteTextWidth + 4
	}

	a.remoteModal.Clear().
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().SetDirection(tview.FlexColumn).
			AddItem(nil, 0, 1, false).
			AddItem(content, width, 0, true).
			AddItem(nil, 0, 1, false), height, 0, true).
		AddItem(nil, 0, 1, false)

	a.pages.ShowPage(a.pageNames[RemotePage])
	a.app.SetFocus(a.remoteText)
}

func (a *Application) hideRemoteModal() {
	a.pages.HidePage(a.pageNames[RemotePage])
}

func (a *Application) isRemoteModalOpen() bool {
	return a.pages.GetPageNames(true)[0] == a.pageNames[RemotePage]
}
