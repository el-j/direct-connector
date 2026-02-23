// FILE: app-wails/internal/p2p/config.go
package p2p

import "time"

// TURNServer holds credentials for a single TURN relay endpoint.
type TURNServer struct {
	URL        string // e.g. "turn:relay.example.com:3478?transport=udp"
	Username   string
	Credential string // plain-text password
}

// SessionConfig holds per-session ICE tuning parameters.
// Zero values apply good defaults (30 s gather timeout, STUN only, no TCP mux).
type SessionConfig struct {
	// GatherTimeout caps the ICE candidate gathering phase.
	// Default (zero) = 30 seconds.
	GatherTimeout time.Duration

	// TURNServers is an optional list of TURN relay servers.
	// When provided, relay (TURN) ICE candidates are gathered in addition
	// to host and srflx candidates, enabling traversal of symmetric NATs and CGNAT.
	TURNServers []TURNServer

	// TCPMuxPort, if > 0, binds a passive ICE-TCP listener on that port.
	// Port 443 maximises corporate firewall penetration.
	// 0 = TCP candidates use OS-assigned ephemeral ports (still works, just less
	// likely to pass strict firewalls).
	TCPMuxPort int
}

// defaultGatherTimeout is used when SessionConfig.GatherTimeout is zero.
const defaultGatherTimeout = 30 * time.Second

func (c SessionConfig) gatherTimeout() time.Duration {
	if c.GatherTimeout <= 0 {
		return defaultGatherTimeout
	}
	return c.GatherTimeout
}
