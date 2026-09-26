package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/PeteMadCoder/vpok/internal/agent"
)

func main() {
	configPath := flag.String("config", "/run/vpok/config.json", "Path to agent config file")
	secretsDir := flag.String("secrets-dir", agent.DefaultSecretsDir, "Directory to write injected secrets")
	flag.Parse()

	if *configPath == "" {
		log.Fatalf("config file path is required")
	}

	cfg, err := agent.LoadConfig(*configPath)
	if err != nil {
		log.Fatalf("failed to load configuration: %v", err)
	}

	// 1. Provision secrets to disk
	if len(cfg.Secrets) > 0 {
		if err := agent.SetupSecrets(*secretsDir, cfg.Secrets); err != nil {
			log.Fatalf("failed to setup secrets: %v", err)
		}
	}

	// 2. Setup context and signal listener
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGTERM, syscall.SIGINT)

	// 3. Initialize supervisor and launch entrypoint
	supervisor := agent.NewSupervisor(cfg, os.Stdout, os.Stderr)
	resultChan, err := supervisor.Start(ctx)
	if err != nil {
		log.Fatalf("failed to start entrypoint: %v", err)
	}

	// 4. Start health checker if configured
	if cfg.HealthCheck != nil {
		checker := agent.NewHealthChecker(cfg.HealthCheck, func(healthy bool, err error) {
			if healthy {
				log.Println("health check: healthy")
			} else {
				log.Printf("health check: unhealthy: %v", err)
			}
		})
		go checker.Start(ctx)
	}

	// 5. Wait for workload termination or incoming termination signal
	select {
	case sig := <-sigChan:
		log.Printf("received signal %s, initiating graceful shutdown...", sig)
		cancel()
		if err := supervisor.Stop(); err != nil {
			log.Printf("error stopping supervisor: %v", err)
		}
		// Wait for the process to exit after stop signal
		res := <-resultChan
		os.Exit(res.ExitCode)

	case res := <-resultChan:
		if res.Err != nil {
			log.Printf("process exit with error: %v (exit code %d)", res.Err, res.ExitCode)
		}
		os.Exit(res.ExitCode)
	}

}
