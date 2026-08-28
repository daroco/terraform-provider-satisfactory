// Package export turns a region of a live world into Terraform configuration.
//
// The output is deliberately *relative*: every position is emitted as an
// offset from a var.origin, so an exported factory is a blueprint that can be
// applied anywhere rather than a recording pinned to the coordinates it was
// built at. That is the whole point of exporting - a config that only works in
// the exact spot it came from is just a save file with extra steps.
package export

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/daroco/terraform-provider-satisfactory/internal/api"
)

// Options controls generation.
type Options struct {
	// Origin is subtracted from every position. Callers usually pass the
	// centre of the region they queried, or a player's feet.
	Origin api.Vec3
	// ModuleName is a comment header only; it does not affect resources.
	ModuleName string
}

// Result is generated configuration plus what could not be represented.
type Result struct {
	HCL string
	// Emitted counts resources actually written.
	Emitted int
	// Skipped counts classes left out, keyed by class name. Belts and power
	// lines land here: they are defined by which connectors they join, and
	// that graph is not part of world enumeration yet.
	Skipped map[string]int
}

// connectionClassPrefixes are buildables whose meaning is their endpoints, not
// their position. Emitting one from a position alone would produce a config
// that applies cleanly and builds a factory that does not run.
var connectionClassPrefixes = []string{
	"Build_ConveyorBelt", "Build_ConveyorLift", "Build_PowerLine",
	"Build_Pipeline", "Build_PipelineMK2", "Build_PipeHyper",
}

func isConnectionClass(class string) bool {
	for _, p := range connectionClassPrefixes {
		if strings.HasPrefix(class, p) {
			return true
		}
	}
	return false
}

// connectionResourceType maps a belt or wire class to the resource that can
// rebuild it, or "" for connections the provider has no resource for yet
// (pipelines and hypertubes). Exporting one of those as a positional
// resource would produce configuration that applies and does nothing.
func connectionResourceType(class string) string {
	switch {
	case strings.HasPrefix(class, "Build_ConveyorBelt"), strings.HasPrefix(class, "Build_ConveyorLift"):
		return "satisfactory_belt"
	case strings.HasPrefix(class, "Build_PowerLine"):
		return "satisfactory_power_line"
	default:
		return ""
	}
}

var nonIdent = regexp.MustCompile(`[^a-zA-Z0-9_]+`)

// resourceBaseName turns Build_Foundation_8x4_01_C into foundation_8x4_01.
func resourceBaseName(class string) string {
	n := strings.TrimPrefix(class, "Build_")
	n = strings.TrimSuffix(n, "_C")
	n = nonIdent.ReplaceAllString(n, "_")
	n = strings.Trim(strings.ToLower(n), "_")
	if n == "" {
		return "buildable"
	}
	// Terraform identifiers may not start with a digit.
	if n[0] >= '0' && n[0] <= '9' {
		n = "b_" + n
	}
	return n
}

// num formats a float for HCL without scientific notation or a trailing ".0",
// so coordinates read the way a person would write them.
func num(f float64) string {
	return strconv.FormatFloat(f, 'f', -1, 64)
}

// offset renders a coordinate relative to var.origin. Writing "+ -200" would
// be correct and would also look generated; blueprints get read and edited by
// hand, so they should read as if a person wrote them.
func offset(axis string, d float64) string {
	base := "var.origin." + axis
	switch {
	case d == 0:
		return base
	case d < 0:
		return base + " - " + num(-d)
	default:
		return base + " + " + num(d)
	}
}

