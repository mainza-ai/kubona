package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/mainza-ai/kubona/core/pkg/api"
	"github.com/mainza-ai/kubona/core/pkg/jobs"
	"github.com/mainza-ai/kubona/core/pkg/ledger"
	"github.com/mainza-ai/kubona/core/pkg/session"
	"github.com/mainza-ai/kubona/core/pkg/supervisor"
	"github.com/mainza-ai/kubona/core/pkg/watchdog"
	pb "github.com/mainza-ai/kubona/core/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	// Operator subcommands run before flag parsing: `kubonad grants ...`
	// mints/attenuates/verifies cross-lab UCAN grants with operator-held keys.
	if len(os.Args) > 1 && os.Args[1] == "grants" {
		os.Exit(runGrants(os.Args[2:]))
	}

	port := flag.Int("port", 8080, "HTTP API gateway port")
	dbPath := flag.String("db", "kubona.db", "Path to SQLite WAL ledger")
	bridgeAddr := flag.String("bridge", "localhost:50051", "Python gRPC bridge address")
	flag.Parse()

	log.Printf("[Kubona Core] Starting orchestrator daemon on port %d...", *port)

	// 1. Initialize SQLite WAL Ledger
	l, err := ledger.OpenLedger(*dbPath)
	if err != nil {
		log.Fatalf("Failed to open ledger: %v", err)
	}
	defer l.Close()

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
	jobRunner.Start()
	defer jobRunner.Stop()

	loopSupervisor := supervisor.NewSupervisor(l)
	taskWatchdog := watchdog.NewTaskWatchdog(l, loopSupervisor)

	// 4. Start HTTP / REST / WebSocket Gateway
	server := api.NewServer(l, sessionMgr, jobRunner, loopSupervisor, taskWatchdog)
	httpServer := &http.Server{
		Addr:         fmt.Sprintf(":%d", *port),
		Handler:      server.Handler(),
		ReadTimeout:  120 * time.Second,
		WriteTimeout: 300 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	go func() {
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP server failure: %v", err)
		}
	}()

	log.Printf("[Kubona Core] Daemon running at http://localhost:%d", *port)

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
