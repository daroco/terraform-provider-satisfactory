// Package mockserver is an in-memory implementation of the SatisfactoTerraform
// mod API (api/openapi.yaml). It exists so the provider can be developed and
// acceptance-tested without a running game, and doubles as executable
// documentation of the contract the UE mod must implement.
package mockserver

import (
	"encoding/json"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/daroco/terraform-provider-satisfactory/internal/api"
)

// Server holds the in-memory world state behind the mock API.
type Server struct {
	mu          sync.Mutex
	buildables  map[string]api.Buildable
	connections map[string]api.Connection
	token       string

	// Buildables the "player" made by hand: they have no tf_id and are
	// invisible to /buildables, but GET /world/buildables must report them.
	// Seeded by tests and by cmd/mockserver so export tooling has something
	// untracked to find - without them the mock can only ever produce a world
	// Terraform already owns, which is the uninteresting half of export.
	untracked []api.WorldBuildable
	players   []api.Player
}

// New returns a mock server. token is optional; when set, every request except
// /health must carry it as a bearer token.
func New(token string) *Server {
	return &Server{
		buildables:  map[string]api.Buildable{},
		connections: map[string]api.Connection{},
		token:       token,
	}
}

// Handler returns the http.Handler serving the API under /api/v1.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("GET /api/v1/world", s.auth(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, api.World{
			SessionName: "mock",
			GameVersion: "mock",
			ModVersion:  "0.1.0",
		})
	}))

	mux.HandleFunc("GET /api/v1/classes/{class}", s.auth(s.getBuildableClass))
	mux.HandleFunc("GET /api/v1/world/buildables", s.auth(s.listWorldBuildables))
	mux.HandleFunc("GET /api/v1/players", s.auth(s.listPlayers))

	mux.HandleFunc("GET /api/v1/buildables", s.auth(s.listBuildables))
	mux.HandleFunc("POST /api/v1/buildables", s.auth(s.createBuildable))
	mux.HandleFunc("GET /api/v1/buildables/{tf_id}", s.auth(s.getBuildable))
	mux.HandleFunc("PATCH /api/v1/buildables/{tf_id}", s.auth(s.patchBuildable))
	mux.HandleFunc("DELETE /api/v1/buildables/{tf_id}", s.auth(s.deleteBuildable))

	mux.HandleFunc("GET /api/v1/connections", s.auth(s.listConnections))
	mux.HandleFunc("POST /api/v1/connections", s.auth(s.createConnection))
	mux.HandleFunc("GET /api/v1/connections/{tf_id}", s.auth(s.getConnection))
	mux.HandleFunc("DELETE /api/v1/connections/{tf_id}", s.auth(s.deleteConnection))
	return mux
}

func (s *Server) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.token != "" && r.Header.Get("Authorization") != "Bearer "+s.token {
			writeErr(w, http.StatusUnauthorized, "missing or invalid bearer token")
			return
		}
		next(w, r)
	}
}

