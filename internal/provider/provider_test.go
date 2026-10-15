package provider_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/daroco/terraform-provider-satisfactory/internal/api"
	"github.com/daroco/terraform-provider-satisfactory/internal/client"
	"github.com/daroco/terraform-provider-satisfactory/internal/mockserver"
	"github.com/daroco/terraform-provider-satisfactory/internal/provider"
)

// captureID stores the resource's current id in *dst for later steps.
func captureID(name string, dst *string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[name]
		if !ok {
			return fmt.Errorf("resource %s not in state", name)
		}
		*dst = rs.Primary.ID
		return nil
	}
}

// checkSameID asserts the resource kept the id captured earlier (in-place update).
func checkSameID(name string, want *string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[name]
		if !ok {
			return fmt.Errorf("resource %s not in state", name)
		}
		if rs.Primary.ID != *want {
			return fmt.Errorf("id changed from %s to %s: update replaced instead of patching", *want, rs.Primary.ID)
		}
		return nil
	}
}

// checkDifferentID asserts the resource's current id differs from the one
// previously captured in *avoid (a RequiresReplace attribute forced a
// recreate rather than an in-place update), then updates *avoid to the new
// id so a later step in the same test can chain another such assertion.
func checkDifferentID(name string, avoid *string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[name]
		if !ok {
			return fmt.Errorf("resource %s not in state", name)
		}
		if rs.Primary.ID == *avoid {
			return fmt.Errorf("id stayed %s: change should have forced replacement", *avoid)
		}
		*avoid = rs.Primary.ID
		return nil
	}
}

// startMock spins up a fresh in-memory world and points the provider at it via
// the endpoint env var so test configs need no provider block arguments.
func startMock(t *testing.T) *client.Client {
	t.Helper()
	srv := httptest.NewServer(mockserver.New("").Handler())
	t.Cleanup(srv.Close)
	t.Setenv("SATISFACTORY_ENDPOINT", srv.URL)
	return client.New(srv.URL, "")
}

var protoFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"satisfactory": providerserver.NewProtocol6WithError(provider.New("test")()),
}

func TestAccBuilding(t *testing.T) {
	c := startMock(t)

	config := `
resource "satisfactory_building" "smelter" {
  class  = "Build_SmelterMk1_C"
  x      = 0
  y      = 0
  z      = 100
  recipe = "Recipe_IngotIron_C"
}
`
	updated := `
resource "satisfactory_building" "smelter" {
  class       = "Build_SmelterMk1_C"
  x           = 0
  y           = 0
  z           = 100
  recipe      = "Recipe_IngotIron_C"
  clock_speed = 1.5
}
`
	var firstID string
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: protoFactories,
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("satisfactory_building.smelter", "clock_speed", "1"),
					resource.TestCheckResourceAttrSet("satisfactory_building.smelter", "id"),
					captureID("satisfactory_building.smelter", &firstID),
				),
			},
			{
				// clock_speed changes in place: same id afterwards.
				Config: updated,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("satisfactory_building.smelter", "clock_speed", "1.5"),
					checkSameID("satisfactory_building.smelter", &firstID),
				),
			},
			{
				// Simulate the pioneer dismantling it in-game: refresh must
				// detect the loss and plan a recreate.
				PreConfig: func() {
					if err := c.DeleteBuildable(context.Background(), firstID); err != nil {
						t.Fatalf("out-of-band delete: %v", err)
					}
				},
				Config:             updated,
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
		},
	})
}

func TestAccBuildingInvalidClass(t *testing.T) {
	startMock(t)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: protoFactories,
		Steps: []resource.TestStep{
			{
				Config: `
resource "satisfactory_building" "bad" {
  class = "NotABuildable"
  x     = 0
  y     = 0
  z     = 0
}
`,
				ExpectError: regexp.MustCompile(`class must be a buildable class name`),
			},
		},
	})
}

