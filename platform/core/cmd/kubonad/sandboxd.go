package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/mainza-ai/kubona/core/pkg/sandbox"
)

// kubonad sandboxd --socket /run/kubona-sandbox/sandbox.sock --runtime runsc
//
// The sidecar is the only process in the supported sandbox topology with the
// Docker socket. It exposes a two-route Unix-socket API: /health and /v1/run.
// There is deliberately no API for selecting an image, command, mount, or
// network: those are fixed at boot and every run is gVisor-gated.
func runSandboxd(args []string) int {
	fs := flag.NewFlagSet("sandboxd", flag.ContinueOnError)
	socket := fs.String("socket", "/run/kubona-sandbox/sandbox.sock", "private Unix socket path")
	image := fs.String("image", sandbox.DefaultRunnerImage, "fixed runner image")
	runtime := fs.String("runtime", "runsc", "container runtime (must be runsc/gVisor)")
	scratch := fs.String("scratch", "/tmp/kubona-sandboxd", "scratch root for isolated benchmark/workspace dirs")
	maxConcurrent := fs.Int("max-concurrent", 2, "maximum concurrent sandbox runs")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *runtime != "runsc" {
		fmt.Fprintln(os.Stderr, "sandboxd: refusing a non-gVisor runtime; only runsc is supported")
		return 1
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	srv := &sandbox.SidecarServer{
		Socket: *socket, Image: *image, Runtime: *runtime,
		ScratchRoot: *scratch, MaxConcurrent: *maxConcurrent,
	}
	fmt.Fprintf(os.Stderr, "sandboxd: serving %s (image=%s runtime=%s)\n", *socket, *image, *runtime)
	if err := sandbox.ServeSidecar(ctx, srv); err != nil {
		fmt.Fprintln(os.Stderr, "sandboxd:", err)
		return 1
	}
	return 0
}
