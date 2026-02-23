package relay

import (
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/pion/logging"
	"github.com/pion/turn/v2"
)

// fakePacketConn is a minimal net.PacketConn that blocks ReadFrom until Close.
type fakePacketConn struct {
	done chan struct{}
}

func newFakePacketConn() *fakePacketConn {
	return &fakePacketConn{done: make(chan struct{})}
}

func (f *fakePacketConn) ReadFrom(p []byte) (int, net.Addr, error) {
	<-f.done
	return 0, nil, net.ErrClosed
}

func (f *fakePacketConn) WriteTo(p []byte, addr net.Addr) (int, error) { return 0, nil }

func (f *fakePacketConn) Close() error {
	select {
	case <-f.done:
	default:
		close(f.done)
	}
	return nil
}

func (f *fakePacketConn) LocalAddr() net.Addr                 { return &net.UDPAddr{} }
func (f *fakePacketConn) SetDeadline(t time.Time) error       { return nil }
func (f *fakePacketConn) SetReadDeadline(t time.Time) error   { return nil }
func (f *fakePacketConn) SetWriteDeadline(t time.Time) error  { return nil }

// newTestTURNServer creates a minimal real *turn.Server backed by fakePacketConn.
func newTestTURNServer(t *testing.T, fc *fakePacketConn, relayIP string) *turn.Server {
	t.Helper()
	srv, err := turn.NewServer(turn.ServerConfig{
		Realm: "test",
		AuthHandler: func(username, realm string, srcAddr net.Addr) ([]byte, bool) {
			return nil, false
		},
		PacketConnConfigs: []turn.PacketConnConfig{
			{
				PacketConn: fc,
				RelayAddressGenerator: &turn.RelayAddressGeneratorStatic{
					RelayAddress: net.ParseIP(relayIP),
					Address:      "0.0.0.0",
				},
			},
		},
		LoggerFactory: logging.NewDefaultLoggerFactory(),
	})
	if err != nil {
		t.Fatalf("newTestTURNServer: %v", err)
	}
	return srv
}

// injectRunning sets up r as if it were running, without real network I/O.
func injectRunning(t *testing.T, r *Relay, host string, port int, secret string) *fakePacketConn {
	t.Helper()
	fc := newFakePacketConn()
	srv := newTestTURNServer(t, fc, host)
	r.mu.Lock()
	r.server = srv
	r.listener = fc
	r.port = port
	r.publicIP = host
	r.secret = secret
	r.mu.Unlock()
	return fc
}

// --- Tests -----------------------------------------------------------------

func TestNew_NotRunning(t *testing.T) {
	r := New(nil)
	if r.IsRunning() {
		t.Fatal("expected not running after New()")
	}
}

func TestStop_WhenNotRunning(t *testing.T) {
	r := New(nil)
	r.Stop() // must not panic
}

func TestTURNAddr_WhenNotRunning(t *testing.T) {
	r := New(nil)
	if addr := r.TURNAddr(); addr != "" {
		t.Fatalf("expected empty TURNAddr, got %q", addr)
	}
}

func TestCredentials_WhenNotRunning(t *testing.T) {
	r := New(nil)
	u, p := r.Credentials()
	if u != "" || p != "" {
		t.Fatalf("expected empty credentials, got %q / %q", u, p)
	}
}

func TestIsRunning_StateTransitions(t *testing.T) {
	r := New(nil)
	injectRunning(t, r, "127.0.0.1", 3478, "testsecret")

	if !r.IsRunning() {
		t.Fatal("expected IsRunning() == true after injection")
	}

	r.Stop()

	if r.IsRunning() {
		t.Fatal("expected IsRunning() == false after Stop()")
	}
}

func TestNew_DefaultRealm(t *testing.T) {
	r := New(nil)
	if r.realm != "direct-connector" {
		t.Fatalf("expected realm %q, got %q", "direct-connector", r.realm)
	}
}

func TestCredentials_Format(t *testing.T) {
	const testSecret = "supersecrettestkey"
	const testHost = "1.2.3.4"
	const testPort = 3478

	r := New(nil)
	injectRunning(t, r, testHost, testPort, testSecret)
	defer r.Stop()

	username, password := r.Credentials()
	if username == "" {
		t.Fatal("expected non-empty username")
	}
	if password == "" {
		t.Fatal("expected non-empty password")
	}

	// pion/turn v2 username format: a plain Unix expiry timestamp string.
	if _, err := strconv.ParseInt(username, 10, 64); err != nil {
		t.Fatalf("expected username to be a numeric timestamp, got %q: %v", username, err)
	}

	// Cross-verify: GenerateLongTermCredentials with the same secret also works.
	u2, p2, err := turn.GenerateLongTermCredentials(testSecret, 24*time.Hour)
	if err != nil {
		t.Fatalf("GenerateLongTermCredentials: %v", err)
	}
	if u2 == "" || p2 == "" {
		t.Fatal("direct GenerateLongTermCredentials returned empty values")
	}
}