func TestAccFullLine(t *testing.T) {
	startMock(t)

	config := `
resource "satisfactory_foundation" "pad" {
  class = "Build_Foundation_8x4_01_C"
  x     = 0
  y     = 0
  z     = 0
}

resource "satisfactory_building" "smelter" {
  class  = "Build_SmelterMk1_C"
  x      = 0
  y      = 0
  z      = 100
  recipe = "Recipe_IngotIron_C"
}

resource "satisfactory_building" "constructor" {
  class  = "Build_ConstructorMk1_C"
  x      = 1000
  y      = 0
  z      = 100
  recipe = "Recipe_IronPlate_C"
}

resource "satisfactory_belt" "smelter_to_constructor" {
  class          = "Build_ConveyorBeltMk1_C"
  from_id        = satisfactory_building.smelter.id
  from_connector = 1
  to_id          = satisfactory_building.constructor.id
  to_connector   = 0
}

resource "satisfactory_power_line" "power" {
  from_id        = satisfactory_building.smelter.id
  from_connector = 0
  to_id          = satisfactory_building.constructor.id
  to_connector   = 0
}

resource "satisfactory_pipeline" "fluid" {
  class          = "Build_Pipeline_C"
  from_id        = satisfactory_building.smelter.id
  from_connector = 0
  to_id          = satisfactory_building.constructor.id
  to_connector   = 0
}

resource "satisfactory_hypertube" "tube" {
  from_id        = satisfactory_building.smelter.id
  from_connector = 0
  to_id          = satisfactory_building.constructor.id
  to_connector   = 0
}
`
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: protoFactories,
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttrSet("satisfactory_belt.smelter_to_constructor", "id"),
					resource.TestCheckResourceAttr("satisfactory_power_line.power", "class", "Build_PowerLine_C"),
					resource.TestCheckResourceAttrPair(
						"satisfactory_belt.smelter_to_constructor", "from_id",
						"satisfactory_building.smelter", "id",
					),
					// Pipelines take an explicit class; hypertubes default
					// theirs, like power lines.
					resource.TestCheckResourceAttrSet("satisfactory_pipeline.fluid", "id"),
					resource.TestCheckResourceAttr("satisfactory_hypertube.tube", "class", "Build_PipeHyper_C"),
				),
			},
			{
				// Re-applying the same config must be a no-op.
				Config:   config,
				PlanOnly: true,
			},
		},
	})
}

func TestAccBuildingImport(t *testing.T) {
	c := startMock(t)
	if _, err := c.CreateBuildable(context.Background(), api.Buildable{
		TFID:       "imported-1",
		Class:      "Build_SmelterMk1_C",
		Transform:  api.Transform{X: 5, Y: 6, Z: 7},
		Recipe:     "Recipe_IngotIron_C",
		ClockSpeed: 1.0,
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: protoFactories,
		Steps: []resource.TestStep{
			{
				Config: `
resource "satisfactory_building" "smelter" {
  class  = "Build_SmelterMk1_C"
  x      = 5
  y      = 6
  z      = 7
  recipe = "Recipe_IngotIron_C"
}
`,
				ResourceName:  "satisfactory_building.smelter",
				ImportState:   true,
				ImportStateId: "imported-1",
				ImportStateCheck: func(states []*terraform.InstanceState) error {
					if len(states) != 1 {
						return fmt.Errorf("expected 1 state, got %d", len(states))
					}
					if got := states[0].Attributes["class"]; got != "Build_SmelterMk1_C" {
						return fmt.Errorf("imported class = %q", got)
					}
					return nil
				},
			},
		},
	})
}

// TestAccBuildingReplaceOnCoordinateAndClassChange verifies the
// RequiresReplace plan modifiers on x/class actually force a recreate
// end to end (schema-level coverage of every RequiresReplace attribute
// lives in resource_internal_test.go; this confirms the behavior survives
// a real plan/apply cycle through terraform-plugin-testing).
func TestAccBuildingReplaceOnCoordinateAndClassChange(t *testing.T) {
	startMock(t)

	base := `
resource "satisfactory_building" "smelter" {
  class = "Build_SmelterMk1_C"
  x     = 0
  y     = 0
  z     = 100
}
`
	movedX := `
resource "satisfactory_building" "smelter" {
  class = "Build_SmelterMk1_C"
  x     = 500
  y     = 0
  z     = 100
}
`
	newClass := `
resource "satisfactory_building" "smelter" {
  class = "Build_SmelterMk2_C"
  x     = 500
  y     = 0
  z     = 100
}
`
	var id string
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: protoFactories,
		Steps: []resource.TestStep{
			{
				Config: base,
				Check:  captureID("satisfactory_building.smelter", &id),
			},
			{
				// x changed: RequiresReplace, expect a new id.
				Config: movedX,
				Check:  checkDifferentID("satisfactory_building.smelter", &id),
			},
			{
				// class changed: RequiresReplace again.
				Config: newClass,
				Check:  checkDifferentID("satisfactory_building.smelter", &id),
			},
		},
	})
}

