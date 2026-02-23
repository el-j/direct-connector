// FILE: app-wails/internal/p2p/p2p.go
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
	"time"

	pionice "github.com/pion/ice/v2"
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
	// UDP STUN
	"stun:stun.cloudflare.com:3478",
	"stun:stun.nextcloud.com:443", // UDP on port 443 (QUIC-friendly)
	// TCP STUN — required to gather server-reflexive TCP candidates
	// when TCPMuxPort is set.  Cloudflare STUN reliably supports TCP on 3478.
	"stun:stun.cloudflare.com:3478?transport=tcp",
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
	mu          sync.Mutex
	pc          *webrtc.PeerConnection
	done        chan struct{}
	active      bool
	savedPorts  string // set by GenerateOffer; used when Connected fires
	cfg         SessionConfig
	tcpListener net.Listener // non-nil when TCPMuxPort > 0 and bind succeeded

	OnStatus func(string)
	OnLog    func(string)
}

// NewP2PSession returns a fresh, idle session with default ICE parameters.
func NewP2PSession(onStatus func(string), onLog func(string)) *P2PSession {
	return NewP2PSessionWithConfig(SessionConfig{}, onStatus, onLog)
}

// NewP2PSessionWithConfig returns a fresh, idle session with the supplied ICE
// tuning parameters.  Use NewP2PSession when the defaults are sufficient.
func NewP2PSessionWithConfig(cfg SessionConfig, onStatus func(string), onLog func(string)) *P2PSession {
	return &P2PSession{cfg: cfg, OnStatus: onStatus, OnLog: onLog}
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
	ln := s.tcpListener
	s.tcpListener = nil
	s.mu.Unlock()

	if pc != nil {
		_ = pc.Close()
	}
	if ln != nil {
		_ = ln.Close()
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

	s.emitStatus("Status: Gathering ICE candidates — this takes up to 30 s\u2026")
	s.emitLog("Gathering ICE candidates (UDP + TCP/443) via STUN — no data sent to any server...")
	timer := time.NewTimer(s.cfg.gatherTimeout())
	defer timer.Stop()
	select {
	case <-gatherDone:
	case <-timer.C:
		s.emitLog(fmt.Sprintf("ICE gather timed out after %.0f s — using candidates collected so far",
			s.cfg.gatherTimeout().Seconds()))
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

	s.emitStatus("Status: Gathering ICE candidates — this takes up to 30 s\u2026")
	s.emitLog("Gathering ICE candidates (UDP + TCP/443) via STUN — no data sent to any server...")
	answerTimer := time.NewTimer(s.cfg.gatherTimeout())
	defer answerTimer.Stop()
	select {
	case <-gatherDone:
	case <-answerTimer.C:
		s.emitLog(fmt.Sprintf("ICE gather timed out after %.0f s — using candidates collected so far",
			s.cfg.gatherTimeout().Seconds()))
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

	// ICE keepalive/timeout tuning: detect dead paths quickly while
	// keeping overhead low on long-idle tunnels.
	// disconnectedTimeout=5s, failedTimeout=25s, keepAliveInterval=2s
	se.SetICETimeouts(5*time.Second, 25*time.Second, 2*time.Second)

	// Optional: bind a fixed TCP port for ICE-TCP so the listen address is
	// predictable (port 443 passes strict firewalls as "HTTPS-like").
	if s.cfg.TCPMuxPort > 0 {
		tcpLn, err := net.Listen("tcp4", fmt.Sprintf(":%d", s.cfg.TCPMuxPort))
		if err != nil {
			s.emitLog(fmt.Sprintf("TCPMux: cannot bind :%d — %v (continuing without fixed TCP port)",
				s.cfg.TCPMuxPort, err))
		} else {
			mux := pionice.NewTCPMuxDefault(pionice.TCPMuxParams{
				Listener:       tcpLn,
				Logger:         nil,
				ReadBufferSize: 8,
			})
			se.SetICETCPMux(mux)
			s.mu.Lock()
			s.tcpListener = tcpLn
			s.mu.Unlock()
		}
	}

	// Build the ICE server list: always include diverse STUN servers, and
	// append any user-configured TURN relays (for symmetric NAT / CGNAT).
	iceServers := []webrtc.ICEServer{{URLs: stunServers}}
	for _, t := range s.cfg.TURNServers {
		iceServers = append(iceServers, webrtc.ICEServer{
			URLs:           []string{t.URL},
			Username:       t.Username,
			Credential:     t.Credential,
			CredentialType: webrtc.ICECredentialTypePassword,
		})
	}

	api := webrtc.NewAPI(webrtc.WithSettingEngine(se))

	pc, err := api.NewPeerConnection(webrtc.Configuration{
		ICEServers: iceServers,
	})
	if err != nil {
		return nil, fmt.Errorf("NewPeerConnection: %w", err)
	}

	// Log each candidate as it is gathered so the user can see what paths
	// are available (host, srflx via STUN, relay via TURN).
	pc.OnICECandidate(func(c *webrtc.ICECandidate) {
		if c == nil {
			return // nil signals gathering complete
		}
		s.emitLog(fmt.Sprintf("ICE candidate: type=%-4s  proto=%-3s  addr=%s:%d",
			c.Typ, c.Protocol, c.Address, c.Port))
	})

	pc.OnICEGatheringStateChange(func(state webrtc.ICEGathererState) {
		s.emitLog(fmt.Sprintf("ICE gathering state \u2192 %s", state.String()))
	})

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
		case webrtc.PeerConnectionStateFailed:
			s.emitLog("⚠  ICE Failed — all candidate pairs were rejected.")
			s.emitLog("   If both sides are behind symmetric NAT / CGNAT, you MUST use a TURN relay.")
			s.emitLog("   Start the built-in relay on the machine with a public IP (or port-forward),")
			s.emitLog("   then copy its URL+credentials to the Advanced/TURN section on the OTHER machine.")
			s.mu.Lock()
			s.active = false
			s.mu.Unlock()
			s.emitStatus("Status: Failed — enable TURN relay and retry")
			select {
			case <-done:
			default:
				close(done)
			}
		case webrtc.PeerConnectionStateDisconnected,
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
