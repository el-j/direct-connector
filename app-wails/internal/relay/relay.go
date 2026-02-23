package relay

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/pion/logging"
	"github.com/pion/stun"
	"github.com/pion/turn/v2"
)

// Relay manages an embedded TURN server.
// One instance per application \u2014 do not create multiple.
// Safe for concurrent use after construction.
type Relay struct {
	mu          sync.Mutex
	server      *turn.Server
	listener    net.PacketConn
	tcpListener net.Listener // non-nil when TCP listener started successfully
	port        int
	publicIP    string
	realm       string
	secret      string // HMAC secret for time-limited credentials
	OnLog       func(string)
}

// New returns a new idle Relay.
func New(onLog func(string)) *Relay {
	if onLog == nil {
		onLog = func(string) {}
	}
	return &Relay{
		realm: "direct-connector",
		OnLog: onLog,
	}
}

// Start starts the TURN server on the given UDP port.
// It discovers the machine's public IP via a STUN query to stun.l.google.com:19302.
// Returns an error if the port is in use or the STUN query fails.
func (r *Relay) Start(port int) error {
	r.mu.Lock()
	if r.server != nil {
		r.mu.Unlock()
		return fmt.Errorf("relay: already running")
	}

	// Generate a random 16-byte hex secret.
	secretBytes := make([]byte, 16)
	if _, err := rand.Read(secretBytes); err != nil {
		r.mu.Unlock()
		return fmt.Errorf("relay: generate secret: %w", err)
	}
	secret := hex.EncodeToString(secretBytes)

	// Discover public IP; unlock before blocking I/O.
	r.mu.Unlock()
	publicIP, err := discoverPublicIP()
	if err != nil {
		return fmt.Errorf("relay: discover public IP: %w", err)
	}
	r.mu.Lock()

	// Re-check: someone else may have called Start while we were unlocked.
	if r.server != nil {
		r.mu.Unlock()
		return fmt.Errorf("relay: already running")
	}

	// Open UDP packet conn.
	udpConn, err := net.ListenPacket("udp4", fmt.Sprintf(":%d", port))
	if err != nil {
		r.mu.Unlock()
		return fmt.Errorf("relay: listen UDP port %d: %w", port, err)
	}

	// Also open a TCP listener so TURN-over-TCP works for clients behind
	// firewalls that block UDP.  Failure is non-fatal: UDP-only TURN still
	// works in most home/office setups.
	var tcpLn net.Listener
	tcpLn, tcpErr := net.Listen("tcp4", fmt.Sprintf(":%d", port))
	if tcpErr != nil {
		r.OnLog(fmt.Sprintf("TURN TCP listener failed (UDP-only fallback): %v", tcpErr))
	}

	// Assemble the server config; add TCP listener only when available.
	cfg := turn.ServerConfig{
		Realm:       r.realm,
		AuthHandler: turn.NewLongTermAuthHandler(secret, nil),
		PacketConnConfigs: []turn.PacketConnConfig{
			{
				PacketConn: udpConn,
				RelayAddressGenerator: &turn.RelayAddressGeneratorStatic{
					RelayAddress: net.ParseIP(publicIP),
					Address:      "0.0.0.0",
				},
			},
		},
		LoggerFactory: logging.NewDefaultLoggerFactory(),
	}
	if tcpLn != nil {
		cfg.ListenerConfigs = []turn.ListenerConfig{
			{
				Listener: tcpLn,
				RelayAddressGenerator: &turn.RelayAddressGeneratorStatic{
					RelayAddress: net.ParseIP(publicIP),
					Address:      "0.0.0.0",
				},
			},
		}
	}

	// Create TURN server. LoggerFactory must be non-nil per pion/turn contract.
	srv, err := turn.NewServer(cfg)
	if err != nil {
		udpConn.Close() //nolint:errcheck
		if tcpLn != nil {
			tcpLn.Close() //nolint:errcheck
		}
		r.mu.Unlock()
		return fmt.Errorf("relay: create TURN server: %w", err)
	}

	r.server = srv
	r.listener = udpConn
	r.tcpListener = tcpLn
	r.port = port
	r.publicIP = publicIP
	r.secret = secret
	r.mu.Unlock()

	r.OnLog(fmt.Sprintf("TURN relay started on 0.0.0.0:%d (public IP: %s)", port, publicIP))
	return nil
}