// TestAccFoundationLifecycle covers satisfactory_foundation on its own -
// previously it only appeared bundled into TestAccFullLine. It checks a
// clean create, a stable no-op re-apply, and in-game-dismantle drift
// detection (404 on Read -> state removal -> non-empty recreate plan, per
// CLAUDE.md's error contract).
func TestAccFoundationLifecycle(t *testing.T) {
	c := startMock(t)

	config := `
resource "satisfactory_foundation" "pad" {
  class = "Build_Foundation_8x4_01_C"
  x     = 0
  y     = 0
  z     = 0
}
`
	var id string
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: protoFactories,
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("satisfactory_foundation.pad", "yaw", "0"),
					resource.TestCheckResourceAttrSet("satisfactory_foundation.pad", "id"),
					captureID("satisfactory_foundation.pad", &id),
				),
			},
			{
				// Re-applying identical config must be a no-op.
				Config:   config,
				PlanOnly: true,
			},
			{
				// Simulate dismantling it in-game.
				PreConfig: func() {
					if err := c.DeleteBuildable(context.Background(), id); err != nil {
						t.Fatalf("out-of-band delete: %v", err)
					}
				},
				Config:             config,
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
		},
	})
}

// TestAccBeltDriftAndReplace covers the belt/power-line-shaped
// connectionResource, which TestAccFullLine only exercises for a clean
// create + no-op plan: this adds out-of-band-delete drift detection and a
// RequiresReplace check on a connector index change.
func TestAccBeltDriftAndReplace(t *testing.T) {
	c := startMock(t)

	config := `
resource "satisfactory_building" "a" {
  class = "Build_SmelterMk1_C"
  x     = 0
  y     = 0
  z     = 0
}

resource "satisfactory_building" "b" {
  class = "Build_SmelterMk1_C"
  x     = 1000
  y     = 0
  z     = 0
}

resource "satisfactory_belt" "line" {
  class          = "Build_ConveyorBeltMk1_C"
  from_id        = satisfactory_building.a.id
  from_connector = 0
  to_id          = satisfactory_building.b.id
  to_connector   = 0
}
`
	movedConnector := `
resource "satisfactory_building" "a" {
  class = "Build_SmelterMk1_C"
  x     = 0
  y     = 0
  z     = 0
}

resource "satisfactory_building" "b" {
  class = "Build_SmelterMk1_C"
  x     = 1000
  y     = 0
  z     = 0
}

resource "satisfactory_belt" "line" {
  class          = "Build_ConveyorBeltMk1_C"
  from_id        = satisfactory_building.a.id
  from_connector = 0
  to_id          = satisfactory_building.b.id
  to_connector   = 1
}
`
	var beltID string
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: protoFactories,
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttrSet("satisfactory_belt.line", "id"),
					captureID("satisfactory_belt.line", &beltID),
				),
			},
			{
				// to_connector changed: RequiresReplace, expect a new id.
				Config: movedConnector,
				Check:  checkDifferentID("satisfactory_belt.line", &beltID),
			},
			{
				// Simulate dismantling the belt in-game.
				PreConfig: func() {
					if err := c.DeleteConnection(context.Background(), beltID); err != nil {
						t.Fatalf("out-of-band delete: %v", err)
					}
				},
				Config:             movedConnector,
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
		},
	})
}

