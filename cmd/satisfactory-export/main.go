// Command satisfactory-export reads a region of a running game and writes it
// out as Terraform configuration - build a factory by hand, export it, share
// the file, apply it somewhere else.
//
// It is a separate binary rather than a provider feature because it is not a
// Terraform operation: there is no state, no plan, and nothing to apply. It
// only reads.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/daroco/terraform-provider-satisfactory/internal/api"
	"github.com/daroco/terraform-provider-satisfactory/internal/client"
	"github.com/daroco/terraform-provider-satisfactory/internal/export"
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
		x        = flag.Float64("x", 0, "centre of the exported region, in centimetres")
		y        = flag.Float64("y", 0, "")
		z        = flag.Float64("z", 0, "")
		radius   = flag.Float64("radius", 5000, "how far around the centre to export, in centimetres")
		atPlayer = flag.Bool("at-player", false, "centre on the first connected player instead of -x/-y/-z")
		out      = flag.String("out", "-", "file to write, or - for stdout")
		name     = flag.String("name", "", "name for the generated config's header comment")
	)
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	c := client.New(*endpoint, *token)
	center := api.Vec3{X: *x, Y: *y, Z: *z}
	if *atPlayer {
		players, err := c.ListPlayers(ctx)
		if err != nil {
			return fmt.Errorf("listing players: %w", err)
		}
		if len(players) == 0 {
			return fmt.Errorf("-at-player: nobody is in the world (a dedicated server with no one connected has no anchor to offer)")
		}
		center = players[0].Location
		fmt.Fprintf(os.Stderr, "centring on %s at (%.0f, %.0f, %.0f)\n",
			displayName(players[0]), center.X, center.Y, center.Z)
	}

	items, err := c.ListWorldBuildables(ctx, center.X, center.Y, center.Z, *radius)
	if err != nil {
		return fmt.Errorf("listing world buildables: %w", err)
	}
	if len(items) == 0 {
		return fmt.Errorf("nothing within %.0f cm of (%.0f, %.0f, %.0f)", *radius, center.X, center.Y, center.Z)
	}

	res := export.Generate(items, export.Options{Origin: center, ModuleName: *name})

	if *out == "-" {
		fmt.Print(res.HCL)
	} else if err := os.WriteFile(*out, []byte(res.HCL), 0o644); err != nil {
		return err
	}

	tracked := 0
	for _, it := range items {
		if it.TFID != "" {
			tracked++
		}
	}
	skipped := 0
	for _, n := range res.Skipped {
		skipped += n
	}
	fmt.Fprintf(os.Stderr, "exported %d buildables (%d already managed by Terraform)",
		res.Emitted, tracked)
	if skipped > 0 {
		fmt.Fprintf(os.Stderr, "; skipped %d belts/wires - see the comment at the end of the file", skipped)
	}
	fmt.Fprintln(os.Stderr)
	return nil
}

func displayName(p api.Player) string {
	if p.Name == "" {
		return "player"
	}
	return p.Name
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
