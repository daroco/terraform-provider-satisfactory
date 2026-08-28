package export

import (
	"regexp"
	"strings"
	"testing"

	"github.com/daroco/terraform-provider-satisfactory/internal/api"
)

// hasAttr matches an attribute regardless of alignment padding, so these tests
// assert what the configuration means, not which column it starts in.
func hasAttr(hcl, name, value string) bool {
	re := regexp.MustCompile(`(?m)^\s+` + regexp.QuoteMeta(name) + `\s+= ` + regexp.QuoteMeta(value) + `$`)
	return re.MatchString(hcl)
}

func sample() []api.WorldBuildable {
	return []api.WorldBuildable{
		{Index: 0, Class: "Build_Foundation_8x4_01_C", Transform: api.Transform{X: 1000, Y: 2000, Z: 20000}, Lightweight: true},
		{Index: 1, Class: "Build_Foundation_8x4_01_C", Transform: api.Transform{X: 1800, Y: 2000, Z: 20000}, Lightweight: true},
		{Index: 2, Class: "Build_SmelterMk1_C", Transform: api.Transform{X: 1200, Y: 2200, Z: 20200, Yaw: 90},
			Recipe: "Recipe_IngotIron_C", ClockSpeed: 1, TFID: "already-managed"},
		{Index: 3, Class: "Build_ConveyorBeltMk1_C", Transform: api.Transform{X: 1200, Y: 2600, Z: 20200}},
	}
}

// The point of exporting is a blueprint that can be applied somewhere else, so
// nothing may reference an absolute coordinate.
func TestGenerateEmitsPositionsRelativeToOrigin(t *testing.T) {
	res := Generate(sample(), Options{Origin: api.Vec3{X: 1000, Y: 2000, Z: 20000}})

	for _, absolute := range []string{"= 1800", "= 2200", "= 20200"} {
		if strings.Contains(res.HCL, absolute) {
			t.Errorf("output contains absolute coordinate %q; every position must be an offset from var.origin:\n%s", absolute, res.HCL)
		}
	}
	for _, want := range [][2]string{
		{"x", "var.origin.x"}, // exactly at the origin: no "+ 0" noise
		{"x", "var.origin.x + 800"},
		{"y", "var.origin.y + 200"},
		{"z", "var.origin.z + 200"},
	} {
		if !hasAttr(res.HCL, want[0], want[1]) {
			t.Errorf("missing %s = %s in:\n%s", want[0], want[1], res.HCL)
		}
	}
	// "+ -200" is correct and also looks generated; blueprints get read by hand.
	if strings.Contains(res.HCL, "+ -") {
		t.Errorf("negative offsets should be written as subtraction:\n%s", res.HCL)
	}
}

func TestGenerateMapsLightweightToFoundation(t *testing.T) {
	res := Generate(sample(), Options{})
	if got := strings.Count(res.HCL, `resource "satisfactory_foundation"`); got != 2 {
		t.Errorf("got %d foundations, want 2:\n%s", got, res.HCL)
	}
	if got := strings.Count(res.HCL, `resource "satisfactory_building"`); got != 1 {
		t.Errorf("got %d buildings, want 1:\n%s", got, res.HCL)
	}
}

// A belt is defined by the two connectors it joins. Emitting one from its
// position would produce configuration that applies cleanly and builds a
// factory that does not run - worse than leaving it out and saying so.
func TestGenerateSkipsConnectionsAndSaysSo(t *testing.T) {
	res := Generate(sample(), Options{})
	if strings.Contains(res.HCL, "Build_ConveyorBeltMk1_C\"\n") {
		t.Fatalf("belt emitted as a positional resource:\n%s", res.HCL)
	}
	if res.Skipped["Build_ConveyorBeltMk1_C"] != 1 {
		t.Errorf("Skipped = %v, want the belt counted", res.Skipped)
	}
	if !strings.Contains(res.HCL, "1 x Build_ConveyorBeltMk1_C") {
		t.Errorf("skipped classes should be listed in the output so the gap is visible:\n%s", res.HCL)
	}
}

func TestGenerateCarriesRecipeAndClockSpeed(t *testing.T) {
	items := sample()
	items[2].ClockSpeed = 1.5
	res := Generate(items, Options{})
	if !hasAttr(res.HCL, "recipe", `"Recipe_IngotIron_C"`) {
		t.Errorf("recipe missing:\n%s", res.HCL)
	}
	if !hasAttr(res.HCL, "clock_speed", "1.5") {
		t.Errorf("clock_speed missing:\n%s", res.HCL)
	}
	// 1.0 is the default; writing it out is noise in a shared blueprint.
	if res2 := Generate(sample(), Options{}); strings.Contains(res2.HCL, "clock_speed") {
		t.Errorf("default clock speed should be omitted:\n%s", res2.HCL)
	}
}

// Exports get committed and re-generated; a reshuffled file makes the diff
// useless for review.
func TestGenerateIsDeterministicRegardlessOfInputOrder(t *testing.T) {
	forward := Generate(sample(), Options{})
	items := sample()
	for i, j := 0, len(items)-1; i < j; i, j = i+1, j-1 {
		items[i], items[j] = items[j], items[i]
	}
	if reversed := Generate(items, Options{}); reversed.HCL != forward.HCL {
		t.Errorf("output depends on input order:\n--- forward ---\n%s\n--- reversed ---\n%s", forward.HCL, reversed.HCL)
	}
}

