package mockserver_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/daroco/terraform-provider-satisfactory/internal/api"
	"github.com/daroco/terraform-provider-satisfactory/internal/client"
	"github.com/daroco/terraform-provider-satisfactory/internal/mockserver"
)

func newNodeTestServer(t *testing.T) (*mockserver.Server, *client.Client) {
	t.Helper()
	s := mockserver.New("")
	srv := httptest.NewServer(s.Handler())
	t.Cleanup(srv.Close)
	return s, client.New(srv.URL, "")
}

func apiStatus(t *testing.T, err error) int {
	t.Helper()
	var ae *client.APIError
	if !errors.As(err, &ae) {
		t.Fatalf("expected *client.APIError, got %v", err)
	}
	return ae.Status
}

// A fixed geography with the nearest node NOT first in seed order, so a
// test can tell distance sorting from insertion order.
func fixtureNodes() []api.ResourceNode {
	return []api.ResourceNode{
		{ID: "n-far", Resource: "Desc_OreIron_C", Purity: "pure", Form: "solid", Type: "node", X: 9000, Y: 0, Z: 0},
		{ID: "n-near", Resource: "Desc_OreIron_C", Purity: "normal", Form: "solid", Type: "node", X: 1000, Y: 0, Z: 0},
		{ID: "n-mid", Resource: "Desc_OreIron_C", Purity: "impure", Form: "solid", Type: "node", X: 5000, Y: 0, Z: 0},
		{ID: "n-taken", Resource: "Desc_OreIron_C", Purity: "pure", Form: "solid", Type: "node", X: 2000, Y: 0, Z: 0, Occupied: true},
		{ID: "n-copper", Resource: "Desc_OreCopper_C", Purity: "normal", Form: "solid", Type: "node", X: 3000, Y: 0, Z: 0},
		{ID: "n-oil", Resource: "Desc_LiquidOil_C", Purity: "normal", Form: "liquid", Type: "node", X: 4000, Y: 0, Z: 0},
		{ID: "w-lake", Resource: "Desc_Water_C", Form: "liquid", Type: "water", X: 0, Y: 5000, Z: -10,
			BoundsMin: &api.Vec3{X: -1000, Y: 4000, Z: -500}, BoundsMax: &api.Vec3{X: 1000, Y: 6000, Z: -10}},
	}
}

func ids(nodes []api.ResourceNode) []string {
	out := make([]string, len(nodes))
	for i, n := range nodes {
		out[i] = n.ID
	}
	return out
}

