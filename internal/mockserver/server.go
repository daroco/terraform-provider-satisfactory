// Package mockserver is an in-memory implementation of the SatisfactoTerraform
// mod API (api/openapi.yaml). It exists so the provider can be developed and
// acceptance-tested without a running game, and doubles as executable
// documentation of the contract the UE mod must implement.
package mockserver

import (
	"encoding/json"
	"math"
	"net/http"
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
