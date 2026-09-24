package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/PeteMadCoder/vpok/internal/daemon"
	"github.com/PeteMadCoder/vpok/internal/store"
)

const version = "0.1.0"

func defaultSocketPath() string {
	if custom := os.Getenv("VPOK_SOCKET"); custom != "" {
		return custom
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "/tmp/vpokd.sock"
	}
	return filepath.Join(home, ".vpok", ".vpokd.sock")
}

func defaultStorePath() string {
	if custom := os.Getenv("VPOK_STORE"); custom != "" {
		return custom
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".", ".vpok", "store")
	}
	return filepath.Join(home, ".vpok", "store")
}

func main() {
	socketFlag := flag.String("socket", defaultSocketPath(), "Path to Unix domain socket")
	storeFlag := flag.String("store", defaultStorePath(), "Path to CAS store")
	flag.Parse()

	casStore, err := store.NewCASStore(*storeFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to initialize store at %s: %v\n", *storeFlag, err)
		os.Exit(1)
	}

	srv := daemon.NewServer(casStore, *socketFlag, version)

	// Set up graceful shutdown channel
	stopChan := make(chan os.Signal, 1)
	signal.Notify(stopChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		fmt.Printf("vpokd %s starting on socket %s\n", version, *socketFlag)
		fmt.Printf("Using store directory: %s\n", *storeFlag)
		if err := srv.Start(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			fmt.Fprintf(os.Stderr, "daemon server error: %v\n", err)
			os.Exit(1)
		}
	}()

	<-stopChan
	fmt.Println("\nShutting down vpokd gracefully...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		fmt.Fprintf(os.Stderr, "error during shutdown: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("vpok stopped.")
}