func equalIDs(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestResourceNodesListingFiltersAndOrder(t *testing.T) {
	s, c := newNodeTestServer(t)
	s.SeedResourceNodes(fixtureNodes())
	ctx := context.Background()

	// No centre: by id, occupied hidden by default.
	all, err := c.ListResourceNodes(ctx, api.ResourceNodeQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := ids(all), []string{"n-copper", "n-far", "n-mid", "n-near", "n-oil", "w-lake"}; !equalIDs(got, want) {
		t.Errorf("default order = %v, want %v", got, want)
	}

	// With a centre: by distance, not seed order.
	iron, err := c.ListResourceNodes(ctx, api.ResourceNodeQuery{
		Resource: "Desc_OreIron_C", Center: &api.Vec3{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := ids(iron), []string{"n-near", "n-mid", "n-far"}; !equalIDs(got, want) {
		t.Errorf("distance order = %v, want %v", got, want)
	}

	// include_occupied brings the taken node back, in its distance slot.
	withTaken, err := c.ListResourceNodes(ctx, api.ResourceNodeQuery{
		Resource: "Desc_OreIron_C", Center: &api.Vec3{}, IncludeOccupied: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := ids(withTaken), []string{"n-near", "n-taken", "n-mid", "n-far"}; !equalIDs(got, want) {
		t.Errorf("include_occupied order = %v, want %v", got, want)
	}

	pure, err := c.ListResourceNodes(ctx, api.ResourceNodeQuery{Purity: "pure"})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := ids(pure), []string{"n-far"}; !equalIDs(got, want) {
		t.Errorf("purity filter = %v, want %v", got, want)
	}

	radius := 3500.0
	near, err := c.ListResourceNodes(ctx, api.ResourceNodeQuery{Center: &api.Vec3{}, Radius: &radius})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := ids(near), []string{"n-near", "n-copper"}; !equalIDs(got, want) {
		t.Errorf("radius filter = %v, want %v", got, want)
	}

	// Water volumes carry bounds; nodes do not.
	water, err := c.ListResourceNodes(ctx, api.ResourceNodeQuery{Resource: "Desc_Water_C"})
	if err != nil {
		t.Fatal(err)
	}
	if len(water) != 1 || water[0].BoundsMin == nil || water[0].BoundsMax == nil || water[0].Type != "water" {
		t.Errorf("water volume = %+v, want one entry with bounds", water)
	}
	if iron[0].BoundsMin != nil {
		t.Error("a resource node should not carry bounds")
	}
}

func TestResourceNodesListingValidation(t *testing.T) {
	_, c := newNodeTestServer(t)
	ctx := context.Background()
	radius := 10.0
	if _, err := c.ListResourceNodes(ctx, api.ResourceNodeQuery{Radius: &radius}); apiStatus(t, err) != http.StatusUnprocessableEntity {
		t.Error("radius without a centre should 422")
	}
	if _, err := c.ListResourceNodes(ctx, api.ResourceNodeQuery{Purity: "shiny"}); apiStatus(t, err) != http.StatusUnprocessableEntity {
		t.Error("unknown purity should 422")
	}
}

func TestDefaultGeographyServesTheExample(t *testing.T) {
	// The tier4-factory example picks, from the anchor, two non-impure iron
	// nodes, one copper, one limestone, two coal and one water volume. The
	// seeded world has to make that selection possible without -seed flags,
	// since CI applies every example against a bare mockserver.
	_, c := newNodeTestServer(t)
	ctx := context.Background()
	center := &api.Vec3{X: -77737, Y: 220338, Z: -337}
	want := map[string]int{"Desc_OreIron_C": 2, "Desc_OreCopper_C": 1, "Desc_Stone_C": 1, "Desc_Coal_C": 2, "Desc_Water_C": 1}
	for resource, n := range want {
		nodes, err := c.ListResourceNodes(ctx, api.ResourceNodeQuery{Resource: resource, Center: center})
		if err != nil {
			t.Fatal(err)
		}
		usable := 0
		for _, node := range nodes {
			if node.Purity != "impure" {
				usable++
			}
		}
		if usable < n {
			t.Errorf("%s: %d usable free nodes, example needs %d", resource, usable, n)
		}
	}
}

func TestExtractorLifecycleOccupiesAndFreesNode(t *testing.T) {
	s, c := newNodeTestServer(t)
	s.SeedResourceNodes(fixtureNodes())
	ctx := context.Background()

	yaw := 45.0
	created, err := c.CreateBuildable(ctx, api.Buildable{
		TFID: "m-1", Class: "Build_MinerMk1_C", NodeID: "n-near", Placement: &api.Placement{Yaw: &yaw},
	})
	if err != nil {
		t.Fatalf("create miner: %v", err)
	}
	if created.Transform.X != 1000 || created.Transform.Yaw != 45 {
		t.Errorf("miner transform = %+v, want node position with yaw override", created.Transform)
	}
	if created.ClockSpeed != 1.0 {
		t.Errorf("clock_speed = %v, want 1.0 default (miners can be clocked)", created.ClockSpeed)
	}
	if created.NodeID != "n-near" || created.Placement != nil {
		t.Errorf("response should echo node_id and drop placement: %+v", created)
	}

	nodes, err := c.ListResourceNodes(ctx, api.ResourceNodeQuery{Resource: "Desc_OreIron_C", IncludeOccupied: true, Center: &api.Vec3{}})
	if err != nil {
		t.Fatal(err)
	}
	if nodes[0].ID != "n-near" || !nodes[0].Occupied || nodes[0].OccupantTFID != "m-1" {
		t.Errorf("node after create = %+v, want occupied by m-1", nodes[0])
	}
	free, _ := c.ListResourceNodes(ctx, api.ResourceNodeQuery{Resource: "Desc_OreIron_C", Center: &api.Vec3{}})
	if free[0].ID == "n-near" {
		t.Error("occupied node should be hidden by default")
	}

	// A second extractor on the same node is a 409 and leaves nothing behind.
	_, err = c.CreateBuildable(ctx, api.Buildable{TFID: "m-2", Class: "Build_MinerMk2_C", NodeID: "n-near"})
	if apiStatus(t, err) != http.StatusConflict {
		t.Errorf("second miner on an occupied node: %v, want 409", err)
	}
	if _, err := c.GetBuildable(ctx, "m-2"); !client.IsNotFound(err) {
		t.Error("the refused miner must not exist")
	}

	// Clock patches in place; recipe is refused.
	clock := 1.5
	patched, err := c.PatchBuildable(ctx, "m-1", api.BuildablePatch{ClockSpeed: &clock})
	if err != nil || patched.ClockSpeed != 1.5 {
		t.Errorf("patch clock: %v %+v", err, patched)
	}
	recipe := "Recipe_IngotIron_C"
	if _, err := c.PatchBuildable(ctx, "m-1", api.BuildablePatch{Recipe: &recipe}); apiStatus(t, err) != http.StatusUnprocessableEntity {
		t.Error("recipe patch on a miner should 422")
	}

	// Export reports the binding.
	world, err := c.ListWorldBuildables(ctx, 1000, 0, 0, 100)
	if err != nil || len(world) != 1 || world[0].NodeID != "n-near" {
		t.Errorf("world export = %+v %v, want the miner with node_id", world, err)
	}

	// Dismantle frees the node; it can be used again.
	if err := c.DeleteBuildable(ctx, "m-1"); err != nil {
		t.Fatal(err)
	}
	free, _ = c.ListResourceNodes(ctx, api.ResourceNodeQuery{Resource: "Desc_OreIron_C", Center: &api.Vec3{}})
	if free[0].ID != "n-near" || free[0].Occupied || free[0].OccupantTFID != "" {
		t.Errorf("node after delete = %+v, want free", free[0])
	}
	if _, err := c.CreateBuildable(ctx, api.Buildable{TFID: "m-3", Class: "Build_MinerMk2_C", NodeID: "n-near"}); err != nil {
		t.Errorf("re-create on freed node: %v", err)
	}
}

func TestExtractorErrorContract(t *testing.T) {
	s, c := newNodeTestServer(t)
	s.SeedResourceNodes(fixtureNodes())
	ctx := context.Background()
	cases := []struct {
		name string
		b    api.Buildable
		want int
	}{
		{"extractor without node", api.Buildable{TFID: "e1", Class: "Build_MinerMk1_C", Transform: api.Transform{X: 1}}, 422},
		{"non-extractor with node", api.Buildable{TFID: "e2", Class: "Build_ConstructorMk1_C", NodeID: "n-near"}, 422},
		{"unknown node", api.Buildable{TFID: "e3", Class: "Build_MinerMk1_C", NodeID: "nope"}, 422},
		{"miner on oil", api.Buildable{TFID: "e4", Class: "Build_MinerMk1_C", NodeID: "n-oil"}, 422},
		{"miner on water", api.Buildable{TFID: "e5", Class: "Build_MinerMk1_C", NodeID: "w-lake"}, 422},
		{"water pump on ore", api.Buildable{TFID: "e6", Class: "Build_WaterPump_C", NodeID: "n-near"}, 422},
		{"hand-occupied node", api.Buildable{TFID: "e7", Class: "Build_MinerMk1_C", NodeID: "n-taken"}, 409},
	}
	for _, tc := range cases {
		_, err := c.CreateBuildable(ctx, tc.b)
		if got := apiStatus(t, err); got != tc.want {
			t.Errorf("%s: status %d, want %d (%v)", tc.name, got, tc.want, err)
		}
	}
}

func TestWaterPumpPlacementWithinVolume(t *testing.T) {
	s, c := newNodeTestServer(t)
	s.SeedResourceNodes(fixtureNodes())
	ctx := context.Background()

	x, y := 500.0, 5500.0
	created, err := c.CreateBuildable(ctx, api.Buildable{
		TFID: "w-1", Class: "Build_WaterPump_C", NodeID: "w-lake", Placement: &api.Placement{X: &x, Y: &y},
	})
	if err != nil {
		t.Fatalf("water pump: %v", err)
	}
	if created.Transform.X != 500 || created.Transform.Y != 5500 || created.Transform.Z != -10 {
		t.Errorf("transform = %+v, want placement x/y at surface z", created.Transform)
	}

	// Water is an area: a second pump elsewhere on the same lake is fine,
	// and the lake is not reported as occupied.
	x2 := -500.0
	if _, err := c.CreateBuildable(ctx, api.Buildable{
		TFID: "w-2", Class: "Build_WaterPump_C", NodeID: "w-lake", Placement: &api.Placement{X: &x2, Y: &y},
	}); err != nil {
		t.Errorf("second pump on the lake: %v", err)
	}

	outside := 9999.0
	_, err = c.CreateBuildable(ctx, api.Buildable{
		TFID: "w-3", Class: "Build_WaterPump_C", NodeID: "w-lake", Placement: &api.Placement{X: &outside},
	})
	if apiStatus(t, err) != http.StatusUnprocessableEntity {
		t.Errorf("placement outside bounds: %v, want 422", err)
	}
}

func TestGeneratorsAreClockable(t *testing.T) {
	_, c := newNodeTestServer(t)
	ctx := context.Background()
	g, err := c.CreateBuildable(ctx, api.Buildable{TFID: "g-1", Class: "Build_GeneratorCoal_C", Transform: api.Transform{X: 1}})
	if err != nil {
		t.Fatal(err)
	}
	if g.ClockSpeed != 1.0 {
		t.Errorf("generator clock_speed = %v, want 1.0 (generators can change potential)", g.ClockSpeed)
	}
	clock := 0.5
	if _, err := c.PatchBuildable(ctx, "g-1", api.BuildablePatch{ClockSpeed: &clock}); err != nil {
		t.Errorf("patch generator clock: %v", err)
	}
	// Still no recipe anywhere on it.
	recipe := "Recipe_IngotIron_C"
	if _, err := c.PatchBuildable(ctx, "g-1", api.BuildablePatch{Recipe: &recipe}); apiStatus(t, err) != http.StatusUnprocessableEntity {
		t.Error("recipe patch on a generator should 422")
	}
}

func TestCatalogPlacesExtractorsViaExtractorResource(t *testing.T) {
	_, c := newNodeTestServer(t)
	classes, err := c.ListClasses(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, cl := range classes {
		if cl.Class == "Build_MinerMk1_C" || cl.Class == "Build_WaterPump_C" {
			if !cl.Supported || cl.Resource != "satisfactory_extractor" || cl.Mechanism != "node_bound" {
				t.Errorf("%s = %+v, want node_bound supported via satisfactory_extractor", cl.Class, cl)
			}
		}
	}
}

func TestPreviewLifecycle(t *testing.T) {
	s, c := newNodeTestServer(t)
	s.SeedResourceNodes(fixtureNodes())
	now := time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)
	s.SetClock(func() time.Time { return now })
	ctx := context.Background()

	res, err := c.PreviewBuildable(ctx, api.Preview{
		TFID: "p-1", Class: "Build_ConstructorMk1_C", Transform: &api.Transform{X: 1, Y: 2, Z: 3, Yaw: 90},
	})
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	if res.Valid == nil || !*res.Valid || len(res.Disqualifiers) != 0 || res.Yaw != 90 || res.ExpiresIn != 120 {
		t.Errorf("preview result = %+v, want valid at the requested transform with the default ttl", res)
	}

	// Extractors preview at their node, and an occupied node shows red.
	res, err = c.PreviewBuildable(ctx, api.Preview{TFID: "p-2", Class: "Build_MinerMk1_C", NodeID: "n-taken", TTLSeconds: 30})
	if err != nil {
		t.Fatalf("preview miner: %v", err)
	}
	if res.Valid == nil || *res.Valid || len(res.Disqualifiers) == 0 || res.X != 2000 {
		t.Errorf("preview on occupied node = %+v, want invalid at the node", res)
	}

	// Same rules as a real spawn for the inputs that are config errors.
	if _, err := c.PreviewBuildable(ctx, api.Preview{TFID: "p-3", Class: "Build_MinerMk1_C"}); apiStatus(t, err) != http.StatusUnprocessableEntity {
		t.Error("extractor preview without node_id should 422")
	}
	if _, err := c.PreviewBuildable(ctx, api.Preview{TFID: "p-4", Class: "Build_ConstructorMk1_C"}); apiStatus(t, err) != http.StatusUnprocessableEntity {
		t.Error("coordinate preview without transform should 422")
	}

	// A foundation already standing where another is previewed shows red.
	if _, err := c.CreateBuildable(ctx, api.Buildable{TFID: "f-1", Class: "Build_Foundation_8x1_01_C", Transform: api.Transform{X: 800}}); err != nil {
		t.Fatal(err)
	}
	res, err = c.PreviewBuildable(ctx, api.Preview{TFID: "p-5", Class: "Build_Foundation_8x1_01_C", Transform: &api.Transform{X: 800}})
	if err != nil || res.Valid == nil || *res.Valid {
		t.Errorf("preview over an existing foundation = %+v %v, want invalid", res, err)
	}

	// p-1, p-2 and p-5 stand; the two 422s left nothing.
	if got := s.PreviewCount(); got != 3 {
		t.Errorf("standing previews = %d, want 3", got)
	}

	// The real spawn with the same key removes its ghost.
	if _, err := c.CreateBuildable(ctx, api.Buildable{TFID: "p-1", Class: "Build_ConstructorMk1_C", Transform: api.Transform{X: 1, Y: 2, Z: 3, Yaw: 90}}); err != nil {
		t.Fatal(err)
	}
	if got := s.PreviewCount(); got != 2 {
		t.Errorf("after spawning p-1, standing previews = %d, want 2", got)
	}

	// Time passes: the 30 s one expires, the 120 s one stays.
	now = now.Add(31 * time.Second)
	list, err := c.ListPreviews(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].TFID != "p-5" {
		t.Errorf("after 31s, previews listed = %+v, want only p-5 (p-2 expired)", list)
	}
	if len(list) == 1 && (list[0].ExpiresIn <= 0 || list[0].ExpiresIn > 89) {
		t.Errorf("p-5 expires_in = %d, want the remaining time", list[0].ExpiresIn)
	}

	// Clearing an expired or unknown preview is fine.
	if err := c.ClearPreview(ctx, "p-2"); err != nil {
		t.Errorf("clearing an expired preview should be fine: %v", err)
	}
	removed, err := c.ClearPreviews(ctx)
	if err != nil || removed != 1 {
		t.Errorf("clear all removed %d (%v), want 1", removed, err)
	}
	if got := s.PreviewCount(); got != 0 {
		t.Errorf("after clear, standing previews = %d, want 0", got)
	}
}