func (s *Server) listBuildables(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]api.Buildable, 0, len(s.buildables))
	for _, b := range s.buildables {
		out = append(out, b)
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) createBuildable(w http.ResponseWriter, r *http.Request) {
	var b api.Buildable
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if b.TFID == "" {
		writeErr(w, http.StatusUnprocessableEntity, "tf_id is required")
		return
	}
	if !validClass(b.Class, "Build_") {
		writeErr(w, http.StatusUnprocessableEntity, "class must be a buildable class name like Build_ConstructorMk1_C")
		return
	}
	if b.Recipe != "" && !validClass(b.Recipe, "Recipe_") {
		writeErr(w, http.StatusUnprocessableEntity, "recipe must be a recipe class name like Recipe_IronPlate_C")
		return
	}
	if b.Recipe != "" && !recipeFitsClass(b.Recipe, b.Class) {
		writeErr(w, http.StatusUnprocessableEntity, b.Recipe+" cannot be produced in "+b.Class)
		return
	}
	if isManufacturerClass(b.Class) {
		if b.ClockSpeed == 0 {
			b.ClockSpeed = 1.0
		}
		if b.ClockSpeed < 0.01 || b.ClockSpeed > 2.5 {
			writeErr(w, http.StatusUnprocessableEntity, "clock_speed must be between 0.01 and 2.5")
			return
		}
	} else {
		// The real mod only reports recipe/clock_speed for buildables that
		// are manufacturers (AFGBuildableManufacturer); for everything else
		// (foundations, splitters, mergers, power poles, ...) any requested
		// values are silently ignored at spawn and the fields are omitted
		// from every response. Zeroing them here + `omitempty` on the wire
		// types reproduces that shape exactly - so a config that sets
		// recipe/clock_speed on a non-manufacturer class fails CI the same
		// way it would fail live (provider sees a response that doesn't
		// echo the plan), instead of passing against a too-lenient mock.
		b.Recipe = ""
		b.ClockSpeed = 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.buildables[b.TFID]; exists {
		writeErr(w, http.StatusConflict, "buildable with tf_id "+b.TFID+" already exists")
		return
	}
	// Mirrors the real mod: a lightweight-eligible buildable (foundations and
	// other passive structures) may not be placed where one of the same class
	// already sits. Co-located instances are indistinguishable after a
	// save/reload, since identity there is class + location, so the mod
	// refuses rather than creating a pair that would strand each other. Only
	// non-manufacturers are eligible, which is why this is keyed off the same
	// predicate the recipe/clock rules use.
	if !isManufacturerClass(b.Class) {
		for _, existing := range s.buildables {
			if existing.Class == b.Class && sameSpot(existing.Transform, b.Transform) {
				writeErr(w, http.StatusConflict,
					"a buildable of that class already exists at that position")
				return
			}
		}
	}
	s.buildables[b.TFID] = b
	writeJSON(w, http.StatusCreated, b)
}

func (s *Server) getBuildable(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, ok := s.buildables[r.PathValue("tf_id")]
	if !ok {
		writeErr(w, http.StatusNotFound, "no buildable with that tf_id")
		return
	}
	writeJSON(w, http.StatusOK, b)
}

func (s *Server) patchBuildable(w http.ResponseWriter, r *http.Request) {
	var p api.BuildablePatch
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	b, ok := s.buildables[r.PathValue("tf_id")]
	if !ok {
		writeErr(w, http.StatusNotFound, "no buildable with that tf_id")
		return
	}
	if !isManufacturerClass(b.Class) {
		// Mirrors the real mod's PatchBuildable: only manufacturers have a
		// recipe/clock to patch; same message, same 422.
		writeErr(w, http.StatusUnprocessableEntity, "this buildable has no recipe/clock_speed to patch")
		return
	}
	if p.Recipe != nil {
		if *p.Recipe != "" && !validClass(*p.Recipe, "Recipe_") {
			writeErr(w, http.StatusUnprocessableEntity, "recipe must be a recipe class name like Recipe_IronPlate_C")
			return
		}
		if *p.Recipe != "" && !recipeFitsClass(*p.Recipe, b.Class) {
			writeErr(w, http.StatusUnprocessableEntity, *p.Recipe+" cannot be produced in "+b.Class)
			return
		}
		b.Recipe = *p.Recipe
	}
	if p.ClockSpeed != nil {
		if *p.ClockSpeed < 0.01 || *p.ClockSpeed > 2.5 {
			writeErr(w, http.StatusUnprocessableEntity, "clock_speed must be between 0.01 and 2.5")
			return
		}
		b.ClockSpeed = *p.ClockSpeed
	}
	s.buildables[b.TFID] = b
	writeJSON(w, http.StatusOK, b)
}

func (s *Server) deleteBuildable(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("tf_id")
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.buildables[id]; !ok {
		writeErr(w, http.StatusNotFound, "no buildable with that tf_id")
		return
	}
	delete(s.buildables, id)
	// Matches the real mod: dismantling a connected buildable is allowed
	// (the belt/wire just dangles), it's not blocked with a 409 - but any
	// connection that referenced this id would otherwise keep resolving to
	// a dead tf_id on GET forever, so prune those the same way the mod's
	// Unregister does.
	for tfid, c := range s.connections {
		if c.From.BuildableTFID == id || c.To.BuildableTFID == id {
			delete(s.connections, tfid)
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) listConnections(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]api.Connection, 0, len(s.connections))
	for _, c := range s.connections {
		out = append(out, c)
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) createConnection(w http.ResponseWriter, r *http.Request) {
	var c api.Connection
	if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if c.TFID == "" {
		writeErr(w, http.StatusUnprocessableEntity, "tf_id is required")
		return
	}
	if !validClass(c.Class, "Build_") {
		writeErr(w, http.StatusUnprocessableEntity, "class must be a class name like Build_ConveyorBeltMk1_C or Build_PowerLine_C")
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.connections[c.TFID]; exists {
		writeErr(w, http.StatusConflict, "connection with tf_id "+c.TFID+" already exists")
		return
	}
	for _, end := range []api.ConnectionEndpoint{c.From, c.To} {
		if _, ok := s.buildables[end.BuildableTFID]; !ok {
			writeErr(w, http.StatusUnprocessableEntity, "endpoint references unknown buildable "+end.BuildableTFID)
			return
		}
		if end.Connector < 0 {
			writeErr(w, http.StatusUnprocessableEntity, "connector index must be >= 0")
			return
		}
	}
	s.connections[c.TFID] = c
	writeJSON(w, http.StatusCreated, c)
}

func (s *Server) getConnection(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.connections[r.PathValue("tf_id")]
	if !ok {
		writeErr(w, http.StatusNotFound, "no connection with that tf_id")
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func (s *Server) deleteConnection(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.connections[r.PathValue("tf_id")]; !ok {
		writeErr(w, http.StatusNotFound, "no connection with that tf_id")
		return
	}
	delete(s.connections, r.PathValue("tf_id"))
	w.WriteHeader(http.StatusNoContent)
}

// validClass approximates the game's class naming: prefix + name + "_C".
// sameSpot reports whether two transforms are the same position within the
// tolerance the mod uses (1cm), which absorbs float32 save round-trip noise.
func sameSpot(a, b api.Transform) bool {
	const tolCm = 1.0
	return math.Abs(a.X-b.X) < tolCm && math.Abs(a.Y-b.Y) < tolCm && math.Abs(a.Z-b.Z) < tolCm
}

func validClass(class, prefix string) bool {
	return strings.HasPrefix(class, prefix) && strings.HasSuffix(class, "_C") && len(class) > len(prefix)+2
}

// manufacturerClassPrefixes lists the vanilla AFGBuildableManufacturer
// families - the buildables that actually have a recipe/clock_speed in the
// real mod's responses.
//
// This is a deliberate, narrow exception to the project rule against baking
// game-content tables into provider-side code (CLAUDE.md "Conventions"): the
// rule exists so new game content works without a provider release, and that
// still holds - the provider itself validates nothing, and an unknown class
// is still passed straight through to the game. This list only shapes the
// *mock's* responses so CI reproduces the real mod's
// manufacturer-vs-not behavior (fields omitted, PATCH 422) instead of
// masking that class of bug; a new manufacturer the list doesn't know about
// merely makes the mock strict where the game would be lenient, which fails
// loud in CI rather than silently diverging live.
var manufacturerClassPrefixes = []string{
	"Build_SmelterMk",
	"Build_FoundryMk",
	"Build_ConstructorMk",
	"Build_AssemblerMk",
	"Build_ManufacturerMk",
	"Build_OilRefinery",
	"Build_Packager",
	"Build_Blender",
	"Build_HadronCollider",
	"Build_Converter",
	"Build_QuantumEncoder",
}

func isManufacturerClass(class string) bool {
	for _, p := range manufacturerClassPrefixes {
		if strings.HasPrefix(class, p) {
			return true
		}
	}
	return false
}

// lightweightClassPrefixes are the structural buildables the game converts to
// non-actor lightweight instances. The mock tracks this only so that
// /world/buildables reports the same "lightweight" flag the mod does; nothing
// else in the mock behaves differently for them.
var lightweightClassPrefixes = []string{
	"Build_Foundation", "Build_Wall", "Build_Ramp", "Build_Roof",
	"Build_QuarterPipe", "Build_Fence", "Build_Barrier", "Build_Pillar",
	"Build_Stair", "Build_Catwalk", "Build_Beam", "Build_Wall_Conveyor",
}

func isLightweightClass(class string) bool {
	for _, p := range lightweightClassPrefixes {
		if strings.HasPrefix(class, p) {
			return true
		}
	}
	return false
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, api.Error{Message: msg})
}

// mockClearance is a stand-in for what the real mod reads off a class default
// object. The mock has no game content, so a few well-known classes carry
// hand-measured footprints and everything else reports none - which is also
// the honest answer for most buildables, since declaring clearance is the
// exception rather than the rule.
//
// This is the same narrow, deliberate exception to "no game content tables in
// provider code" that the manufacturer-class list is: it exists so CI can
// exercise the endpoint's shape, not so the provider can reason about content.
// Values are half-extents in centimetres about the buildable's origin.
var mockClearance = map[string]api.Bounds{
	// Measured from a live game via GET /classes/{class}, not guessed. The
	// first version of this table WAS guessed and two of four entries were
	// badly wrong (a constructor's Y by ~6m), which is precisely the kind of
	// lie that sends a grid layout astray - the thing this endpoint exists to
	// prevent. Note the z origins differ by family: foundations are centred,
	// machines sit on their base.
	"Build_Foundation_8x1_01_C":          {Min: api.Vec3{X: -400, Y: -400, Z: -50}, Max: api.Vec3{X: 400, Y: 400, Z: 50}},
	"Build_Foundation_8x4_01_C":          {Min: api.Vec3{X: -400, Y: -400, Z: -200}, Max: api.Vec3{X: 400, Y: 400, Z: 200}},
	"Build_ConstructorMk1_C":             {Min: api.Vec3{X: -400, Y: -500, Z: 0}, Max: api.Vec3{X: 400, Y: 500, Z: 850}},
	"Build_SmelterMk1_C":                 {Min: api.Vec3{X: -255, Y: -500, Z: 0}, Max: api.Vec3{X: 255, Y: 500, Z: 850}},
	"Build_ManufacturerMk1_C":            {Min: api.Vec3{X: -900, Y: -1000, Z: 0}, Max: api.Vec3{X: 900, Y: 1000, Z: 1459.1}},
	"Build_ConveyorAttachmentSplitter_C": {Min: api.Vec3{X: -200, Y: -200, Z: -80}, Max: api.Vec3{X: 200, Y: 200, Z: 180}},
	// Belts genuinely declare no clearance - deliberately absent so the
	// "bounds omitted" path is exercised by a real class, not a fake one.
}

// recipeProducers mirrors the game's mProducedIn: which machine each recipe
// can actually be made in. The real mod reads this from the recipe class;
// the mock needs enough of it to reproduce the 422, because the game accepts
// an impossible pairing silently (a smelter will happily display a
// constructor recipe and simply never produce), so nothing else catches it.
var recipeProducers = map[string]string{
	"Recipe_IngotIron_C":   "Build_SmelterMk1_C",
	"Recipe_IngotCopper_C": "Build_SmelterMk1_C",
	"Recipe_IronPlate_C":   "Build_ConstructorMk1_C",
	"Recipe_IronRod_C":     "Build_ConstructorMk1_C",
	"Recipe_Wire_C":        "Build_ConstructorMk1_C",
}

// recipeFitsClass fails OPEN for recipes it has never heard of, matching the
// mod: a wrong rejection breaks a legitimate apply, while a wrong acceptance
// only yields a machine that does not run.
func recipeFitsClass(recipe, class string) bool {
	producer, known := recipeProducers[recipe]
	if !known {
		return true
	}
	return producer == class
}

func (s *Server) getBuildableClass(w http.ResponseWriter, r *http.Request) {
	class := r.PathValue("class")
	if !validClass(class, "Build_") {
		writeErr(w, http.StatusNotFound, "no buildable class named "+class)
		return
	}
	out := api.BuildableClass{Class: class, Clearance: []api.ClearanceBox{}}
	if b, ok := mockClearance[class]; ok {
		bounds := b
		bounds.Size = api.Vec3{X: b.Max.X - b.Min.X, Y: b.Max.Y - b.Min.Y, Z: b.Max.Z - b.Min.Z}
		out.Bounds = &bounds
		out.Clearance = []api.ClearanceBox{{Type: "default", Min: b.Min, Max: b.Max}}
	}
	writeJSON(w, http.StatusOK, out)
}

// Seed adds buildables and players that the mod would report as existing in
// the world but that Terraform never created. Call before serving.
func (s *Server) Seed(untracked []api.WorldBuildable, players []api.Player) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.untracked = append(s.untracked, untracked...)
	s.players = append(s.players, players...)
}

// listWorldBuildables reports everything within a sphere, tracked or not.
// The spatial filter is required, matching the mod: a real save has far too
// many buildables to return unfiltered, and a caller that forgets the filter
// should find that out against the mock rather than against a live game.
func (s *Server) listWorldBuildables(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	var center api.Vec3
	var radius float64
	for _, p := range []struct {
		name string
		out  *float64
	}{{"x", &center.X}, {"y", &center.Y}, {"z", &center.Z}, {"radius", &radius}} {
		raw := q.Get(p.name)
		if raw == "" {
			writeErr(w, http.StatusUnprocessableEntity, "x, y, z and radius query parameters are all required")
			return
		}
		v, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			writeErr(w, http.StatusUnprocessableEntity, p.name+" must be a number")
			return
		}
		*p.out = v
	}
	if radius <= 0 || radius > 100000 {
		writeErr(w, http.StatusUnprocessableEntity, "radius must be between 0 and 100000 centimetres")
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	within := func(t api.Transform) bool {
		dx, dy, dz := t.X-center.X, t.Y-center.Y, t.Z-center.Z
		return dx*dx+dy*dy+dz*dz <= radius*radius
	}

	// Sorted for a stable response: generated HCL is diffed and committed, so
	// a stable order is the difference between a readable diff and noise.
	tracked := make([]api.Buildable, 0, len(s.buildables))
	for _, b := range s.buildables {
		if within(b.Transform) {
			tracked = append(tracked, b)
		}
	}
	sort.Slice(tracked, func(i, j int) bool { return tracked[i].TFID < tracked[j].TFID })

	// Response indices are assigned as entries are appended, and connections
	// refer to them - so endpoints are resolved only after everything in range
	// is collected.
	out := []api.WorldBuildable{}
	tfidToIndex := map[string]int64{}
	for _, b := range tracked {
		tfidToIndex[b.TFID] = int64(len(out))
		out = append(out, api.WorldBuildable{
			TFID:        b.TFID,
			Class:       b.Class,
			Transform:   b.Transform,
			Lightweight: isLightweightClass(b.Class),
			Recipe:      b.Recipe,
			ClockSpeed:  b.ClockSpeed,
		})
	}
	seedToIndex := map[int]int64{}
	for i, b := range s.untracked {
		if !within(b.Transform) {
			continue
		}
		seedToIndex[i] = int64(len(out))
		b.TFID = "" // untracked by definition; ignore anything seeded there
		out = append(out, b)
	}

	// A seeded belt names its ends by their position in the seed; translate to
	// response indices, and drop the connection outright if an end fell
	// outside the radius. Half a connection is worse than none: it would be
	// re-created against whatever happened to sit at that index.
	for i := range out {
		if out[i].Connects == nil {
			continue
		}
		from, fromOK := seedToIndex[int(out[i].Connects.From.Index)]
		to, toOK := seedToIndex[int(out[i].Connects.To.Index)]
		if !fromOK || !toOK {
			out[i].Connects = nil
			continue
		}
		out[i].Connects = &api.WorldConnection{
			From: api.WorldEndpoint{Index: from, Connector: out[i].Connects.From.Connector},
			To:   api.WorldEndpoint{Index: to, Connector: out[i].Connects.To.Connector},
		}
	}

	// Belts and wires Terraform created are buildables in the world too. The
	// mock stores them without a position (the contract does not give
	// connections one), so they are reported at the midpoint of the two
	// buildables they join - enough for a spatial query to behave sensibly.
	for _, conn := range s.connections {
		fromIdx, okFrom := tfidToIndex[conn.From.BuildableTFID]
		toIdx, okTo := tfidToIndex[conn.To.BuildableTFID]
		if !okFrom || !okTo {
			continue
		}
		a, b := out[fromIdx].Transform, out[toIdx].Transform
		mid := api.Transform{X: (a.X + b.X) / 2, Y: (a.Y + b.Y) / 2, Z: (a.Z + b.Z) / 2}
		if !within(mid) {
			continue
		}
		out = append(out, api.WorldBuildable{
			TFID:      conn.TFID,
			Class:     conn.Class,
			Transform: mid,
			Connects: &api.WorldConnection{
				From: api.WorldEndpoint{Index: fromIdx, Connector: conn.From.Connector},
				To:   api.WorldEndpoint{Index: toIdx, Connector: conn.To.Connector},
			},
		})
	}

	for i := range out {
		out[i].Index = int64(i)
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) listPlayers(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.players == nil {
		writeJSON(w, http.StatusOK, []api.Player{})
		return
	}
	writeJSON(w, http.StatusOK, s.players)
}

// SampleHandBuiltWorld is a small factory that nothing in Terraform created:
// a 2x2 floor, a smelter feeding a constructor, and the belt between them.
// It gives `satisfactory-export` something to export against the mock, and
// gives CI a fixed world to assert generated configuration against.
func SampleHandBuiltWorld() ([]api.WorldBuildable, []api.Player) {
	const tile = 800 // Build_Foundation_8x4_01_C
	world := []api.WorldBuildable{}
	for _, p := range [][2]float64{{0, 0}, {tile, 0}, {0, tile}, {tile, tile}} {
		world = append(world, api.WorldBuildable{
			Class:       "Build_Foundation_8x4_01_C",
			Transform:   api.Transform{X: p[0], Y: p[1], Z: 20000},
			Lightweight: true,
		})
	}
	world = append(world,
		api.WorldBuildable{
			Class:      "Build_SmelterMk1_C",
			Transform:  api.Transform{X: 200, Y: 200, Z: 20200, Yaw: 90},
			Recipe:     "Recipe_IngotIron_C",
			ClockSpeed: 1,
		},
		api.WorldBuildable{
			Class:      "Build_ConstructorMk1_C",
			Transform:  api.Transform{X: 200, Y: 1000, Z: 20200, Yaw: 90},
			Recipe:     "Recipe_IronPlate_C",
			ClockSpeed: 1.5,
		},
		// The belt joins them, output connector of the smelter to input
		// connector of the constructor. Indices here are positions in this
		// slice; listWorldBuildables translates them to response indices.
		api.WorldBuildable{
			Class:     "Build_ConveyorBeltMk1_C",
			Transform: api.Transform{X: 200, Y: 600, Z: 20200},
			Connects: &api.WorldConnection{
				From: api.WorldEndpoint{Index: 4, Connector: 1},
				To:   api.WorldEndpoint{Index: 5, Connector: 0},
			},
		},
	)

	players := []api.Player{{
		Name:     "pioneer",
		Location: api.Vec3{X: 400, Y: 400, Z: 20200},
		Yaw:      45,
	}}
	return world, players
}
