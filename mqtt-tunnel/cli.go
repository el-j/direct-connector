package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

// ── CLI flag set ──────────────────────────────────────────────────────────────
//
// Every flag also has a DC_<UPPERCASE> environment variable equivalent so the
// binary can be configured entirely through Docker environment variables.
//
// Modes:
//   ssh           — persistent reverse-SSH tunnel (headless TunnelManager)
//   p2p-consumer  — WebRTC consumer: prints offer, reads answer, keeps tunnel alive
//   p2p-provider  — WebRTC provider: reads offer, prints answer, bridges services
//   keygen        — generate the app Ed25519 keypair and print the public key
//
// Usage examples:
//
//	direct-connector --mode ssh --host jump.example.com --port 8080 \
//	  --user admin --ports 1883,3391 --ip 6
//
//	direct-connector --mode p2p-consumer --ports 1883,8080
//	  # → prints Offer to stdout; paste Answer to stdin then press Enter
//
//	direct-connector --mode p2p-provider
//	  # → paste Offer to stdin; Answer is printed to stdout
//
//	direct-connector --mode p2p-consumer --ports 1883 \
//	  --offer-out /data/offer.txt --answer-in /data/answer.txt
//	  # → file-based exchange for Docker Compose shared-volume workflows
//
//	direct-connector --mode keygen
//	  # → generates id_ed25519 in the config dir; prints the public key

// envOr returns the value of the named environment variable, or fallback if unset.
func envOr(name, fallback string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return fallback
}

// IsCLIMode returns true if the invocation looks like a CLI (non-GUI) run.
// We look for --mode / -mode anywhere in os.Args so that Fyne never initialises.
func IsCLIMode() bool {
	for _, a := range os.Args[1:] {
		s := strings.TrimLeft(a, "-")
		if strings.HasPrefix(s, "mode") {
			return true
		}
	}
	// Also triggered by the DC_MODE environment variable.
	return os.Getenv("DC_MODE") != ""
}

// runCLI is the full headless entry point.  It never touches Fyne.
func runCLI() {
	fs := flag.NewFlagSet("direct-connector", flag.ExitOnError)

	mode := fs.String("mode", envOr("DC_MODE", ""), "Mode: ssh | p2p-consumer | p2p-provider | keygen")
	host := fs.String("host", envOr("DC_HOST", ""), "SSH host / DNS name")
	port := fs.String("port", envOr("DC_PORT", "22"), "SSH port")
	user := fs.String("user", envOr("DC_USER", ""), "SSH username")
	ports := fs.String("ports", envOr("DC_PORTS", ""), "Comma-separated ports to forward")
	ip := fs.String("ip", envOr("DC_IP", ""), "IP version: 4 | 6 | \"\" (auto)")
	keyFlag := fs.String("key", envOr("DC_KEY", ""), "Path to SSH private key (uses app key if blank)")

	// P2P file-based exchange flags — useful when piping between containers.
	offerOut := fs.String("offer-out", envOr("DC_OFFER_OUT", ""), "Write Offer SDP to this file (p2p-consumer; default: stdout)")
	answerIn := fs.String("answer-in", envOr("DC_ANSWER_IN", ""), "Read Answer SDP from this file (p2p-consumer; default: stdin)")
	offerIn := fs.String("offer-in", envOr("DC_OFFER_IN", ""), "Read Offer SDP from this file (p2p-provider; default: stdin)")
	answerOut := fs.String("answer-out", envOr("DC_ANSWER_OUT", ""), "Write Answer SDP to this file (p2p-provider; default: stdout)")

	_ = fs.Parse(os.Args[1:])

	logf := func(format string, a ...any) {
		fmt.Fprintf(os.Stderr, "%s  %s\n", time.Now().Format("15:04:05"), fmt.Sprintf(format, a...))
	}

	switch *mode {
	case "ssh":
		cliSSH(logf, *host, *port, *user, *ports, *ip, *keyFlag)
	case "p2p-consumer":
		cliP2PConsumer(logf, *ports, *offerOut, *answerIn)
	case "p2p-provider":
		cliP2PProvider(logf, *offerIn, *answerOut)
	case "keygen":
		cliKeygen(logf)
	default:
		fmt.Fprintln(os.Stderr, "direct-connector CLI")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Usage: direct-connector --mode <mode> [flags]")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Modes:")
		fmt.Fprintln(os.Stderr, "  ssh            Persistent reverse-SSH tunnel")
		fmt.Fprintln(os.Stderr, "  p2p-consumer   WebRTC consumer: opens local ports, tunnels to provider")
		fmt.Fprintln(os.Stderr, "  p2p-provider   WebRTC provider: bridges DataChannels to local services")
		fmt.Fprintln(os.Stderr, "  keygen         Generate app Ed25519 keypair, print public key")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Flags (every flag also reads DC_<FLAG_UPPERCASE> from environment):")
		fs.PrintDefaults()
		os.Exit(1)
	}
}

