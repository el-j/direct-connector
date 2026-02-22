package p2p

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"

	"github.com/pion/webrtc/v3"
)

// STUN servers are ONLY used to discover your public IP:port (NAT traversal).
// They act like a mirror — they never relay or store any tunnel data.
// Zero bytes of your actual traffic ever pass through them.
// After the one-time SDP handshake (which you do yourself, out-of-band),
// ALL data flows directly between the two machines over encrypted DTLS.
var stunServers = []string{
	"stun:stun.l.google.com:19302",
	"stun:stun1.l.google.com:19302",
}

// ── SDP encode / decode ────────────────────────────────────────────────────
// SDP blobs are base64-encoded JSON so they fit on one line —
// easy to copy from a text box and paste into any chat or email.

func encodeSDP(sd webrtc.SessionDescription) (string, error) {
	b, err := json.Marshal(sd)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(b), nil
}

func decodeSDP(raw string) (webrtc.SessionDescription, error) {
	b, err := base64.StdEncoding.DecodeString(strings.TrimSpace(raw))
	if err != nil {
		return webrtc.SessionDescription{}, fmt.Errorf("base64 decode: %w", err)
	}
	var sd webrtc.SessionDescription
	if err := json.Unmarshal(b, &sd); err != nil {
		return webrtc.SessionDescription{}, fmt.Errorf("JSON parse: %w", err)
	}
	return sd, nil
}

// ── P2PSession ─────────────────────────────────────────────────────────────
//
// 100 % serverless after the one-time SDP copy-paste:
//
//   Consumer (Mac)                      Provider (Windows)
//   ──────────────                      ──────────────────
//   1. GenerateOffer() → base64 ──▶ paste into Provider UI
//                       ◀── base64 ── AcceptOfferAndGenerateAnswer()
//   2. ApplyAnswer(pasted text)
//         ↕  WebRTC DataChannels (DIRECT, encrypted peer-to-peer)  ↕
//   TCP listeners on Mac           bridge ↔ local services on Windows
//
// STUN is called once per side at startup to learn the public IP:port.
// No traffic of any kind passes through the STUN server after that.

// P2PSession manages a single WebRTC session.
// Create a new one for each connection attempt via NewP2PSession.
type P2PSession struct {
	mu         sync.Mutex
	pc         *webrtc.PeerConnection
	done       chan struct{}
	active     bool
	savedPorts string // set by GenerateOffer; used when Connected fires

	OnStatus func(string)
	OnLog    func(string)
}

// NewP2PSession returns a fresh, idle session.
func NewP2PSession(onStatus func(string), onLog func(string)) *P2PSession {
	return &P2PSession{OnStatus: onStatus, OnLog: onLog}
}

// IsActive reports whether the session is currently live.
func (s *P2PSession) IsActive() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.active
}

// Stop terminates the session.  Safe to call multiple times or when idle.
func (s *P2PSession) Stop() {
	s.mu.Lock()
	s.active = false
	pc := s.pc
	done := s.done
	s.mu.Unlock()

	if pc != nil {
		_ = pc.Close()
	}
	if done != nil {
		select {
		case <-done:
		default:
			close(done)
		}
	}
	s.emitStatus("Status: Disconnected")
	s.emitLog("Session stopped.")
}

// ── Consumer path (Initiator / Mac) ───────────────────────────────────────

// GenerateOffer creates a PeerConnection, gathers ICE candidates, and returns
// a compact base64 SDP string.  Share this with the Provider out-of-band
// (chat, email, USB stick — anything).  Then call ApplyAnswer.
//
// ctx cancels the ICE gather phase; passing it also prevents the goroutine from
// leaking if the user clicks Stop before ICE finishes.
func (s *P2PSession) GenerateOffer(ctx context.Context, ports string) (string, error) {
	s.mu.Lock()
	if s.active {
		s.mu.Unlock()
		return "", fmt.Errorf("session already active — call Stop() first")
	}
	done := make(chan struct{})
	s.active = true
	s.done = done
	s.savedPorts = ports
	s.mu.Unlock()

	pc, err := s.createPC()
	if err != nil {
		s.setInactive()
		return "", err
	}
	s.setupStateChange(pc, done)

	// A "control" channel forces SCTP negotiation before real channels open.
	if _, err := pc.CreateDataChannel("control", nil); err != nil {
		_ = pc.Close()
		s.setInactive()
		return "", fmt.Errorf("CreateDataChannel(control): %w", err)
	}

	offer, err := pc.CreateOffer(nil)
	if err != nil {
		_ = pc.Close()
		s.setInactive()
		return "", fmt.Errorf("CreateOffer: %w", err)
	}

	// GatheringCompletePromise MUST be registered before SetLocalDescription
	// to avoid a race where ICE finishes before the callback is wired up.
	gatherDone := webrtc.GatheringCompletePromise(pc)
	if err := pc.SetLocalDescription(offer); err != nil {
		_ = pc.Close()
		s.setInactive()
		return "", fmt.Errorf("SetLocalDescription: %w", err)
	}

	s.emitLog("Gathering ICE candidates (UDP + TCP/443) via STUN — no data sent to any server...")
	select {
	case <-gatherDone:
	case <-ctx.Done():
		_ = pc.Close()
		s.setInactive()
		return "", ctx.Err()
	}

	encoded, err := encodeSDP(*pc.LocalDescription())
	if err != nil {
		_ = pc.Close()
		s.setInactive()
		return "", fmt.Errorf("encodeSDP: %w", err)
	}

	s.emitLog("Offer ready. Copy it and send it to the Provider via any channel you like.")
	s.emitStatus("Status: Offer generated — waiting for Provider's Answer")
	return encoded, nil
}

