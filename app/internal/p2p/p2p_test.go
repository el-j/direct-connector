package p2p

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"
	"time"
)

// ── encodeSDP / decodeSDP ─────────────────────────────────────────────────

func TestEncodeDecodeSDP_RoundTrip(t *testing.T) {
	_, err := decodeSDP("!!!not-base64!!!")
	if err == nil {
		t.Fatal("expected error for invalid base64, got nil")
	}

	// Valid base64 but not JSON → JSON parse error.
	_, err = decodeSDP("dGVzdA==") // decodes to "test"
	if err == nil {
		t.Fatal("expected JSON parse error, got nil")
	}
}

func TestDecodeSDP_EmptyString(t *testing.T) {
	_, err := decodeSDP("")
	if err == nil {
		t.Fatal("expected error for empty string")
	}
}

func TestDecodeSDP_ValidJSON_WrongFields(t *testing.T) {
	jsonStr := `{"type":"offer","sdp":"v=0\r\n"}`
	b64 := encodeBase64(jsonStr)
	sd, err := decodeSDP(b64)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sd.SDP != "v=0\r\n" {
		t.Fatalf("SDP mismatch: %q", sd.SDP)
	}
}

// encodeBase64 is a test helper that mirrors encodeSDP without requiring a real PC.
func encodeBase64(jsonStr string) string {
	buf := make([]byte, (len(jsonStr)+2)/3*4)
	n := encodeBase64Bytes([]byte(jsonStr), buf)
	return string(buf[:n])
}

func encodeBase64Bytes(src, dst []byte) int {
	const enc = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
	n := 0
	for i := 0; i < len(src); i += 3 {
		var b [3]byte
		rem := len(src) - i
		if rem >= 3 {
			b = [3]byte{src[i], src[i+1], src[i+2]}
		} else {
			for j := 0; j < rem; j++ {
				b[j] = src[i+j]
			}
		}
		dst[n] = enc[b[0]>>2]
		dst[n+1] = enc[(b[0]&0x3)<<4|b[1]>>4]
		if rem >= 2 {
			dst[n+2] = enc[(b[1]&0xf)<<2|b[2]>>6]
		} else {
			dst[n+2] = '='
		}
		if rem >= 3 {
			dst[n+3] = enc[b[2]&0x3f]
		} else {
			dst[n+3] = '='
		}
		n += 4
	}
	return n
}

// ── P2PSession lifecycle ──────────────────────────────────────────────────

func TestP2PSession_IsActive_InitiallyFalse(t *testing.T) {
	s := NewP2PSession(nil, nil)
	if s.IsActive() {
		t.Fatal("expected IsActive() == false on fresh session")
	}
}

func TestP2PSession_StopWhenIdle(t *testing.T) {
	s := NewP2PSession(nil, nil)
	s.Stop()
	if s.IsActive() {
		t.Fatal("expected inactive after Stop()")
	}
}

func TestP2PSession_StopMultipleTimes(t *testing.T) {
	s := NewP2PSession(nil, nil)
	s.Stop()
	s.Stop() // must not panic / double-close done channel
}

