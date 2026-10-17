package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/mainza-ai/kubona/core/pkg/api"
	"github.com/mainza-ai/kubona/core/pkg/disk"
	"github.com/mainza-ai/kubona/core/pkg/hf"
	"github.com/mainza-ai/kubona/core/pkg/jobs"
	"github.com/mainza-ai/kubona/core/pkg/ledger"
	"github.com/mainza-ai/kubona/core/pkg/qir"
	"github.com/mainza-ai/kubona/core/pkg/session"
	"github.com/mainza-ai/kubona/core/pkg/supervisor"
	"github.com/mainza-ai/kubona/core/pkg/watchdog"
	pb "github.com/mainza-ai/kubona/core/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// loadDotEnv reads KEY=VALUE lines from ./.env (repo, gitignored) then
// ~/.kubona/.env into the process environment without overriding
// already-set vars. No third-party dependency; production uses real env /
// secret manager instead.
func loadDotEnv() {
	paths := []string{".env"}
	if home, err := os.UserHomeDir(); err == nil {
		paths = append(paths, filepath.Join(home, ".kubona", ".env"))
	}
	for _, p := range paths {
		loadDotEnvFile(p)
	}
}

// isLoopback reports whether a bind address is loopback-only.
func isLoopback(host string) bool {
	h := strings.Trim(strings.TrimSpace(host), "[]")
	return h == "127.0.0.1" || h == "::1" || strings.ToLower(h) == "localhost"
}

// validateServeConfig enforces the TLS posture before any socket opens
// (K6: "non-local binds refuse without TLS material", strict mode).
//   - cert without key (or vice versa): refuse — half material serves nothing.
//   - --tls-required without both files present: refuse.
//   - cert/key paths that don't exist: refuse early with the path named
//     (rather than crashing mid-serve inside ListenAndServeTLS).
// Plain HTTP on loopback, or declared-LAN plaintext, passes validation
// (the LAN case logs a warning at boot instead).
func validateServeConfig(bind, cert, key string, required bool) error {
	if (cert == "") != (key == "") {
		return fmt.Errorf("tls-cert and tls-key must be set together")
	}
	if required && (cert == "" || key == "") {
		return fmt.Errorf("--tls-required needs --tls-cert and --tls-key")
	}
	for label, path := range map[string]string{"tls-cert": cert, "tls-key": key} {
		if path == "" {
			continue
		}
		fi, err := os.Stat(path)
		if err != nil {
			return fmt.Errorf("%s file unreadable %q: %v", label, path, err)
		}
		if fi.IsDir() {
			return fmt.Errorf("%s path %q is a directory", label, path)
		}
	}
	return nil
}

func loadDotEnvFile(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		kv := strings.SplitN(line, "=", 2)
		if len(kv) != 2 {
			continue
		}
		k := strings.TrimSpace(kv[0])
		v := strings.Trim(strings.TrimSpace(kv[1]), `"'`)
		if k == "" || os.Getenv(k) != "" {
			continue
		}
		_ = os.Setenv(k, v)
	}
}

