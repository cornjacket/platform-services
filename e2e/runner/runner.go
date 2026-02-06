package runner

import (
	"context"
	"fmt"
	"os"
	"sort"
	"time"
)

// Test represents a single e2e test.
type Test struct {
	Name        string
	Description string
	Run         func(ctx context.Context, cfg *Config) error
}

// Config holds test runner configuration.
type Config struct {
	IngestionURL string
	QueryURL     string
	Env          string
	Timeout      time.Duration
}

// Result represents the outcome of a test run.
type Result struct {
	Test     *Test
	Passed   bool
	Duration time.Duration
	Error    error
}

var registry = make(map[string]*Test)

// Register adds a test to the registry (called from test init()).
func Register(t *Test) {
	if _, exists := registry[t.Name]; exists {
		panic(fmt.Sprintf("test %q already registered", t.Name))
	}
	registry[t.Name] = t
}

// GetTest returns a test by name.
func GetTest(name string) (*Test, bool) {
	t, ok := registry[name]
	return t, ok
}

// GetAllTests returns all registered tests sorted by name.
func GetAllTests() []*Test {
	tests := make([]*Test, 0, len(registry))
	for _, t := range registry {
		tests = append(tests, t)
	}
	sort.Slice(tests, func(i, j int) bool {
		return tests[i].Name < tests[j].Name
	})
	return tests
}

// ListTests prints all available tests.
func ListTests() {
	tests := GetAllTests()
	fmt.Println("Available tests:")
	for _, t := range tests {
		fmt.Printf("  %-25s %s\n", t.Name, t.Description)
	}
}

// RunTest executes a single test and returns the result.
func RunTest(ctx context.Context, t *Test, cfg *Config) *Result {
	start := time.Now()

	// Create context with timeout
	testCtx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()

	err := t.Run(testCtx, cfg)
	duration := time.Since(start)

	return &Result{
		Test:     t,
		Passed:   err == nil,
		Duration: duration,
		Error:    err,
	}
}

// RunAll executes all registered tests and returns results.
func RunAll(ctx context.Context, cfg *Config) []*Result {
	tests := GetAllTests()
	results := make([]*Result, 0, len(tests))

	for _, t := range tests {
		result := RunTest(ctx, t, cfg)
		results = append(results, result)
		printResult(result)
	}

	return results
}

// RunSingle executes a single test by name.
func RunSingle(ctx context.Context, name string, cfg *Config) (*Result, error) {
	t, ok := GetTest(name)
	if !ok {
		return nil, fmt.Errorf("unknown test: %s", name)
	}

	result := RunTest(ctx, t, cfg)
	printResult(result)
	return result, nil
}

func printResult(r *Result) {
	status := "✓ PASS"
	if !r.Passed {
		status = "✗ FAIL"
	}

	fmt.Printf("%s  %-25s  (%v)\n", status, r.Test.Name, r.Duration.Round(time.Millisecond))

	if r.Error != nil {
		fmt.Fprintf(os.Stderr, "       Error: %v\n", r.Error)
	}
}

// PrintSummary prints a summary of test results.
func PrintSummary(results []*Result) {
	passed := 0
	failed := 0
	var totalDuration time.Duration

	for _, r := range results {
		totalDuration += r.Duration
		if r.Passed {
			passed++
		} else {
			failed++
		}
	}

	fmt.Println()
	fmt.Println("─────────────────────────────────────────")
	fmt.Printf("Total: %d  Passed: %d  Failed: %d  Duration: %v\n",
		len(results), passed, failed, totalDuration.Round(time.Millisecond))

	if failed > 0 {
		fmt.Println("\nFailed tests:")
		for _, r := range results {
			if !r.Passed {
				fmt.Printf("  - %s: %v\n", r.Test.Name, r.Error)
			}
		}
	}
}

// LoadConfig creates a Config from environment variables.
func LoadConfig(env string) *Config {
	cfg := &Config{
		Env:     env,
		Timeout: 30 * time.Second,
	}

	// Check for environment variable overrides first
	if url := os.Getenv("E2E_INGESTION_URL"); url != "" {
		cfg.IngestionURL = url
	}
	if url := os.Getenv("E2E_QUERY_URL"); url != "" {
		cfg.QueryURL = url
	}

	// Apply defaults based on environment if not set
	if cfg.IngestionURL == "" || cfg.QueryURL == "" {
		switch env {
		case "local":
			if cfg.IngestionURL == "" {
				cfg.IngestionURL = "http://localhost:8080"
			}
			if cfg.QueryURL == "" {
				cfg.QueryURL = "http://localhost:8081"
			}
		case "dev":
			if cfg.IngestionURL == "" {
				cfg.IngestionURL = "https://api-dev.cornjacket.com"
			}
			if cfg.QueryURL == "" {
				cfg.QueryURL = "https://query-dev.cornjacket.com"
			}
		case "staging":
			if cfg.IngestionURL == "" {
				cfg.IngestionURL = "https://api-staging.cornjacket.com"
			}
			if cfg.QueryURL == "" {
				cfg.QueryURL = "https://query-staging.cornjacket.com"
			}
		}
	}

	return cfg
}