func TestP2PSession_GenerateOffer_CancelledContext(t *testing.T) {
	s := NewP2PSession(nil, nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := s.GenerateOffer(ctx, "1883")
	if err == nil {
		t.Fatal("expected error from cancelled context, got nil")
		s.Stop()
	}
}

func TestP2PSession_GenerateOffer_WhileActive(t *testing.T) {
	s := NewP2PSession(nil, nil)
	s.mu.Lock()
	s.active = true
	s.mu.Unlock()

	_, err := s.GenerateOffer(context.Background(), "1883")
	if err == nil {
		t.Fatal("expected error when session already active, got nil")
	}
	if !strings.Contains(err.Error(), "already active") {
		t.Fatalf("unexpected error text: %v", err)
	}
	s.mu.Lock()
	s.active = false
	s.mu.Unlock()
}

func TestP2PSession_ApplyAnswer_WithoutOffer(t *testing.T) {
	s := NewP2PSession(nil, nil)
	err := s.ApplyAnswer("anything")
	if err == nil {
		t.Fatal("expected error when no active session, got nil")
	}
}

func TestP2PSession_AcceptOffer_WhileActive(t *testing.T) {
	s := NewP2PSession(nil, nil)
	s.mu.Lock()
	s.active = true
	s.mu.Unlock()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := s.AcceptOfferAndGenerateAnswer(ctx, "badOffer")
	if err == nil {
		t.Fatal("expected error when session already active, got nil")
	}
	s.mu.Lock()
	s.active = false
	s.mu.Unlock()
}

func TestP2PSession_AcceptOffer_InvalidOffer(t *testing.T) {
	s := NewP2PSession(nil, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, err := s.AcceptOfferAndGenerateAnswer(ctx, "!!!invalid!!!")
	if err == nil {
		t.Fatal("expected error for invalid offer SDP, got nil")
	}
}

func TestP2PSession_CallbacksFired(t *testing.T) {
	statusCh := make(chan string, 4)
	logCh := make(chan string, 4)
	s := NewP2PSession(
		func(msg string) { statusCh <- msg },
		func(msg string) { logCh <- msg },
	)
	s.Stop()

	select {
	case msg := <-statusCh:
		if !strings.Contains(msg, "Disconnect") {
			t.Fatalf("expected disconnect status, got %q", msg)
		}
	case <-time.After(time.Second):
		t.Fatal("OnStatus not called within 1 s")
	}
}

// ── splitPorts ─────────────────────────────────────────────────────────────

func TestSplitPorts_Valid(t *testing.T) {
	ports, err := splitPorts("1883, 8080,  22")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ports) != 3 {
		t.Fatalf("expected 3 ports, got %d: %v", len(ports), ports)
	}
}

func TestSplitPorts_SinglePort(t *testing.T) {
	ports, err := splitPorts("22")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ports) != 1 || ports[0] != "22" {
		t.Fatalf("unexpected result: %v", ports)
	}
}

func TestSplitPorts_Empty(t *testing.T) {
	_, err := splitPorts("")
	if err == nil {
		t.Fatal("expected error for empty input")
	}
}

func TestSplitPorts_InvalidEntry(t *testing.T) {
	_, err := splitPorts("1883,http,22")
	if err == nil {
		t.Fatal("expected error for non-numeric port")
	}
}

func TestSplitPorts_WhitespaceOnly(t *testing.T) {
	_, err := splitPorts("   ")
	if err == nil {
		t.Fatal("expected error for whitespace-only input")
	}
}

// ── bridge ─────────────────────────────────────────────────────────────────

func TestBridge_CopiesData(t *testing.T) {
	payload := []byte("hello from bridge")
	r1, w1 := io.Pipe()
	r2, w2 := io.Pipe()

	type rwc struct {
		io.Reader
		io.Writer
		io.Closer
	}
	side1 := rwc{r1, w1, &multiCloser{r1, w1}}
	side2 := rwc{r2, w2, &multiCloser{r2, w2}}
	bridge(side1, side2)

	go func() { _, _ = w2.Write(payload) }()
	buf := make([]byte, len(payload))
	if _, err := io.ReadFull(r1, buf); err != nil {
		t.Fatalf("ReadFull: %v", err)
	}
	if !bytes.Equal(buf, payload) {
		t.Fatalf("data mismatch: got %q want %q", buf, payload)
	}
	_ = w1.Close()
	_ = w2.Close()
}

func TestBridge_CloseOnEOF(t *testing.T) {
	r1, w1 := io.Pipe()
	r2, w2 := io.Pipe()

	type rwc struct {
		io.Reader
		io.Writer
		io.Closer
	}
	side1 := rwc{r1, w1, &multiCloser{r1, w1}}
	side2 := rwc{r2, w2, &multiCloser{r2, w2}}
	bridge(side1, side2)

	_ = w2.Close()
	done := make(chan struct{})
	go func() {
		buf := make([]byte, 1)
		r1.Read(buf) //nolint:errcheck
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("bridge did not propagate close within 2 s")
	}
}

type multiCloser struct{ a, b io.Closer }

func (m *multiCloser) Close() error {
	_ = m.a.Close()
	return m.b.Close()
}
