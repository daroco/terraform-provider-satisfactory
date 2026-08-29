package mockserver_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/daroco/terraform-provider-satisfactory/internal/api"
	"github.com/daroco/terraform-provider-satisfactory/internal/client"
	"github.com/daroco/terraform-provider-satisfactory/internal/mockserver"
)

func newTestClient(t *testing.T) *client.Client {
	t.Helper()
	srv := httptest.NewServer(mockserver.New("").Handler())
	t.Cleanup(srv.Close)
	return client.New(srv.URL, "")
}

// newTestServerAndClient is like newTestClient but also hands back the raw
// httptest.Server, for tests that need to hit the mock without going
// through the typed client (e.g. auth header checks, world/health).
func newTestServerAndClient(t *testing.T, token string) (*httptest.Server, *client.Client) {
	t.Helper()
	srv := httptest.NewServer(mockserver.New(token).Handler())
	t.Cleanup(srv.Close)
	return srv, client.New(srv.URL, token)
}

func TestBuildableLifecycle(t *testing.T) {
	c := newTestClient(t)
	ctx := context.Background()

	if err := c.Health(ctx); err != nil {
		t.Fatalf("health: %v", err)
	}

	b := api.Buildable{
		TFID:      "b-1",
		Class:     "Build_ConstructorMk1_C",
		Transform: api.Transform{X: 100, Y: 200, Z: 0, Yaw: 90},
		Recipe:    "Recipe_IronPlate_C",
	}
	created, err := c.CreateBuildable(ctx, b)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.ClockSpeed != 1.0 {
		t.Errorf("clock_speed default = %v, want 1.0", created.ClockSpeed)
	}

	if _, err := c.CreateBuildable(ctx, b); err == nil {
		t.Error("duplicate tf_id should 409")
	}

	clock := 1.5
	patched, err := c.PatchBuildable(ctx, "b-1", api.BuildablePatch{ClockSpeed: &clock})
	if err != nil {
		t.Fatalf("patch: %v", err)
	}
	if patched.ClockSpeed != 1.5 || patched.Recipe != "Recipe_IronPlate_C" {
		t.Errorf("patch result = %+v", patched)
	}

	if err := c.DeleteBuildable(ctx, "b-1"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	_, err = c.GetBuildable(ctx, "b-1")
	if !client.IsNotFound(err) {
		t.Errorf("get after delete: want NotFound, got %v", err)
	}
	// Idempotent destroy.
	if err := c.DeleteBuildable(ctx, "b-1"); err != nil {
		t.Errorf("second delete should be nil, got %v", err)
	}
}

func TestBuildableValidation(t *testing.T) {
	c := newTestClient(t)
	ctx := context.Background()

	cases := []api.Buildable{
		{TFID: "", Class: "Build_SmelterMk1_C"},                       // missing tf_id
		{TFID: "x", Class: "Smelter"},                                 // bad class name
		{TFID: "x", Class: "Build_SmelterMk1_C", Recipe: "IronIngot"}, // bad recipe name
		{TFID: "x", Class: "Build_SmelterMk1_C", ClockSpeed: 9.0},     // clock out of range
	}
	for _, b := range cases {
		if _, err := c.CreateBuildable(ctx, b); err == nil {
			t.Errorf("create %+v: want validation error", b)
		}
	}
}

func TestConnectionLifecycle(t *testing.T) {
	c := newTestClient(t)
	ctx := context.Background()

	for _, id := range []string{"m-1", "m-2"} {
		if _, err := c.CreateBuildable(ctx, api.Buildable{TFID: id, Class: "Build_SmelterMk1_C"}); err != nil {
			t.Fatalf("create %s: %v", id, err)
		}
	}

	conn := api.Connection{
		TFID:  "c-1",
		Class: "Build_ConveyorBeltMk1_C",
		From:  api.ConnectionEndpoint{BuildableTFID: "m-1", Connector: 0},
		To:    api.ConnectionEndpoint{BuildableTFID: "m-2", Connector: 0},
	}
	if _, err := c.CreateConnection(ctx, conn); err != nil {
		t.Fatalf("create connection: %v", err)
	}

	// Unknown endpoint must be rejected.
	bad := conn
	bad.TFID = "c-2"
	bad.To.BuildableTFID = "nope"
	if _, err := c.CreateConnection(ctx, bad); err == nil {
		t.Error("connection to unknown buildable should 422")
	}

	// Deleting a buildable with an attached connection is allowed - matches
	// the real mod/game (dismantling a connected buildable isn't blocked,
	// the belt just dangles) - but the connection it was part of should no
	// longer resolve afterward, since one of its endpoints is now gone.
	if err := c.DeleteBuildable(ctx, "m-1"); err != nil {
		t.Fatalf("delete buildable with attached connection: %v", err)
	}
	if _, err := c.GetConnection(ctx, "c-1"); !client.IsNotFound(err) {
		t.Errorf("connection referencing a deleted buildable should now 404, got: %v", err)
	}
	if err := c.DeleteBuildable(ctx, "m-2"); err != nil {
		t.Errorf("delete remaining buildable: %v", err)
	}
}

// TestConnectionErrorContract drives the 404/409/422 contract (CLAUDE.md:
// "Error contract: 404 unknown tf_id, 409 duplicate tf_id, 422 validation")
// specifically through /api/v1/connections, which TestConnectionLifecycle
// only partly exercises.
func TestConnectionErrorContract(t *testing.T) {
	c := newTestClient(t)
	ctx := context.Background()

	for _, id := range []string{"m-1", "m-2"} {
		if _, err := c.CreateBuildable(ctx, api.Buildable{TFID: id, Class: "Build_SmelterMk1_C"}); err != nil {
			t.Fatalf("create %s: %v", id, err)
		}
	}
	base := api.Connection{
		TFID:  "c-1",
		Class: "Build_ConveyorBeltMk1_C",
		From:  api.ConnectionEndpoint{BuildableTFID: "m-1", Connector: 0},
		To:    api.ConnectionEndpoint{BuildableTFID: "m-2", Connector: 0},
	}

	// 404: reading or deleting an unknown connection.
	if _, err := c.GetConnection(ctx, "nope"); !client.IsNotFound(err) {
		t.Errorf("get unknown connection: want NotFound, got %v", err)
	}
	if err := c.DeleteConnection(ctx, "nope"); err != nil {
		t.Errorf("delete unknown connection should be idempotent-nil, got %v", err)
	}

	// 422: missing tf_id, bad class, negative connector index.
	badCases := []api.Connection{
		{TFID: "", Class: "Build_ConveyorBeltMk1_C", From: base.From, To: base.To},                                                      // missing tf_id
		{TFID: "c-2", Class: "ConveyorBelt", From: base.From, To: base.To},                                                              // bad class name
		{TFID: "c-2", Class: "Build_ConveyorBeltMk1_C", From: api.ConnectionEndpoint{BuildableTFID: "m-1", Connector: -1}, To: base.To}, // negative connector
	}
	for _, bad := range badCases {
		if _, err := c.CreateConnection(ctx, bad); err == nil {
			t.Errorf("create %+v: want validation error", bad)
		} else if client.IsNotFound(err) {
			t.Errorf("create %+v: validation error misclassified as NotFound", bad)
		}
	}

	// Happy path, then 409 on a duplicate tf_id.
	if _, err := c.CreateConnection(ctx, base); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := c.CreateConnection(ctx, base); err == nil {
		t.Error("duplicate connection tf_id should 409")
	}

	// 404 again: after deleting, both Get and a second Delete report gone.
	if err := c.DeleteConnection(ctx, "c-1"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := c.GetConnection(ctx, "c-1"); !client.IsNotFound(err) {
		t.Errorf("get after delete: want NotFound, got %v", err)
	}
}

// TestBuildableErrorContract fills the gaps TestBuildableLifecycle and
// TestBuildableValidation don't cover for PATCH: unknown tf_id (404) and an
// out-of-range/invalid patch (422).
func TestBuildableErrorContract(t *testing.T) {
	c := newTestClient(t)
	ctx := context.Background()

	badRecipe := "IronIngot" // no Recipe_ prefix
	if _, err := c.PatchBuildable(ctx, "nope", api.BuildablePatch{Recipe: &badRecipe}); !client.IsNotFound(err) {
		t.Errorf("patch unknown tf_id: want NotFound, got %v", err)
	}

	if _, err := c.CreateBuildable(ctx, api.Buildable{TFID: "b-1", Class: "Build_SmelterMk1_C"}); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := c.PatchBuildable(ctx, "b-1", api.BuildablePatch{Recipe: &badRecipe}); err == nil {
		t.Error("patch with invalid recipe class should 422")
	} else if client.IsNotFound(err) {
		t.Error("422 misclassified as NotFound")
	}
	tooHot := 9.0
	if _, err := c.PatchBuildable(ctx, "b-1", api.BuildablePatch{ClockSpeed: &tooHot}); err == nil {
		t.Error("patch with out-of-range clock_speed should 422")
	}
}

// TestBuildableManufacturerFidelity pins the mock's manufacturer-vs-not
// behavior to the real mod's (issues #5/#8): manufacturers get a 1.0
// clock_speed default and accept PATCH; everything else has recipe/clock
// silently dropped on create (so both fields are omitted from responses,
// decoding to zero values) and 422s on PATCH with the mod's own message.
func TestBuildableManufacturerFidelity(t *testing.T) {
	c := newTestClient(t)
	ctx := context.Background()

	smelter, err := c.CreateBuildable(ctx, api.Buildable{TFID: "man-1", Class: "Build_SmelterMk1_C"})
	if err != nil {
		t.Fatalf("create manufacturer: %v", err)
	}
	if smelter.ClockSpeed != 1.0 {
		t.Errorf("manufacturer ClockSpeed default = %v, want 1.0", smelter.ClockSpeed)
	}

	// Non-manufacturer with a clock but no recipe: clock is ignored, not
	// stored. clock_speed MUST stay silently ignored rather than 422 - the
	// provider defaults it to 1.0 on every building create (see
	// resource_building.go), so every merger and splitter sends one, and
	// rejecting it would fail every such apply.
	merger, err := c.CreateBuildable(ctx, api.Buildable{
		TFID: "att-1", Class: "Build_ConveyorAttachmentMerger_C", ClockSpeed: 1.5,
	})
	if err != nil {
		t.Fatalf("create non-manufacturer: %v", err)
	}
	if merger.ClockSpeed != 0 || merger.Recipe != "" {
		t.Errorf("non-manufacturer echoed recipe=%q clock=%v, want both omitted (zero)", merger.Recipe, merger.ClockSpeed)
	}

	// A recipe, by contrast, is only ever sent when someone explicitly asked
	// for it, so an impossible one is a config error worth surfacing rather
	// than discarding - see TestRecipeMustFitMachine.
	if _, err := c.CreateBuildable(ctx, api.Buildable{
		TFID: "att-2", Class: "Build_ConveyorAttachmentMerger_C", Recipe: "Recipe_IronPlate_C",
	}); err == nil {
		t.Error("a recipe on a non-manufacturer should be refused, not silently dropped")
	}

	clock := 2.0
	if _, err := c.PatchBuildable(ctx, "att-1", api.BuildablePatch{ClockSpeed: &clock}); err == nil {
		t.Error("PATCH on a non-manufacturer should 422, same as the real mod")
	}
	if _, err := c.PatchBuildable(ctx, "man-1", api.BuildablePatch{ClockSpeed: &clock}); err != nil {
		t.Errorf("PATCH on a manufacturer should still work: %v", err)
	}
}

// TestListBuildablesAndConnections exercises the list endpoints directly
// over HTTP: the typed client (internal/client) doesn't expose list
// methods, only the individual CRUD ones the provider needs.
func TestListBuildablesAndConnections(t *testing.T) {
	srv, c := newTestServerAndClient(t, "")
	ctx := context.Background()

	// Empty world: both lists come back as an empty JSON array, not null.
	assertListLength(t, srv.URL+"/api/v1/buildables", 0)
	assertListLength(t, srv.URL+"/api/v1/connections", 0)

	for _, id := range []string{"m-1", "m-2"} {
		if _, err := c.CreateBuildable(ctx, api.Buildable{TFID: id, Class: "Build_SmelterMk1_C"}); err != nil {
			t.Fatalf("create %s: %v", id, err)
		}
	}
	assertListLength(t, srv.URL+"/api/v1/buildables", 2)

	if _, err := c.CreateConnection(ctx, api.Connection{
		TFID:  "c-1",
		Class: "Build_ConveyorBeltMk1_C",
		From:  api.ConnectionEndpoint{BuildableTFID: "m-1", Connector: 0},
		To:    api.ConnectionEndpoint{BuildableTFID: "m-2", Connector: 0},
	}); err != nil {
		t.Fatalf("create connection: %v", err)
	}
	assertListLength(t, srv.URL+"/api/v1/connections", 1)
}

func assertListLength(t *testing.T, url string, want int) {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s: status %d", url, resp.StatusCode)
	}
	var items []json.RawMessage
	if err := json.NewDecoder(resp.Body).Decode(&items); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(items) != want {
		t.Errorf("GET %s: got %d items, want %d", url, len(items), want)
	}
}

func TestWorldEndpoint(t *testing.T) {
	srv, _ := newTestServerAndClient(t, "")
	resp, err := http.Get(srv.URL + "/api/v1/world")
	if err != nil {
		t.Fatalf("GET /world: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var w api.World
	if err := json.NewDecoder(resp.Body).Decode(&w); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if w.ModVersion == "" {
		t.Error("mod_version should not be empty")
	}
}

// TestAuth covers the bearer-token gate: every endpoint except /health must
// reject a missing or wrong token with 401, and accept the right one.
func TestAuth(t *testing.T) {
	srv, _ := newTestServerAndClient(t, "s3cr3t")

	// /health is exempt from auth.
	resp, err := http.Get(srv.URL + "/api/v1/health")
	if err != nil {
		t.Fatalf("GET /health: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("/health status = %d, want 200 even without a token", resp.StatusCode)
	}

	// Everything else requires it.
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/v1/buildables", nil)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /buildables (no token): %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("no token: status = %d, want 401", resp.StatusCode)
	}

	req, _ = http.NewRequest(http.MethodGet, srv.URL+"/api/v1/buildables", nil)
	req.Header.Set("Authorization", "Bearer wrong")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /buildables (wrong token): %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("wrong token: status = %d, want 401", resp.StatusCode)
	}

	req, _ = http.NewRequest(http.MethodGet, srv.URL+"/api/v1/buildables", nil)
	req.Header.Set("Authorization", "Bearer s3cr3t")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /buildables (right token): %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("right token: status = %d, want 200", resp.StatusCode)
	}

	// The typed client goes through the same path and should work end to end.
	tokenedClient := client.New(srv.URL, "s3cr3t")
	if err := tokenedClient.Health(context.Background()); err != nil {
		t.Errorf("Health() with client token: %v", err)
	}
}

// TestFoundationPositionConflict pins the mod's co-location rule (mod issue
// #5): a lightweight-eligible buildable may not be placed where one of the
// same class already sits, because two instances at one position cannot be
// told apart after a save/reload - identity there is class + location - and
// they end up stranding each other. Manufacturers are not lightweight-eligible
// and are unaffected.
func TestFoundationPositionConflict(t *testing.T) {
	c := newTestClient(t)
	ctx := context.Background()

	at := api.Transform{X: 800, Y: 1600, Z: 20000}
	first := api.Buildable{TFID: "f-1", Class: "Build_Foundation_8x1_01_C", Transform: at}
	if _, err := c.CreateBuildable(ctx, first); err != nil {
		t.Fatalf("first foundation: %v", err)
	}

	dup := api.Buildable{TFID: "f-2", Class: "Build_Foundation_8x1_01_C", Transform: at}
	if _, err := c.CreateBuildable(ctx, dup); err == nil {
		t.Error("a second foundation at the same position should conflict")
	}

	// Within tolerance (<1cm) still counts as the same spot.
	near := dup
	near.TFID = "f-3"
	near.Transform.X += 0.5
	if _, err := c.CreateBuildable(ctx, near); err == nil {
		t.Error("a foundation within 1cm should conflict too")
	}

	// A clearly different position is fine.
	far := dup
	far.TFID = "f-4"
	far.Transform.X += 800
	if _, err := c.CreateBuildable(ctx, far); err != nil {
		t.Errorf("a foundation 8m away should be accepted: %v", err)
	}

	// A different class at the same spot is allowed - the rule is per class,
	// and machines are not lightweight-eligible at all.
	other := api.Buildable{TFID: "f-5", Class: "Build_SmelterMk1_C", Transform: at}
	if _, err := c.CreateBuildable(ctx, other); err != nil {
		t.Errorf("a manufacturer at the same spot should be accepted: %v", err)
	}
}

// TestGetBuildableClass covers the spawn-free class lookup that lets callers
// size a layout instead of guessing spacing (mod issue #6).
func TestGetBuildableClass(t *testing.T) {
	c := newTestClient(t)
	ctx := context.Background()

	got, err := c.GetBuildableClass(ctx, "Build_Foundation_8x1_01_C")
	if err != nil {
		t.Fatalf("known class: %v", err)
	}
	if got.Bounds == nil {
		t.Fatal("a foundation should report a footprint")
	}
	if got.Bounds.Size.X != 800 || got.Bounds.Size.Y != 800 {
		t.Errorf("8m foundation size = %vx%v, want 800x800", got.Bounds.Size.X, got.Bounds.Size.Y)
	}
	if got.Bounds.Size.X != got.Bounds.Max.X-got.Bounds.Min.X {
		t.Error("size must equal max-min; callers rely on not having to redo the arithmetic")
	}
	if len(got.Clearance) == 0 {
		t.Error("expected at least one clearance box alongside the union")
	}

	// A class the game knows but that reserves no space: bounds must be
	// absent, not a zero box. A zero box would read as "needs no room" and
	// silently stack buildables on one another.
	none, err := c.GetBuildableClass(ctx, "Build_SomethingWithNoClearance_C")
	if err != nil {
		t.Fatalf("clearance-free class: %v", err)
	}
	if none.Bounds != nil {
		t.Error("a class with no clearance must omit bounds rather than report zeros")
	}
	if none.Clearance == nil {
		t.Error("clearance should be an empty array, not null, so callers can range over it")
	}

	// Not a buildable class name at all.
	if _, err := c.GetBuildableClass(ctx, "Recipe_IronPlate_C"); !client.IsNotFound(err) {
		t.Errorf("non-buildable class should 404, got %v", err)
	}
}

// TestRecipeMustFitMachine pins the compatibility rule. Confirmed live: the
// game accepts a recipe its machine cannot produce (a smelter set to
// Recipe_IronPlate_C), reports it back on GET, and even displays it in the
// machine UI - so nothing surfaces the mistake. Terraform sees zero drift on
// a factory that can never produce anything, which is why this is rejected at
// the API rather than left to the game.
func TestRecipeMustFitMachine(t *testing.T) {
	c := newTestClient(t)
	ctx := context.Background()

	ok := api.Buildable{TFID: "m-ok", Class: "Build_SmelterMk1_C", Recipe: "Recipe_IngotIron_C"}
	if _, err := c.CreateBuildable(ctx, ok); err != nil {
		t.Fatalf("a smelter recipe on a smelter must be accepted: %v", err)
	}

	bad := api.Buildable{TFID: "m-bad", Class: "Build_SmelterMk1_C", Recipe: "Recipe_IronPlate_C"}
	if _, err := c.CreateBuildable(ctx, bad); err == nil {
		t.Error("a constructor recipe on a smelter must be refused at create")
	}

	// The same broken state was reachable through PATCH, so it is checked too.
	plate := "Recipe_IronPlate_C"
	if _, err := c.PatchBuildable(ctx, "m-ok", api.BuildablePatch{Recipe: &plate}); err == nil {
		t.Error("a constructor recipe on a smelter must be refused at patch")
	}
	// ...and the machine must be left as it was, not half-updated.
	after, err := c.GetBuildable(ctx, "m-ok")
	if err != nil {
		t.Fatalf("get after refused patch: %v", err)
	}
	if after.Recipe != "Recipe_IngotIron_C" {
		t.Errorf("refused patch changed the recipe to %q", after.Recipe)
	}

	// Fails open on recipes with no known producer: a wrong rejection breaks a
	// legitimate apply, a wrong acceptance only yields an idle machine.
	unknown := api.Buildable{TFID: "m-unknown", Class: "Build_SmelterMk1_C", Recipe: "Recipe_SomeModdedThing_C"}
	if _, err := c.CreateBuildable(ctx, unknown); err != nil {
		t.Errorf("an unrecognised recipe should be allowed through, got %v", err)
	}
}

// seededServer serves the sample hand-built world (foundations, two machines,
// a belt) plus one player.
func seededServer(t *testing.T) *client.Client {
	t.Helper()
	s := mockserver.New("")
	s.Seed(mockserver.SampleHandBuiltWorld())
	srv := httptest.NewServer(s.Handler())
	t.Cleanup(srv.Close)
	return client.New(srv.URL, "")
}

// The filter is required, not optional: a real save has tens of thousands of
// buildables, and a caller who forgets it should find out here rather than by
// hanging a live game.
func TestWorldBuildablesRequiresSpatialFilter(t *testing.T) {
	s := mockserver.New("")
	srv := httptest.NewServer(s.Handler())
	t.Cleanup(srv.Close)

	for _, q := range []string{
		"",
		"?x=0&y=0&z=0",
		"?x=0&y=0&z=0&radius=0",
		"?x=0&y=0&z=0&radius=100001",
		"?x=nope&y=0&z=0&radius=100",
	} {
		resp, err := http.Get(srv.URL + "/api/v1/world/buildables" + q)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusUnprocessableEntity {
			t.Errorf("GET /world/buildables%s = %d, want 422", q, resp.StatusCode)
		}
	}
}

func TestWorldBuildablesReportsUntrackedAndTracked(t *testing.T) {
	c := seededServer(t)
	ctx := context.Background()

	created, err := c.CreateBuildable(ctx, api.Buildable{
		TFID:      "tf-managed",
		Class:     "Build_ConstructorMk1_C",
		Transform: api.Transform{X: 100, Y: 100, Z: 20200},
		Recipe:    "Recipe_IronPlate_C",
	})
	if err != nil {
		t.Fatal(err)
	}

	items, err := c.ListWorldBuildables(ctx, 400, 400, 20200, 5000)
	if err != nil {
		t.Fatal(err)
	}

	var tracked, untracked int
	var sawManaged bool
	for _, it := range items {
		if it.TFID == "" {
			untracked++
			continue
		}
		tracked++
		if it.TFID == created.TFID {
			sawManaged = true
		}
	}
	if !sawManaged {
		t.Errorf("the buildable Terraform created is missing its tf_id; an exporter would duplicate it")
	}
	if tracked != 1 {
		t.Errorf("tracked = %d, want 1", tracked)
	}
	// 4 foundations + 2 machines + 1 belt from the sample world.
	if untracked != 7 {
		t.Errorf("untracked = %d, want 7 (hand-built things are the point of this endpoint)", untracked)
	}
}

// Foundations are lightweight instances rather than actors. They are reported
// here precisely because actor-based enumeration cannot see them.
func TestWorldBuildablesFlagsLightweight(t *testing.T) {
	c := seededServer(t)
	items, err := c.ListWorldBuildables(context.Background(), 400, 400, 20200, 5000)
	if err != nil {
		t.Fatal(err)
	}
	var foundations, lightweight int
	for _, it := range items {
		if it.Class == "Build_Foundation_8x4_01_C" {
			foundations++
			if it.Lightweight {
				lightweight++
			}
		}
		if it.Class == "Build_SmelterMk1_C" && it.Lightweight {
			t.Errorf("a smelter is an actor, not a lightweight instance")
		}
	}
	if foundations == 0 || foundations != lightweight {
		t.Errorf("got %d foundations, %d flagged lightweight; want them equal and non-zero", foundations, lightweight)
	}
}

func TestWorldBuildablesRespectsRadius(t *testing.T) {
	c := seededServer(t)
	ctx := context.Background()

	near, err := c.ListWorldBuildables(ctx, 0, 0, 20000, 100)
	if err != nil {
		t.Fatal(err)
	}
	far, err := c.ListWorldBuildables(ctx, 0, 0, 20000, 5000)
	if err != nil {
		t.Fatal(err)
	}
	if len(near) >= len(far) {
		t.Errorf("radius 100 returned %d and radius 5000 returned %d; the filter does nothing", len(near), len(far))
	}
	if len(near) != 1 {
		t.Errorf("only the foundation at the origin is within 100cm, got %d", len(near))
	}
}

// Index is how an untracked buildable is referred to at all, so it must match
// the entry's position in the response.
func TestWorldBuildablesIndexMatchesPosition(t *testing.T) {
	c := seededServer(t)
	items, err := c.ListWorldBuildables(context.Background(), 400, 400, 20200, 5000)
	if err != nil {
		t.Fatal(err)
	}
	for i, it := range items {
		if it.Index != int64(i) {
			t.Errorf("items[%d].Index = %d", i, it.Index)
		}
	}
}

func TestPlayers(t *testing.T) {
	c := seededServer(t)
	players, err := c.ListPlayers(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(players) != 1 || players[0].Name != "pioneer" {
		t.Fatalf("players = %+v", players)
	}
	if players[0].Location.X != 400 {
		t.Errorf("location = %+v", players[0].Location)
	}

	// An empty world must still answer with an array, not null: an exporter
	// that ranges over the result should get zero players, not a nil panic.
	empty := newTestClient(t)
	got, err := empty.ListPlayers(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("players = %+v, want empty", got)
	}
}

// A belt names the two buildables it joins by their index in the same
// response, so those indices have to be translated from whatever the mock
// stores into the positions the caller actually receives.
func TestWorldBuildablesReportsConnectionGraph(t *testing.T) {
	c := seededServer(t)
	items, err := c.ListWorldBuildables(context.Background(), 400, 400, 20200, 5000)
	if err != nil {
		t.Fatal(err)
	}

	var belt *api.WorldBuildable
	for i := range items {
		if items[i].Class == "Build_ConveyorBeltMk1_C" {
			belt = &items[i]
		}
	}
	if belt == nil {
		t.Fatal("belt missing from the world listing")
	}
	if belt.Connects == nil {
		t.Fatal("belt has no connection graph; an exporter cannot rebuild it")
	}
	from, to := belt.Connects.From, belt.Connects.To
	if from.Index < 0 || int(from.Index) >= len(items) || to.Index < 0 || int(to.Index) >= len(items) {
		t.Fatalf("connection indices %d/%d are outside the response of %d items", from.Index, to.Index, len(items))
	}
	if got := items[from.Index].Class; got != "Build_SmelterMk1_C" {
		t.Errorf("from index points at %s, want the smelter", got)
	}
	if got := items[to.Index].Class; got != "Build_ConstructorMk1_C" {
		t.Errorf("to index points at %s, want the constructor", got)
	}
	if from.Connector != 1 || to.Connector != 0 {
		t.Errorf("connectors = %d/%d, want 1/0", from.Connector, to.Connector)
	}
}

// Half a connection is worse than none: re-created, it would wire whatever
// happens to sit at that index.
func TestWorldBuildablesDropsConnectionsWithAnEndOutsideTheRadius(t *testing.T) {
	c := seededServer(t)
	// Centred on the belt, tight enough to exclude the constructor at y=1000.
	items, err := c.ListWorldBuildables(context.Background(), 200, 600, 20200, 300)
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range items {
		if it.Class == "Build_ConveyorBeltMk1_C" && it.Connects != nil {
			t.Errorf("belt kept a connection whose far end was filtered out: %+v", it.Connects)
		}
	}
}

// Belts Terraform created are buildables in the world too, and an exporter
// that could not see them would report a factory with no belts in it.
func TestWorldBuildablesIncludesTrackedConnections(t *testing.T) {
	c := seededServer(t)
	ctx := context.Background()

	for _, b := range []api.Buildable{
		{TFID: "tf-smelter", Class: "Build_SmelterMk1_C", Transform: api.Transform{X: 5000, Y: 0, Z: 20200}, Recipe: "Recipe_IngotIron_C"},
		{TFID: "tf-constructor", Class: "Build_ConstructorMk1_C", Transform: api.Transform{X: 5000, Y: 800, Z: 20200}, Recipe: "Recipe_IronPlate_C"},
	} {
		if _, err := c.CreateBuildable(ctx, b); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := c.CreateConnection(ctx, api.Connection{
		TFID:  "tf-belt",
		Class: "Build_ConveyorBeltMk1_C",
		From:  api.ConnectionEndpoint{BuildableTFID: "tf-smelter", Connector: 1},
		To:    api.ConnectionEndpoint{BuildableTFID: "tf-constructor", Connector: 0},
	}); err != nil {
		t.Fatal(err)
	}

	items, err := c.ListWorldBuildables(ctx, 5000, 400, 20200, 2000)
	if err != nil {
		t.Fatal(err)
	}
	var belt *api.WorldBuildable
	for i := range items {
		if items[i].TFID == "tf-belt" {
			belt = &items[i]
		}
	}
	if belt == nil {
		t.Fatal("a belt Terraform created is missing from the world listing")
	}
	if belt.Connects == nil {
		t.Fatal("tracked belt has no connection graph")
	}
	if items[belt.Connects.From.Index].TFID != "tf-smelter" || items[belt.Connects.To.Index].TFID != "tf-constructor" {
		t.Errorf("connection points at the wrong buildables: %+v", belt.Connects)
	}
}

// With nothing tracked, a seeded belt's stored indices happen to equal its
// response indices - so a test on the seeded world alone passes whether or not
// the translation actually runs. Creating buildables first shifts everything
// and makes the remap observable.
func TestWorldBuildablesRemapsConnectionIndicesWhenOffset(t *testing.T) {
	c := seededServer(t)
	ctx := context.Background()

	// Tracked buildables sort ahead of the seeded world, pushing every seeded
	// index along by two.
	for _, b := range []api.Buildable{
		{TFID: "aaa-first", Class: "Build_ConstructorMk1_C", Transform: api.Transform{X: 300, Y: 300, Z: 20200}, Recipe: "Recipe_IronPlate_C"},
		{TFID: "aab-second", Class: "Build_ConstructorMk1_C", Transform: api.Transform{X: 350, Y: 350, Z: 20200}, Recipe: "Recipe_IronPlate_C"},
	} {
		if _, err := c.CreateBuildable(ctx, b); err != nil {
			t.Fatal(err)
		}
	}

	items, err := c.ListWorldBuildables(ctx, 400, 400, 20200, 5000)
	if err != nil {
		t.Fatal(err)
	}
	var belt *api.WorldBuildable
	for i := range items {
		if items[i].Class == "Build_ConveyorBeltMk1_C" && items[i].TFID == "" {
			belt = &items[i]
		}
	}
	if belt == nil || belt.Connects == nil {
		t.Fatal("seeded belt missing or unresolved")
	}
	if got := items[belt.Connects.From.Index].Class; got != "Build_SmelterMk1_C" {
		t.Errorf("from index points at %s, want the smelter - stored indices were not translated", got)
	}
	if got := items[belt.Connects.To.Index].Class; got != "Build_ConstructorMk1_C" {
		t.Errorf("to index points at %s, want the constructor", got)
	}
	// And specifically not the raw stored values, which would now be wrong.
	if belt.Connects.From.Index == 4 && belt.Connects.To.Index == 5 {
		t.Errorf("indices are unchanged from the seed (%d/%d) despite two tracked buildables ahead of them",
			belt.Connects.From.Index, belt.Connects.To.Index)
	}
}