// ── SSH mode ──────────────────────────────────────────────────────────────────

func cliSSH(logf func(string, ...any), host, port, user, ports, ipVer, keyPath string) {
	if host == "" || user == "" || ports == "" {
		logf("ERROR: --host, --user, and --ports are required for SSH mode")
		os.Exit(1)
	}
	if keyPath == "" && AppKeyExists() {
		keyPath = appPrivateKeyPath()
		logf("Using app key: %s", keyPath)
	}

	cfg := TunnelConfig{
		Host:         host,
		Port:         port,
		ForwardPorts: ports,
		User:         user,
		IPVersion:    ipVer,
		KeyPath:      keyPath,
	}

	if _, err := buildSSHArgs(cfg); err != nil {
		logf("ERROR: invalid configuration: %v", err)
		os.Exit(1)
	}

	logf("Starting SSH tunnel → %s@%s:%s  ports=%s  ip=%s", user, host, port, ports, ipVer)

	tm := &TunnelManager{
		OnStatus: func(s TunnelStatus) { logf("Status: %s", s.String()) },
		OnLog:    func(msg string) { logf("%s", msg) },
	}
	tm.Start(cfg)

	waitForSignal(logf, func() { tm.Stop() })
}

// ── P2P consumer mode ─────────────────────────────────────────────────────────

func cliP2PConsumer(logf func(string, ...any), ports, offerOutFile, answerInFile string) {
	if ports == "" {
		logf("ERROR: --ports is required for p2p-consumer mode")
		os.Exit(1)
	}

	session := NewP2PSession(
		func(s string) { logf("%s", s) },
		func(s string) { logf("%s", s) },
	)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	logf("Generating WebRTC offer (STUN lookup)...")
	offer, err := session.GenerateOffer(ctx, ports)
	if err != nil {
		logf("ERROR: GenerateOffer: %v", err)
		os.Exit(1)
	}

	// ── Write offer ──
	if offerOutFile != "" {
		if err := os.WriteFile(offerOutFile, []byte(offer+"\n"), 0o644); err != nil {
			logf("ERROR: writing offer to %s: %v", offerOutFile, err)
			os.Exit(1)
		}
		logf("Offer written to: %s", offerOutFile)
		logf("Send this file to the Provider, then wait for their answer.")
	} else {
		fmt.Println("=== COPY EVERYTHING BETWEEN THE LINES AND SEND IT TO THE PROVIDER ===")
		fmt.Println(offer)
		fmt.Println("=====================================================================")
	}

	// ── Read answer ──
	var answer string
	if answerInFile != "" {
		logf("Waiting for answer at: %s", answerInFile)
		answer, err = waitForFile(ctx, answerInFile, logf)
		if err != nil {
			logf("ERROR: %v", err)
			os.Exit(1)
		}
	} else {
		fmt.Fprintln(os.Stderr, "Paste the Provider's Answer and press Enter:")
		answer, err = readLine(ctx)
		if err != nil {
			logf("Cancelled.")
			os.Exit(0)
		}
	}

	logf("Applying answer...")
	if err := session.ApplyAnswer(strings.TrimSpace(answer)); err != nil {
		logf("ERROR: ApplyAnswer: %v", err)
		os.Exit(1)
	}

	waitForSignal(logf, func() { session.Stop() })
}