// ApplyAnswer completes the Consumer's connection.
// Must be called after GenerateOffer returns successfully.
func (s *P2PSession) ApplyAnswer(answerSDP string) error {
	s.mu.Lock()
	pc := s.pc
	s.mu.Unlock()

	if pc == nil {
		return fmt.Errorf("no active session — call GenerateOffer first")
	}
	answer, err := decodeSDP(answerSDP)
	if err != nil {
		return fmt.Errorf("invalid answer: %w", err)
	}
	if err := pc.SetRemoteDescription(answer); err != nil {
		return fmt.Errorf("SetRemoteDescription: %w", err)
	}
	s.emitLog("Answer applied. Direct P2P handshake in progress...")
	s.emitStatus("Status: Connecting — direct P2P handshake in progress...")
	return nil
}

// ── Provider path (Responder / Windows) ───────────────────────────────────

// AcceptOfferAndGenerateAnswer creates a PeerConnection, applies the Consumer's
// offer, gathers ICE, and returns a compact base64 answer.  Send this back to
// the Consumer so they can call ApplyAnswer.  Once both sides have exchanged
// SDP the P2P tunnel is live and the Provider starts bridging DataChannels.
func (s *P2PSession) AcceptOfferAndGenerateAnswer(ctx context.Context, offerSDP string) (string, error) {
	s.mu.Lock()
	if s.active {
		s.mu.Unlock()
		return "", fmt.Errorf("session already active — call Stop() first")
	}
	done := make(chan struct{})
	s.active = true
	s.done = done
	s.mu.Unlock()

	offer, err := decodeSDP(offerSDP)
	if err != nil {
		s.setInactive()
		return "", fmt.Errorf("invalid offer: %w", err)
	}

	pc, err := s.createPC()
	if err != nil {
		s.setInactive()
		return "", err
	}

	// Bridge each incoming DataChannel to the matching local service.
	pc.OnDataChannel(func(dc *webrtc.DataChannel) {
		port := dc.Label()
		if port == "control" {
			return
		}
		s.emitLog("Incoming channel for port " + port)
		dc.OnOpen(func() {
			raw, err := dc.Detach()
			if err != nil {
				s.emitLog("Detach(" + port + "): " + err.Error())
				return
			}
			localConn, err := net.Dial("tcp", "localhost:"+port)
			if err != nil {
				s.emitLog("Cannot reach localhost:" + port + " — is the service running?")
				_ = raw.Close()
				return
			}
			s.emitLog("Bridging port " + port + " ↔ localhost:" + port)
			bridge(raw, localConn)
		})
	})

	s.setupStateChange(pc, done)

	if err := pc.SetRemoteDescription(offer); err != nil {
		_ = pc.Close()
		s.setInactive()
		return "", fmt.Errorf("SetRemoteDescription: %w", err)
	}
	answer, err := pc.CreateAnswer(nil)
	if err != nil {
		_ = pc.Close()
		s.setInactive()
		return "", fmt.Errorf("CreateAnswer: %w", err)
	}

	// GatheringCompletePromise MUST come before SetLocalDescription.
	gatherDone := webrtc.GatheringCompletePromise(pc)
	if err := pc.SetLocalDescription(answer); err != nil {
		_ = pc.Close()
		s.setInactive()
		return "", fmt.Errorf("SetLocalDescription: %w", err)
	}

	s.emitLog("Gathering ICE candidates (UDP + TCP/443) via STUN — no data sent to any server...")
	select {
	case <-gatherDone:
	case <-ctx.Done():
		_ = pc.Close()
		s.setInactive()
		return "", ctx.Err()
	}

	encoded, err := encodeSDP(*pc.LocalDescription())
	if err != nil {
		_ = pc.Close()
		s.setInactive()
		return "", fmt.Errorf("encodeSDP: %w", err)
	}

	s.emitLog("Answer ready. Copy it and send it back to the Consumer.")
	s.emitStatus("Status: Answer generated — waiting for Consumer to connect")
	return encoded, nil
}

// ── Internal helpers ───────────────────────────────────────────────────────