// Stop shuts down the TURN server and releases the UDP and TCP ports.
// Safe to call when not running.
func (r *Relay) Stop() {
	r.mu.Lock()
	if r.server == nil {
		r.mu.Unlock()
		return
	}

	r.server.Close()   //nolint:errcheck
	r.listener.Close() //nolint:errcheck
	if r.tcpListener != nil {
		r.tcpListener.Close() //nolint:errcheck
		r.tcpListener = nil
	}
	r.server = nil
	r.listener = nil
	r.port = 0
	r.publicIP = ""
	r.secret = ""
	r.mu.Unlock()

	r.OnLog("TURN relay stopped")
}

// IsRunning reports whether the TURN server is currently running.
func (r *Relay) IsRunning() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.server != nil
}

// PublicIP returns the public IP discovered when the relay was last started.
// Returns "" when the relay is not running.
func (r *Relay) PublicIP() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.publicIP
}

// TURNAddr returns a TURN URL in the form "turn:HOST:PORT?transport=udp".
// HOST is the public IP discovered at Start() time.
// Returns "" when not running.
func (r *Relay) TURNAddr() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.server == nil {
		return ""
	}
	return fmt.Sprintf("turn:%s:%d?transport=udp", r.publicIP, r.port)
}

// TURNTCPAddr returns a TURN URL in the form "turn:HOST:PORT?transport=tcp".
// Returns "" when not running or when the TCP listener failed to start.
func (r *Relay) TURNTCPAddr() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.server == nil || r.tcpListener == nil {
		return ""
	}
	return fmt.Sprintf("turn:%s:%d?transport=tcp", r.publicIP, r.port)
}

// Credentials generates a pair of time-limited TURN credentials valid for
// the next 24 hours. The credentials use the HMAC-SHA1 long-term mechanism
// described in RFC 5766 \u00a710.2 and pion/turn's GenerateLongTermCredentials.
// Returns ("", "") when not running.
func (r *Relay) Credentials() (username, password string) {
	r.mu.Lock()
	secret := r.secret
	running := r.server != nil
	r.mu.Unlock()

	if !running {
		return "", ""
	}

	u, p, err := turn.GenerateLongTermCredentials(secret, 24*time.Hour)
	if err != nil {
		r.OnLog(fmt.Sprintf("relay: generate credentials: %v", err))
		return "", ""
	}
	return u, p
}

// discoverPublicIP attempts to discover the machine's public IP via STUN.
// Falls back to outbound local IP on failure.
func discoverPublicIP() (string, error) {
	c, err := stun.Dial("udp4", "stun.l.google.com:19302")
	if err == nil {
		defer c.Close() //nolint:errcheck
		var xorAddr stun.XORMappedAddress
		var stunInner error
		doErr := c.Do(stun.MustBuild(stun.TransactionID, stun.BindingRequest), func(res stun.Event) {
			if res.Error != nil {
				stunInner = res.Error
				return
			}
			_ = xorAddr.GetFrom(res.Message)
		})
		if doErr == nil && stunInner == nil && xorAddr.IP != nil {
			return xorAddr.IP.String(), nil
		}
	}

	// Fallback: derive outbound IP from a UDP dial (no data sent).
	conn, err := net.Dial("udp4", "8.8.8.8:80")
	if err != nil {
		return "", fmt.Errorf("relay: fallback IP discovery: %w", err)
	}
	defer conn.Close() //nolint:errcheck
	return conn.LocalAddr().(*net.UDPAddr).IP.String(), nil
}