// TestAccBuildingNonManufacturerClockSpeedOmitted is the acceptance-level
// counterpart to TestBuildingCreate_DefaultsUnsetClockSpeedTo1 in
// resource_internal_test.go. That test drives buildingResource.Create
// directly; this one goes through a full terraform apply + plan cycle via
// terraform-plugin-testing, which is the only way to actually catch a
// "provider produced inconsistent result" plan-time error if one ever
// regressed.
//
// It deliberately does NOT use startMock()/internal/mockserver: that mock
// unconditionally defaults ClockSpeed to 1.0 for every class (see
// internal/mockserver/server_test.go's
// TestBuildableClockSpeedDefaultIgnoresClass for why - a documented,
// out-of-scope fidelity gap vs. the real mod), so it can never produce the
// "clock_speed key absent from the response" shape this test needs. Instead
// this spins up a tiny purpose-built fake API that omits clock_speed from
// its create/read responses, mirroring the real mod's BuildableToJson for a
// buildable that isn't a manufacturer.
func TestAccBuildingNonManufacturerClockSpeedOmitted(t *testing.T) {
	var created api.Buildable
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/health":
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/buildables":
			var posted api.Buildable
			_ = json.NewDecoder(r.Body).Decode(&posted)
			// Omit clock_speed, like the real mod does for a
			// non-manufacturer buildable (see the ClockSpeed comment on
			// api.Buildable and resource_building.go's Schema/Create).
			created = api.Buildable{TFID: posted.TFID, Class: posted.Class, Transform: posted.Transform}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(created)
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v1/buildables/"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(created)
		case r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	t.Setenv("SATISFACTORY_ENDPOINT", srv.URL)

	config := `
resource "satisfactory_building" "splitter" {
  class = "Build_SplitterMk1_C"
  x     = 0
  y     = 0
  z     = 0
}
`
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: protoFactories,
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeTestCheckFunc(
					// The fake API echoed back an omitted (zero-value)
					// clock_speed, even though Create() sent 1.0 - proving
					// the provider stores the API's actual response
					// rather than fighting it.
					resource.TestCheckResourceAttr("satisfactory_building.splitter", "clock_speed", "0"),
					resource.TestCheckResourceAttrSet("satisfactory_building.splitter", "id"),
				),
			},
			{
				// The actual regression this test guards: re-planning
				// must NOT produce a "provider produced inconsistent
				// result" error or a perpetual diff, despite clock_speed
				// having no static schema Default.
				Config:   config,
				PlanOnly: true,
			},
		},
	})
}

func TestAccExtractorLifecycle(t *testing.T) {
	c := startMock(t)

	// include_occupied + the ownership filter: once the miner stands on a
	// node, that node must keep its place in the list or the next plan picks
	// the next free node and replaces the miner.
	config := `
data "satisfactory_resource_nodes" "iron" {
  resource         = "Desc_OreIron_C"
  x                = -77737
  y                = 220338
  z                = -337
  include_occupied = true
}

locals {
  usable = [for n in data.satisfactory_resource_nodes.iron.nodes : n if n.purity != "impure" && (!n.occupied || n.occupant_tf_id != "")]
}

resource "satisfactory_extractor" "miner" {
  class   = "Build_MinerMk1_C"
  node_id = local.usable[0].id
}
`
	updated := `
data "satisfactory_resource_nodes" "iron" {
  resource         = "Desc_OreIron_C"
  x                = -77737
  y                = 220338
  z                = -337
  include_occupied = true
}

locals {
  usable = [for n in data.satisfactory_resource_nodes.iron.nodes : n if n.purity != "impure" && (!n.occupied || n.occupant_tf_id != "")]
}

resource "satisfactory_extractor" "miner" {
  class       = "Build_MinerMk1_C"
  node_id     = local.usable[0].id
  clock_speed = 1.5
}
`
	var firstID string
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: protoFactories,
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("satisfactory_extractor.miner", "clock_speed", "1"),
					resource.TestCheckResourceAttrSet("satisfactory_extractor.miner", "id"),
					resource.TestCheckResourceAttrSet("satisfactory_extractor.miner", "x"),
					resource.TestCheckResourceAttrSet("satisfactory_extractor.miner", "z"),
					// Nearest non-impure iron node to the anchor in the mock geography.
					resource.TestCheckResourceAttr("satisfactory_extractor.miner", "node_id", "Persistent_Level:PersistentLevel.BP_ResourceNode_Iron_1"),
					captureID("satisfactory_extractor.miner", &firstID),
				),
			},
			{
				// The transform the game reported is computed state, not
				// config: re-planning must not try to "fix" it.
				Config:   config,
				PlanOnly: true,
			},
			{
				// Same node, only the clock changes: in place.
				Config: updated,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("satisfactory_extractor.miner", "clock_speed", "1.5"),
					checkSameID("satisfactory_extractor.miner", &firstID),
				),
			},
			{
				// Dismantled in-game: refresh must detect the loss.
				PreConfig: func() {
					if err := c.DeleteBuildable(context.Background(), firstID); err != nil {
						t.Fatalf("out-of-band delete: %v", err)
					}
				},
				Config:             updated,
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
		},
	})
}