func TestResourceNamesAreUniqueAndValidIdentifiers(t *testing.T) {
	res := Generate(sample(), Options{})
	if !strings.Contains(res.HCL, `"foundation_8x4_01_0"`) || !strings.Contains(res.HCL, `"foundation_8x4_01_1"`) {
		t.Errorf("repeated classes must get distinct labels:\n%s", res.HCL)
	}
	if strings.Contains(res.HCL, `"8x4`) {
		t.Errorf("a label may not start with a digit:\n%s", res.HCL)
	}
}

func TestResourceBaseNameLeadingDigit(t *testing.T) {
	if got := resourceBaseName("Build_4x4_C"); got != "b_4x4" {
		t.Errorf("resourceBaseName = %q, want b_4x4 (identifiers cannot start with a digit)", got)
	}
}

// Generated configuration has to declare the provider it needs, or terraform
// looks for hashicorp/satisfactory and fails on a registry lookup.
func TestGenerateDeclaresTheProvider(t *testing.T) {
	res := Generate(sample(), Options{})
	if !strings.Contains(res.HCL, `source = "daroco/satisfactory"`) {
		t.Errorf("no required_providers block:\n%s", res.HCL)
	}
}

// connected returns a smelter, a constructor, and the belt joining them.
func connected() []api.WorldBuildable {
	return []api.WorldBuildable{
		{Index: 0, Class: "Build_SmelterMk1_C", Transform: api.Transform{X: 0, Y: 0, Z: 0}, Recipe: "Recipe_IngotIron_C"},
		{Index: 1, Class: "Build_ConstructorMk1_C", Transform: api.Transform{X: 0, Y: 800, Z: 0}, Recipe: "Recipe_IronPlate_C"},
		{Index: 2, Class: "Build_ConveyorBeltMk1_C", Transform: api.Transform{X: 0, Y: 400, Z: 0},
			Connects: &api.WorldConnection{
				From: api.WorldEndpoint{Index: 0, Connector: 1},
				To:   api.WorldEndpoint{Index: 1, Connector: 0},
			}},
	}
}

func TestGenerateEmitsBeltsReferencingTheirEndpoints(t *testing.T) {
	res := Generate(connected(), Options{})
	if !strings.Contains(res.HCL, `resource "satisfactory_belt"`) {
		t.Fatalf("no belt emitted:\n%s", res.HCL)
	}
	if !hasAttr(res.HCL, "from_id", "satisfactory_building.smeltermk1_0.id") {
		t.Errorf("belt should reference the smelter resource, not an id literal:\n%s", res.HCL)
	}
	if !hasAttr(res.HCL, "to_id", "satisfactory_building.constructormk1_0.id") {
		t.Errorf("belt should reference the constructor resource:\n%s", res.HCL)
	}
	if !hasAttr(res.HCL, "from_connector", "1") || !hasAttr(res.HCL, "to_connector", "0") {
		t.Errorf("connector indices lost:\n%s", res.HCL)
	}
	if len(res.Skipped) != 0 {
		t.Errorf("Skipped = %v, want nothing skipped", res.Skipped)
	}
}

// The mod omits `connects` when one end falls outside the exported radius.
// Guessing an endpoint would wire the belt to whatever sits at that index.
func TestGenerateSkipsConnectionsWithUnresolvableEnds(t *testing.T) {
	items := connected()
	items[2].Connects.To.Index = 99 // never enumerated
	res := Generate(items, Options{})
	if strings.Contains(res.HCL, `resource "satisfactory_belt"`) {
		t.Errorf("a belt with an unknown end must not be emitted:\n%s", res.HCL)
	}
	if res.Skipped["Build_ConveyorBeltMk1_C"] != 1 {
		t.Errorf("Skipped = %v, want the belt counted", res.Skipped)
	}
}

func TestGenerateSkipsConnectionTypesWithNoResource(t *testing.T) {
	items := connected()
	items[2].Class = "Build_Pipeline_C" // no provider resource for pipes yet
	res := Generate(items, Options{})
	if strings.Contains(res.HCL, "Build_Pipeline_C\"\n") {
		t.Errorf("pipelines have no resource type and must not be emitted:\n%s", res.HCL)
	}
	if res.Skipped["Build_Pipeline_C"] != 1 {
		t.Errorf("Skipped = %v", res.Skipped)
	}
}

// Terraform resolves ordering from references, but a file where a belt appears
// before the machines it names is confusing to read and to edit.
func TestGenerateWritesConnectionsAfterBuildables(t *testing.T) {
	res := Generate(connected(), Options{})
	belt := strings.Index(res.HCL, `resource "satisfactory_belt"`)
	last := strings.LastIndex(res.HCL, `resource "satisfactory_building"`)
	if belt < last {
		t.Errorf("belt emitted before the buildables it references:\n%s", res.HCL)
	}
}

func TestGeneratePowerLinesUseTheirOwnResource(t *testing.T) {
	items := connected()
	items[2].Class = "Build_PowerLine_C"
	res := Generate(items, Options{})
	if !strings.Contains(res.HCL, `resource "satisfactory_power_line"`) {
		t.Errorf("power lines need satisfactory_power_line, not satisfactory_belt:\n%s", res.HCL)
	}
}