func (s *P2PSession) createPC() (*webrtc.PeerConnection, error) {
	se := webrtc.SettingEngine{}
	se.DetachDataChannels()

	// Enable TCP ICE candidates alongside UDP (RFC 6544).
	// TCP candidates on port 443 look identical to HTTPS to firewalls —
	// they pass through corporate/home NATs that block UDP entirely.
	// No relay is involved; this is still 100 % direct peer-to-peer.
	se.SetNetworkTypes([]webrtc.NetworkType{
		webrtc.NetworkTypeUDP4,
		webrtc.NetworkTypeUDP6,
		webrtc.NetworkTypeTCP4,
		webrtc.NetworkTypeTCP6,
	})

	api := webrtc.NewAPI(webrtc.WithSettingEngine(se))

	pc, err := api.NewPeerConnection(webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{{URLs: stunServers}},
	})
	if err != nil {
		return nil, fmt.Errorf("NewPeerConnection: %w", err)
	}
	s.mu.Lock()
	s.pc = pc
	s.mu.Unlock()
	return pc, nil
}

func (s *P2PSession) setupStateChange(pc *webrtc.PeerConnection, done chan struct{}) {
	ports := s.savedPorts
	pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		s.emitLog("P2P state → " + state.String())
		switch state {
		case webrtc.PeerConnectionStateConnected:
			s.emitStatus("Status: DIRECT P2P CONNECTED ✓")
			if ports != "" {
				go startLocalListeners(ports, pc, done, s.OnLog)
			}
		case webrtc.PeerConnectionStateDisconnected,
			webrtc.PeerConnectionStateFailed,
			webrtc.PeerConnectionStateClosed:
			s.mu.Lock()
			s.active = false
			s.mu.Unlock()
			s.emitStatus("Status: Disconnected")
			select {
			case <-done:
			default:
				close(done)
			}
		}
	})
}

func (s *P2PSession) setInactive() {
	s.mu.Lock()
	s.active = false
	s.mu.Unlock()
}

func (s *P2PSession) emitStatus(msg string) {
	if s.OnStatus != nil {
		s.OnStatus(msg)
	}
}

func (s *P2PSession) emitLog(msg string) {
	if s.OnLog != nil {
		s.OnLog(msg)
	}
}

// ── Port listeners ─────────────────────────────────────────────────────────

// splitPorts parses a comma-separated port list and validates each entry.
func splitPorts(raw string) ([]string, error) {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if _, err := strconv.Atoi(p); err != nil {
			return nil, fmt.Errorf("port %q is not a valid number", p)
		}
		out = append(out, p)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no valid ports specified")
	}
	return out, nil
}

// startLocalListeners opens TCP listeners for ports and tunnels every accepted
// connection to the Provider via a DataChannel.  Goroutines exit when done closes.
func startLocalListeners(
	ports string,
	pc *webrtc.PeerConnection,
	done <-chan struct{},
	logFn func(string),
) {
	portList, err := splitPorts(ports)
	if err != nil {
		if logFn != nil {
			logFn("Invalid ports: " + err.Error())
		}
		return
	}

	emit := func(msg string) {
		if logFn != nil {
			logFn(msg)
		}
	}

	for _, port := range portList {
		port := port
		go func() {
			l, err := net.Listen("tcp", ":"+port)
			if err != nil {
				emit("Cannot listen on :" + port + ": " + err.Error())
				return
			}
			go func() { <-done; _ = l.Close() }()

			emit("Listening on localhost:" + port + " — tunnelling to Provider")
			for {
				conn, err := l.Accept()
				if err != nil {
					return
				}
				emit("New connection on :" + port + " → DataChannel to Provider...")
				dc, err := pc.CreateDataChannel(port, nil)
				if err != nil {
					emit("CreateDataChannel(" + port + "): " + err.Error())
					_ = conn.Close()
					continue
				}
				dc.OnOpen(func() {
					raw, err := dc.Detach()
					if err != nil {
						emit("Detach(" + port + "): " + err.Error())
						_ = conn.Close()
						return
					}
					bridge(raw, conn)
				})
			}
		}()
	}
}

// ── bridge ─────────────────────────────────────────────────────────────────

// rwCloser is satisfied by both net.Conn and datachannel.ReadWriteCloser.
type rwCloser = io.ReadWriteCloser

// bridge copies data bi-directionally.  Each side is closed exactly once via
// sync.Once to prevent double-close races.
func bridge(c1, c2 rwCloser) {
	var once1, once2 sync.Once
	close1 := func() { once1.Do(func() { _ = c1.Close() }) }
	close2 := func() { once2.Do(func() { _ = c2.Close() }) }

	go func() {
		_, _ = io.Copy(c1, c2)
		close1()
		close2()
	}()
	go func() {
		_, _ = io.Copy(c2, c1)
		close2()
		close1()
	}()
}