func TestAccExtractorRejectsBadNode(t *testing.T) {
	startMock(t)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: protoFactories,
		Steps: []resource.TestStep{
			{
				// A hand-built miner is already on this node.
				Config: `
resource "satisfactory_extractor" "miner" {
  class   = "Build_MinerMk1_C"
  node_id = "Persistent_Level:PersistentLevel.BP_ResourceNode_Iron_4"
}
`,
				ExpectError: regexp.MustCompile(`already occupied`),
			},
			{
				Config: `
resource "satisfactory_extractor" "pump" {
  class   = "Build_WaterPump_C"
  node_id = "Persistent_Level:PersistentLevel.BP_ResourceNode_Iron_1"
}
`,
				ExpectError: regexp.MustCompile(`cannot extract`),
			},
		},
	})
}

func TestAccWaterExtractorPlacement(t *testing.T) {
	startMock(t)
	config := `
data "satisfactory_resource_nodes" "water" {
  resource = "Desc_Water_C"
}

resource "satisfactory_extractor" "pump" {
  class   = "Build_WaterPump_C"
  node_id = data.satisfactory_resource_nodes.water.nodes[0].id
  x       = data.satisfactory_resource_nodes.water.nodes[0].x + 1500
  y       = data.satisfactory_resource_nodes.water.nodes[0].y
  yaw     = 90
}
`
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: protoFactories,
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("satisfactory_extractor.pump", "x", "-74237"),
					resource.TestCheckResourceAttr("satisfactory_extractor.pump", "yaw", "90"),
					resource.TestCheckResourceAttr("satisfactory_extractor.pump", "z", "-340"),
				),
			},
			{Config: config, PlanOnly: true},
		},
	})
}

func TestAccResourceNodesDataSource(t *testing.T) {
	startMock(t)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: protoFactories,
		Steps: []resource.TestStep{
			{
				Config: `
data "satisfactory_resource_nodes" "coal" {
  resource = "Desc_Coal_C"
  purity   = "normal"
  x        = -77737
  y        = 220338
  z        = -337
  radius   = 20000
}
`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("data.satisfactory_resource_nodes.coal", "nodes.#", "2"),
					resource.TestCheckResourceAttr("data.satisfactory_resource_nodes.coal", "nodes.0.id", "Persistent_Level:PersistentLevel.BP_ResourceNode_Coal_1"),
					resource.TestCheckResourceAttr("data.satisfactory_resource_nodes.coal", "nodes.0.form", "solid"),
					resource.TestCheckResourceAttr("data.satisfactory_resource_nodes.coal", "nodes.0.occupied", "false"),
				),
			},
			{
				Config: `
data "satisfactory_resource_nodes" "bad" {
  radius = 100
}
`,
				ExpectError: regexp.MustCompile(`radius`),
			},
		},
	})
}