// Generate renders items as Terraform configuration.
func Generate(items []api.WorldBuildable, opts Options) Result {
	res := Result{Skipped: map[string]int{}}

	// Stable output: same world, same file, so regenerating an export produces
	// a reviewable diff rather than a reshuffle.
	sorted := append([]api.WorldBuildable(nil), items...)
	sort.SliceStable(sorted, func(i, j int) bool {
		a, b := sorted[i], sorted[j]
		if a.Class != b.Class {
			return a.Class < b.Class
		}
		if a.Transform.X != b.Transform.X {
			return a.Transform.X < b.Transform.X
		}
		if a.Transform.Y != b.Transform.Y {
			return a.Transform.Y < b.Transform.Y
		}
		return a.Transform.Z < b.Transform.Z
	})

	var b strings.Builder
	name := opts.ModuleName
	if name == "" {
		name = "exported factory"
	}
	fmt.Fprintf(&b, "# %s\n", name)
	b.WriteString("#\n")
	b.WriteString("# Generated from a live world. Positions are relative to var.origin, so\n")
	b.WriteString("# this can be applied anywhere - move the origin, move the factory.\n\n")
	// A shared blueprint has to declare what it needs; without this Terraform
	// looks for hashicorp/satisfactory and fails on a registry lookup.
	b.WriteString("terraform {\n")
	b.WriteString("  required_providers {\n")
	b.WriteString("    satisfactory = {\n")
	b.WriteString("      source = \"daroco/satisfactory\"\n")
	b.WriteString("    }\n")
	b.WriteString("  }\n")
	b.WriteString("}\n\n")
	b.WriteString("variable \"origin\" {\n")
	b.WriteString("  description = \"World position this layout is built around, in centimetres.\"\n")
	b.WriteString("  type        = object({ x = number, y = number, z = number })\n")
	fmt.Fprintf(&b, "  default     = { x = %s, y = %s, z = %s }\n",
		num(opts.Origin.X), num(opts.Origin.Y), num(opts.Origin.Z))
	b.WriteString("}\n")

	// Labels are assigned before anything is written, because a belt refers to
	// the two buildables it joins and either of them may be emitted after it.
	// WorldBuildable.Index is the response-local handle those references use.
	type placed struct {
		kind  string
		label string
	}
	byIndex := map[int64]placed{}
	used := map[string]int{}
	for _, it := range sorted {
		if isConnectionClass(it.Class) {
			continue
		}
		kind := "satisfactory_building"
		if it.Lightweight {
			kind = "satisfactory_foundation"
		}
		base := resourceBaseName(it.Class)
		label := fmt.Sprintf("%s_%d", base, used[base])
		used[base]++
		byIndex[it.Index] = placed{kind, label}
	}

	emitted := 0
	writeBlock := func(kind, label string, attrs [][2]string) {
		fmt.Fprintf(&b, "\nresource %q %q {\n", kind, label)
		width := 0
		for _, a := range attrs {
			if len(a[0]) > width {
				width = len(a[0])
			}
		}
		for _, a := range attrs {
			fmt.Fprintf(&b, "  %-*s = %s\n", width, a[0], a[1])
		}
		b.WriteString("}\n")
		emitted++
	}

	for _, it := range sorted {
		if isConnectionClass(it.Class) {
			continue
		}
		p := byIndex[it.Index]

		dx := it.Transform.X - opts.Origin.X
		dy := it.Transform.Y - opts.Origin.Y
		dz := it.Transform.Z - opts.Origin.Z

		attrs := [][2]string{
			{"class", fmt.Sprintf("%q", it.Class)},
			{"x", offset("x", dx)},
			{"y", offset("y", dy)},
			{"z", offset("z", dz)},
		}
		if it.Transform.Yaw != 0 {
			attrs = append(attrs, [2]string{"yaw", num(it.Transform.Yaw)})
		}
		if it.Recipe != "" {
			attrs = append(attrs, [2]string{"recipe", fmt.Sprintf("%q", it.Recipe)})
		}
		if it.ClockSpeed != 0 && it.ClockSpeed != 1 {
			attrs = append(attrs, [2]string{"clock_speed", num(it.ClockSpeed)})
		}
		writeBlock(p.kind, p.label, attrs)
	}

	// Connections last: they reference the resources above, and Terraform
	// works out the ordering from those references rather than from the file.
	for _, it := range sorted {
		if !isConnectionClass(it.Class) {
			continue
		}
		kind := connectionResourceType(it.Class)
		var from, to placed
		okFrom, okTo := false, false
		if it.Connects != nil {
			from, okFrom = byIndex[it.Connects.From.Index]
			to, okTo = byIndex[it.Connects.To.Index]
		}
		if kind == "" || !okFrom || !okTo {
			// Either the mod could not resolve both ends (one lay outside the
			// exported radius), or this is something with no resource type
			// yet, like a pipeline. Naming it in a comment beats emitting a
			// connection to nowhere.
			res.Skipped[it.Class]++
			continue
		}
		base := resourceBaseName(it.Class)
		label := fmt.Sprintf("%s_%d", base, used[base])
		used[base]++
		writeBlock(kind, label, [][2]string{
			{"class", fmt.Sprintf("%q", it.Class)},
			{"from_id", fmt.Sprintf("%s.%s.id", from.kind, from.label)},
			{"from_connector", strconv.FormatInt(it.Connects.From.Connector, 10)},
			{"to_id", fmt.Sprintf("%s.%s.id", to.kind, to.label)},
			{"to_connector", strconv.FormatInt(it.Connects.To.Connector, 10)},
		})
	}

	if len(res.Skipped) > 0 {
		b.WriteString("\n# Not exported. A belt or wire is defined by the two connectors it\n")
		b.WriteString("# joins, so one is only exportable when both ends are inside the\n")
		b.WriteString("# region and the game reported both connector indices:\n")
		classes := make([]string, 0, len(res.Skipped))
		for c := range res.Skipped {
			classes = append(classes, c)
		}
		sort.Strings(classes)
		for _, c := range classes {
			fmt.Fprintf(&b, "#   %d x %s\n", res.Skipped[c], c)
		}
	}

	res.HCL = b.String()
	res.Emitted = emitted
	return res
}