// ── P2P provider mode ─────────────────────────────────────────────────────────

func cliP2PProvider(logf func(string, ...any), offerInFile, answerOutFile string) {
	session := NewP2PSession(
		func(s string) { logf("%s", s) },
		func(s string) { logf("%s", s) },
	)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// ── Read offer ──
	var offer string
	var err error
	if offerInFile != "" {
		logf("Waiting for offer at: %s", offerInFile)
		offer, err = waitForFile(ctx, offerInFile, logf)
		if err != nil {
			logf("ERROR: %v", err)
			os.Exit(1)
		}
	} else {
		fmt.Fprintln(os.Stderr, "Paste the Consumer's Offer and press Enter:")
		offer, err = readLine(ctx)
		if err != nil {
			logf("Cancelled.")
			os.Exit(0)
		}
	}

	logf("Generating WebRTC answer (STUN lookup)...")
	answer, err := session.AcceptOfferAndGenerateAnswer(ctx, strings.TrimSpace(offer))
	if err != nil {
		logf("ERROR: AcceptOfferAndGenerateAnswer: %v", err)
		os.Exit(1)
	}

	// ── Write answer ──
	if answerOutFile != "" {
		if err := os.WriteFile(answerOutFile, []byte(answer+"\n"), 0o644); err != nil {
			logf("ERROR: writing answer to %s: %v", answerOutFile, err)
			os.Exit(1)
		}
		logf("Answer written to: %s", answerOutFile)
		logf("Send this file back to the Consumer.")
	} else {
		fmt.Println("=== COPY EVERYTHING BETWEEN THE LINES AND SEND IT BACK TO THE CONSUMER ===")
		fmt.Println(answer)
		fmt.Println("===========================================================================")
	}

	waitForSignal(logf, func() { session.Stop() })
}

// ── Keygen mode ───────────────────────────────────────────────────────────────

func cliKeygen(logf func(string, ...any)) {
	logf("Generating Ed25519 keypair in: %s", appKeyDir())
	if err := GenerateAppKey(); err != nil {
		logf("ERROR: %v", err)
		os.Exit(1)
	}
	pub, err := LoadAppPublicKey()
	if err != nil {
		logf("ERROR: %v", err)
		os.Exit(1)
	}
	logf("Private key: %s", appPrivateKeyPath())
	logf("Public key:  %s", appPublicKeyPath())
	fmt.Println("")
	fmt.Println("Add the following line to the server's ~/.ssh/authorized_keys:")
	fmt.Println("")
	fmt.Println(pub)
}

// ── Helpers ───────────────────────────────────────────────────────────────────

// waitForSignal blocks until SIGINT or SIGTERM, then calls cleanup.
func waitForSignal(logf func(string, ...any), cleanup func()) {
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	logf("Running — press Ctrl+C to stop.")
	<-sig
	logf("Shutting down...")
	cleanup()
}

// waitForFile polls path every 2 seconds until the file exists or ctx is cancelled.
func waitForFile(ctx context.Context, path string, logf func(string, ...any)) (string, error) {
	for {
		data, err := os.ReadFile(path)
		if err == nil {
			return strings.TrimSpace(string(data)), nil
		}
		if !os.IsNotExist(err) {
			return "", fmt.Errorf("read %s: %w", path, err)
		}
		select {
		case <-ctx.Done():
			return "", fmt.Errorf("cancelled while waiting for %s", path)
		case <-time.After(2 * time.Second):
			logf("Still waiting for %s...", path)
		}
	}
}

// readLine reads one line from stdin, unblocking if ctx is cancelled.
func readLine(ctx context.Context) (string, error) {
	ch := make(chan string, 1)
	errCh := make(chan error, 1)
	go func() {
		scanner := bufio.NewScanner(os.Stdin)
		if scanner.Scan() {
			ch <- scanner.Text()
		} else {
			if err := scanner.Err(); err != nil {
				errCh <- err
			} else {
				errCh <- fmt.Errorf("EOF")
			}
		}
	}()
	select {
	case line := <-ch:
		return line, nil
	case err := <-errCh:
		return "", err
	case <-ctx.Done():
		return "", ctx.Err()
	}
}
