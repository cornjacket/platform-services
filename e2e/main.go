package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/cornjacket/platform-services/e2e/runner"
	_ "github.com/cornjacket/platform-services/e2e/tests" // Register all tests
)

func main() {
	env := flag.String("env", "local", "Environment (local, dev, staging)")
	testName := flag.String("test", "", "Specific test to run (runs all if empty)")
	list := flag.Bool("list", false, "List available tests")
	flag.Parse()

	// List tests and exit
	if *list {
		runner.ListTests()
		os.Exit(0)
	}

	// Load configuration
	cfg := runner.LoadConfig(*env)

	fmt.Printf("E2E Test Runner\n")
	fmt.Printf("Environment: %s\n", cfg.Env)
	fmt.Printf("Ingestion:   %s\n", cfg.IngestionURL)
	fmt.Printf("Query:       %s\n", cfg.QueryURL)
	fmt.Println("─────────────────────────────────────────")

	// Create context with signal handling
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		fmt.Println("\nInterrupted, stopping tests...")
		cancel()
	}()

	var exitCode int

	if *testName != "" {
		// Run single test
		result, err := runner.RunSingle(ctx, *testName, cfg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		if !result.Passed {
			exitCode = 1
		}
	} else {
		// Run all tests
		results := runner.RunAll(ctx, cfg)
		runner.PrintSummary(results)

		for _, r := range results {
			if !r.Passed {
				exitCode = 1
				break
			}
		}
	}

	os.Exit(exitCode)
}
