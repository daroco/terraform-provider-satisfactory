// Command satisfactory-coverage asks a running game which buildable classes
// exist and reports how many of them Terraform can place - and, for the rest,
// which mechanism each one is waiting on.
//
// "Everything placeable" is the goal. This is how it is measured.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/daroco/terraform-provider-satisfactory/internal/client"
	"github.com/daroco/terraform-provider-satisfactory/internal/coverage"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run() error {
	var (
		endpoint = flag.String("endpoint", envOr("SATISFACTORY_ENDPOINT", "http://localhost:8090"), "mod API endpoint")
		token    = flag.String("token", os.Getenv("SATISFACTORY_TOKEN"), "bearer token, if the mod requires one")
		all      = flag.Bool("all", false, "list every class under the supported groups too, not just the gaps")
		asJSON   = flag.Bool("json", false, "emit the raw catalog as JSON instead of a report")
	)
	flag.Parse()

	// The first call loads every buildable class the game has, which takes
	// the mod a few seconds; later calls are served from its cache.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	classes, err := client.New(*endpoint, *token).ListClasses(ctx)
	if err != nil {
		return fmt.Errorf("listing classes: %w", err)
	}
	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(classes)
	}
	fmt.Print(coverage.Report(coverage.Summarize(classes), *all))
	return nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
