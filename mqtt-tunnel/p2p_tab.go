//go:build !nofyne

package main

import (
	"context"
	"strings"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// buildP2PTab constructs the P2P tab as a copy-paste SDP wizard.
//
// Either role can run on any OS (Mac, Windows, Linux).
//
// Consumer (Initiator) flow:
//  1. Enter port list → "Generate Offer" → copy base64 SDP → send to Provider
//  2. Paste Provider's Answer → "Connect" → tunnel is live
//
// Provider (Responder) flow:
//  1. Paste Consumer's Offer → "Accept & Generate Answer" → copy base64 SDP → send back
//
// No server, no third party.  Only STUN is contacted (once, to learn public IP).
func buildP2PTab(win fyne.Window) *container.TabItem {
	// ── Shared widgets ───────────────────────────────────────────────────────

	statusLabel := widget.NewLabel("Status: Idle")

	var logMu sync.Mutex
	var logLines []string
	logWidget := widget.NewLabel("")
	logWidget.Wrapping = fyne.TextWrapWord
	logScroll := container.NewVScroll(logWidget)
	logScroll.SetMinSize(fyne.NewSize(0, 140))

	appendLog := func(msg string) {
		fyne.Do(func() {
			logMu.Lock()
			logLines = append(logLines, msg)
			if len(logLines) > 200 {
				logLines = logLines[len(logLines)-200:]
			}
			logWidget.SetText(strings.Join(logLines, "\n"))
			logScroll.ScrollToBottom()
			logMu.Unlock()
		})
	}
	setStatus := func(s string) {
		fyne.Do(func() { statusLabel.SetText(s) })
	}

	// ── Session ──────────────────────────────────────────────────────────────

	session := NewP2PSession(setStatus, appendLog)

	// stopBtn is shared between modes
	stopBtn := widget.NewButtonWithIcon("Stop", theme.MediaStopIcon(), nil)
	stopBtn.Disable()

	// ── Mode selector ────────────────────────────────────────────────────────

	modeSelect := widget.NewSelect(
		[]string{"Consumer (Initiator \u2014 opens local ports)", "Provider (Responder \u2014 bridges local services)"},
		nil,
	)

	// ── Consumer (Initiator) widgets ───────────────────────────────────────────

	portsEntry := widget.NewEntry()
	portsEntry.SetPlaceHolder("e.g. 1883,8080,22")

	offerEntry := widget.NewMultiLineEntry()
	offerEntry.SetPlaceHolder("Offer SDP will appear here after \"Generate Offer\"")
	offerEntry.Wrapping = fyne.TextWrapBreak
	offerEntry.SetMinRowsVisible(3)

	copyOfferBtn := widget.NewButtonWithIcon("Copy Offer", theme.ContentCopyIcon(), func() {
		win.Clipboard().SetContent(offerEntry.Text)
		appendLog("Offer copied to clipboard.")
	})
	copyOfferBtn.Disable()

	answerEntry := widget.NewMultiLineEntry()
	answerEntry.SetPlaceHolder("Paste the Provider's Answer here, then click \"Connect\"")
	answerEntry.Wrapping = fyne.TextWrapBreak
	answerEntry.SetMinRowsVisible(3)

	var genOfferCtxCancel context.CancelFunc

	generateOfferBtn := widget.NewButtonWithIcon("Generate Offer", theme.MediaRecordIcon(), nil)
	connectBtn := widget.NewButtonWithIcon("Connect", theme.ConfirmIcon(), nil)
	connectBtn.Disable()

	generateOfferBtn.OnTapped = func() {
		ports := strings.TrimSpace(portsEntry.Text)
		if ports == "" {
			appendLog("Enter port(s) first.")
			return
		}
		disableWidgets(generateOfferBtn, portsEntry, connectBtn)
		stopBtn.Enable()
		offerEntry.SetText("")
		copyOfferBtn.Disable()
		answerEntry.SetText("")

		var ctx context.Context
		ctx, genOfferCtxCancel = context.WithCancel(context.Background())
		go func() {
			sdp, err := session.GenerateOffer(ctx, ports)
			if err != nil {
				appendLog("GenerateOffer error: " + err.Error())
				fyne.Do(func() {
					enableWidgets(generateOfferBtn, portsEntry)
					stopBtn.Disable()
				})
				return
			}
			fyne.Do(func() {
				offerEntry.SetText(sdp)
				copyOfferBtn.Enable()
				connectBtn.Enable()
			})
		}()
	}

	connectBtn.OnTapped = func() {
		answer := strings.TrimSpace(answerEntry.Text)
		if answer == "" {
			appendLog("Paste the Provider's Answer first.")
			return
		}
		disableWidgets(connectBtn, answerEntry)
		if err := session.ApplyAnswer(answer); err != nil {
			appendLog("ApplyAnswer error: " + err.Error())
			fyne.Do(func() { enableWidgets(connectBtn, answerEntry) })
		}
	}

	consumerCard := widget.NewCard("Consumer \u2013 Initiator", "",
		container.NewVBox(
			widget.NewLabel("Ports to tunnel (comma-separated):"),
			portsEntry,
			generateOfferBtn,
			widget.NewSeparator(),
			widget.NewLabel("Offer (copy and send to the Provider):"),
			container.NewVScroll(offerEntry),
			copyOfferBtn,
			widget.NewSeparator(),
			widget.NewLabel("Paste the Provider's Answer:"),
			container.NewVScroll(answerEntry),
			connectBtn,
		),
	)

	// ── Provider (Responder) widgets ───────────────────────────────────────────

	offerInputEntry := widget.NewMultiLineEntry()
	offerInputEntry.SetPlaceHolder("Paste the Consumer's Offer here")
	offerInputEntry.Wrapping = fyne.TextWrapBreak
	offerInputEntry.SetMinRowsVisible(3)

	answerOutputEntry := widget.NewMultiLineEntry()
	answerOutputEntry.SetPlaceHolder("Answer SDP will appear here after \"Accept & Generate Answer\"")
	answerOutputEntry.Wrapping = fyne.TextWrapBreak
	answerOutputEntry.SetMinRowsVisible(3)

	copyAnswerBtn := widget.NewButtonWithIcon("Copy Answer", theme.ContentCopyIcon(), func() {
		win.Clipboard().SetContent(answerOutputEntry.Text)
		appendLog("Answer copied to clipboard.")
	})
	copyAnswerBtn.Disable()

	var acceptCtxCancel context.CancelFunc
	acceptBtn := widget.NewButtonWithIcon("Accept & Generate Answer", theme.MediaRecordIcon(), nil)
	acceptBtn.OnTapped = func() {
		offer := strings.TrimSpace(offerInputEntry.Text)
		if offer == "" {
			appendLog("Paste the Consumer's Offer first.")
			return
		}
		disableWidgets(acceptBtn, offerInputEntry)
		stopBtn.Enable()
		answerOutputEntry.SetText("")
		copyAnswerBtn.Disable()

		var ctx context.Context
		ctx, acceptCtxCancel = context.WithCancel(context.Background())
		go func() {
			sdp, err := session.AcceptOfferAndGenerateAnswer(ctx, offer)
			if err != nil {
				appendLog("Accept error: " + err.Error())
				fyne.Do(func() {
					enableWidgets(acceptBtn, offerInputEntry)
					stopBtn.Disable()
				})
				return
			}
			fyne.Do(func() {
				answerOutputEntry.SetText(sdp)
				copyAnswerBtn.Enable()
			})
		}()
	}

	providerCard := widget.NewCard("Provider \u2013 Responder", "",
		container.NewVBox(
			widget.NewLabel("Paste the Consumer's Offer:"),
			container.NewVScroll(offerInputEntry),
			acceptBtn,
			widget.NewSeparator(),
			widget.NewLabel("Answer (copy and send back to the Consumer):"),
			container.NewVScroll(answerOutputEntry),
			copyAnswerBtn,
		),
	)

	// ── Mode switching ───────────────────────────────────────────────────────

	consumerCard.Hide()
	providerCard.Hide()

	modeSelect.OnChanged = func(choice string) {
		switch choice {
		case "Consumer (Initiator \u2014 opens local ports)":
			consumerCard.Show()
			providerCard.Hide()
		case "Provider (Responder \u2014 bridges local services)":
			providerCard.Show()
			consumerCard.Hide()
		}
	}

	// ── Stop button ──────────────────────────────────────────────────────────

	stopBtn.OnTapped = func() {
		if genOfferCtxCancel != nil {
			genOfferCtxCancel()
		}
		if acceptCtxCancel != nil {
			acceptCtxCancel()
		}
		session.Stop()
		fyne.Do(func() {
			stopBtn.Disable()
			enableWidgets(generateOfferBtn, portsEntry, connectBtn)
			enableWidgets(acceptBtn, offerInputEntry)
			copyOfferBtn.Disable()
			copyAnswerBtn.Disable()
			offerEntry.SetText("")
			answerEntry.SetText("")
			answerOutputEntry.SetText("")
		})
	}

	// ── Layout ───────────────────────────────────────────────────────────────

	controls := container.NewVBox(
		widget.NewLabel("Select your role on this machine:"),
		modeSelect,
		consumerCard,
		providerCard,
		stopBtn,
	)

	content := container.NewVBox(
		statusLabel,
		widget.NewSeparator(),
		controls,
		widget.NewSeparator(),
		widget.NewLabel("Log:"),
		logScroll,
	)

	return container.NewTabItemWithIcon("P2P Tunnel", theme.ComputerIcon(), content)
}