// TestAccNodeCoordinateArithmeticPlans guards the whole-centimetre rounding
// in the data source. The game reports positions as arbitrary doubles; a
// plan whose config does arithmetic on one of those ("node.x - 600") failed
// with "planned value does not match config value" because Terraform
// compares a double-precision planned number with its 512-bit config twin
// by decimal text (seen live). Rounded to whole centimetres, the text is the
// same at either precision and the plan goes through.
func TestAccNodeCoordinateArithmeticPlans(t *testing.T) {
	s := mockserver.New("")
	srv := httptest.NewServer(s.Handler())
	t.Cleanup(srv.Close)
	t.Setenv("SATISFACTORY_ENDPOINT", srv.URL)
	s.SeedResourceNodes([]api.ResourceNode{{
		ID: "Persistent_Level:PersistentLevel.FGWaterVolume78", Resource: "Desc_Water_C", Form: "liquid", Type: "water",
		X: -75539.999099824796, Y: 231719.99962137232, Z: -2630,
		BoundsMin: &api.Vec3{X: -81773.951078381157, Y: 226153.18582960669, Z: -3630},
		BoundsMax: &api.Vec3{X: -69305.99, Y: 237286.81, Z: -2630},
	}})
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: protoFactories,
		Steps: []resource.TestStep{
			{
				Config: `
data "satisfactory_resource_nodes" "water" {
  resource = "Desc_Water_C"
}

locals {
  w = data.satisfactory_resource_nodes.water.nodes[0]
}

resource "satisfactory_building" "pole" {
  class = "Build_PowerPoleMk1_C"
  x     = local.w.x - 600
  y     = local.w.bounds_min.y - 3000
  z     = local.w.z + 50
  yaw   = 0
}
`,
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("data.satisfactory_resource_nodes.water", "nodes.0.x", "-75540"),
					resource.TestCheckResourceAttr("data.satisfactory_resource_nodes.water", "nodes.0.y", "231720"),
					resource.TestCheckResourceAttr("data.satisfactory_resource_nodes.water", "nodes.0.bounds_min.y", "226153"),
				),
			},
		},
	})
}

// TestAccPreviewOnPlan turns previewing on and checks the hologram flow:
// a plan leaves ghosts standing, apply replaces them, and a placement the
// game would refuse surfaces as a warning rather than failing the plan.
func TestAccPreviewOnPlan(t *testing.T) {
	s := mockserver.New("")
	srv := httptest.NewServer(s.Handler())
	t.Cleanup(srv.Close)
	t.Setenv("SATISFACTORY_ENDPOINT", srv.URL)
	t.Setenv("SATISFACTORY_PREVIEW", "1")

	config := `
resource "satisfactory_foundation" "pad" {
  class = "Build_Foundation_8x1_01_C"
  x     = 0
  y     = 0
  z     = 0
}

resource "satisfactory_building" "smelter" {
  class  = "Build_SmelterMk1_C"
  x      = 0
  y      = 0
  z      = 50
  recipe = "Recipe_IngotIron_C"
}

data "satisfactory_resource_nodes" "iron" {
  resource         = "Desc_OreIron_C"
  purity           = "pure"
  include_occupied = true
}

locals {
  iron = [for n in data.satisfactory_resource_nodes.iron.nodes : n if !n.occupied || n.occupant_tf_id != ""]
}

resource "satisfactory_extractor" "miner" {
  class   = "Build_MinerMk1_C"
  node_id = local.iron[0].id
}
`
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: protoFactories,
		Steps: []resource.TestStep{
			{
				Config:             config,
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
			{
				PreConfig: func() {
					// The plan above previewed three placements; nothing
					// was built, so all three ghosts still stand.
					if got := s.PreviewCount(); got != 3 {
						t.Fatalf("after plan, %d holograms standing, want 3", got)
					}
				},
				Config: config,
			},
			{
				PreConfig: func() {
					// Apply replaced each ghost with its building.
					if got := s.PreviewCount(); got != 0 {
						t.Fatalf("after apply, %d holograms standing, want 0", got)
					}
				},
				Config:   config,
				PlanOnly: true,
			},
			{
				// A second foundation on the pad's spot: the mock's hologram
				// flags it, the plan still succeeds (warning, not error),
				// and apply then hits the real 409.
				Config: config + `
resource "satisfactory_foundation" "clash" {
  class = "Build_Foundation_8x1_01_C"
  x     = 0
  y     = 0
  z     = 0
}
`,
				ExpectError: regexp.MustCompile(`already exists at that\s+position`),
			},
		},
	})
}