func main() {
	// Operator subcommands run before flag parsing: `kubonad grants ...`
	// mints/attenuates/verifies cross-lab UCAN grants with operator-held keys.
	loadDotEnv()
	if len(os.Args) > 1 && os.Args[1] == "grants" {
		os.Exit(runGrants(os.Args[2:]))
	}
	if len(os.Args) > 1 && os.Args[1] == "verify" {
		os.Exit(runVerify(os.Args[2:]))
	}

	port := flag.Int("port", 8080, "HTTP API gateway port")
	dbPath := flag.String("db", "", "Path to SQLite WAL ledger (default: ~/.kubona/kubona.db)")
	bridgeAddr := flag.String("bridge", "localhost:50051", "Python gRPC bridge address")
	// K6: bind/TLS posture. Default bind is loopback (fail-closed network
	// posture matching the rest of the edge: bearer 503s, opt-in host
	// fallback). LAN serving is an explicit --bind; TLS serving needs
	// operator-owned cert material; --tls-required refuses non-TLS
	// operation outright. See [[security-and-pqc]] transport decision.
	bindAddr := flag.String("bind", "127.0.0.1", "HTTP bind address (loopback default; 0.0.0.0 only behind TLS or declared LAN)")
	tlsCert := flag.String("tls-cert", "", "TLS certificate file (PEM); with --tls-key enables HTTPS serving")
	tlsKey := flag.String("tls-key", "", "TLS private key file (PEM)")
	tlsRequired := flag.Bool("tls-required", false, "Refuse to serve without TLS material (strict edge posture)")
	flag.Parse()

	if err := validateServeConfig(*bindAddr, *tlsCert, *tlsKey, *tlsRequired); err != nil {
		log.Fatalf("Serve configuration refused: %v", err)
	}

	resolvedDB := *dbPath
	if resolvedDB == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			log.Fatalf("Cannot resolve home directory for default ledger path: %v", err)
		}
		resolvedDB = filepath.Join(home, ".kubona", "kubona.db")
	}

	log.Printf("[Kubona Core] Starting orchestrator daemon on port %d...", *port)
	log.Printf("[Kubona Core] Ledger path: %s", resolvedDB)

	// 1. Initialize SQLite WAL Ledger
	l, err := ledger.OpenLedger(resolvedDB)
	if err != nil {
		log.Fatalf("Failed to open ledger: %v", err)
	}
	defer l.Close()

	// 1b. S2 disk guard: watch the filesystems holding state (DB dir +
	// HF models dir — guarding / while state lives elsewhere is fiction).
	// Thresholds come from env over Q2 defaults; invalid config refuses boot.
	diskThresholds, err := disk.ThresholdsFromEnv()
	if err != nil {
		log.Fatalf("Disk guard configuration refused: %v", err)
	}
	diskPollSeconds, err := disk.PollSecondsFromEnv()
	if err != nil {
		log.Fatalf("Disk guard configuration refused: %v", err)
	}
	diskGuard := disk.NewGuard(diskThresholds,
		[]string{filepath.Dir(resolvedDB), hf.DefaultModelsDir()}, nil,
		func(snap disk.Snapshot) {
			event := "disk_warning"
			if snap.Level == disk.LevelBlocked {
				event = "disk_blocked"
			} else if snap.Level == disk.LevelOK {
				event = "disk_resumed"
			}
			_ = l.AppendEvent("global", event, map[string]any{
				"level": snap.Level, "avail_bytes": snap.AvailBytes,
				"paths": snap.Paths, "reason": snap.Reason,
			})
			log.Printf("[Kubona Core] disk guard: %s (%s)", snap.Level, snap.Reason)
		})
	diskGuard.PollOnce() // fail-closed first reading before serving
	diskStop := make(chan struct{})
	defer close(diskStop)
	go diskGuard.Start(diskStop, time.Duration(diskPollSeconds)*time.Second)
	log.Printf("[Kubona Core] Disk guard watching %s + %s (warn/block/resume %d/%d/%d bytes)",
		filepath.Dir(resolvedDB), hf.DefaultModelsDir(),
		diskThresholds.WarnBytes, diskThresholds.BlockBytes, diskThresholds.ResumeBytes)

	// 1c. S3: reconcile persisted loops. Engines are in-memory, so nothing
	// can be running after a restart — non-terminal rows sleep (history
	// kept, explicit wake only). Terminal rows untouched.
	if reconciled, rerr := l.ReconcileLoopsOnBoot(); rerr != nil {
		log.Fatalf("Loop reconcile refused: %v", rerr)
	} else if reconciled > 0 {
		log.Printf("[Kubona Core] Reconciled %d persisted loops to sleeping", reconciled)
	}

	// 2. Connect to Python Quantum Bridge via gRPC
	conn, err := grpc.NewClient(*bridgeAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("Failed to connect to Python bridge: %v", err)
	}
	defer conn.Close()
	bridgeClient := pb.NewQuantumBridgeServiceClient(conn)

	// 3. Initialize Core Subsystems
	sessionMgr := session.NewManager(l)
	jobRunner := jobs.NewRunner(l, bridgeClient, 4)
	// Compile gate: every circuit_run is analyzed pre-dispatch and the
	// provenance (compiler + fallback flag) persists on the job row.
	jobRunner.SetCompiler(qir.NewCompiler())
	jobRunner.Start()
	defer jobRunner.Stop()

	loopSupervisor := supervisor.NewSupervisor(l)
	taskWatchdog := watchdog.NewTaskWatchdog(l, loopSupervisor)

	// 4. Start HTTP / REST / WebSocket Gateway
	server := api.NewServer(l, sessionMgr, jobRunner, loopSupervisor, taskWatchdog)
	server.SetDiskGuard(diskGuard)
	jobRunner.SetDiskGuard(diskGuard)

	// 4b. S3: idle-loop sweeper (10 min threshold, 60 s poll).
	loopStop := make(chan struct{})
	defer close(loopStop)
	go server.StartLoopSweeper(loopStop)
	httpServer := &http.Server{
		Addr:    fmt.Sprintf("%s:%d", *bindAddr, *port),
		Handler: server.Handler(),
		// Write budget covers uncapped local generations (~6 tok/s can run
		// minutes); the 600s LLM call budget plus margin. Read stays tight
		// (request bodies are small).
		ReadTimeout:  120 * time.Second,
		WriteTimeout: 660 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	useTLS := *tlsCert != "" && *tlsKey != ""
	if !useTLS && !isLoopback(*bindAddr) {
		// Declared-LAN posture (see transport decision): audible, never silent.
		log.Printf("[Kubona Core] WARNING: serving plain HTTP on non-loopback %s — TLS termination belongs at the deployment edge; set --tls-cert/--tls-key or --bind 127.0.0.1", *bindAddr)
	}

	go func() {
		var err error
		if useTLS {
			log.Printf("[Kubona Core] Serving HTTPS on %s", httpServer.Addr)
			err = httpServer.ListenAndServeTLS(*tlsCert, *tlsKey)
		} else {
			err = httpServer.ListenAndServe()
		}
		if err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP server failure: %v", err)
		}
	}()

	scheme := "http"
	if *tlsCert != "" && *tlsKey != "" {
		scheme = "https"
	}
	log.Printf("[Kubona Core] Daemon running at %s://%s:%d", scheme, *bindAddr, *port)

	// Graceful shutdown handling
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	<-sigCh

	log.Println("[Kubona Core] Shutting down gracefully...")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	httpServer.Shutdown(ctx)
	log.Println("[Kubona Core] Clean exit complete.")
}
