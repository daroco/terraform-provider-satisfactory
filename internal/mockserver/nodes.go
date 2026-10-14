package mockserver

import (
	"encoding/json"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/daroco/terraform-provider-satisfactory/internal/api"
)

// Resource nodes, extractors and preview holograms.
//
// The real mod enumerates AFGResourceNodeBase and AFGWaterVolume actors and
// binds extractors to them through IFGExtractableResourceInterface. The mock
// keeps a fixed set of features so configs that select nodes ("nearest free
// iron node to the anchor") can be applied in CI, and reproduces the rules
// the mod enforces around them: an extractor must name a node, the node
// must exist and be free, and its resource form must suit the class.

// extractorRule says which features an extractor class family may bind to.
// Like manufacturerClassPrefixes this is a deliberate, narrow content table
// confined to the mock: the provider validates nothing itself, the real mod
// reads the equivalent (mAllowedResourceForms, mOnlyAllowCertainResources)
// off the class default object. A class family the table does not know is
// simply not an extractor here, which fails loud in CI rather than lying.
type extractorRule struct {
	prefix string
	// forms the class can extract (empty = any), and feature types it binds
	// to (empty = any). Both must pass.
	forms []string
	types []string
}

var extractorRules = []extractorRule{
	{prefix: "Build_MinerMk", forms: []string{"solid"}, types: []string{"node"}},
	{prefix: "Build_WaterPump", types: []string{"water"}},
	{prefix: "Build_OilPump", forms: []string{"liquid"}, types: []string{"node"}},
	{prefix: "Build_FrackingExtractor", types: []string{"fracking_satellite"}},
}

func extractorRuleFor(class string) (extractorRule, bool) {
	for _, r := range extractorRules {
		if strings.HasPrefix(class, r.prefix) {
			return r, true
		}
	}
	return extractorRule{}, false
}

func isExtractorClass(class string) bool {
	_, ok := extractorRuleFor(class)
	return ok
}

// isClockableClass mirrors AFGBuildableFactory::GetCanChangePotential: the
// buildables the mod reports clock_speed for. Manufacturers, extractors and
// generators can be over/underclocked; storage, splitters and foundations
// cannot, and the mod omits the field for them entirely.
func isClockableClass(class string) bool {
	return isManufacturerClass(class) || isExtractorClass(class) || strings.HasPrefix(class, "Build_Generator")
}

// canBecomeOccupied mirrors IFGExtractableResourceInterface::CanBecomeOccupied:
// a node takes exactly one extractor, a water volume is an area and takes
// any number, so it never reads as occupied.
func canBecomeOccupied(n api.ResourceNode) bool {
	return n.Type != "water"
}

func (r extractorRule) accepts(n api.ResourceNode) bool {
	okForm := len(r.forms) == 0
	for _, f := range r.forms {
		if f == n.Form {
			okForm = true
		}
	}
	okType := len(r.types) == 0
	for _, t := range r.types {
		if t == n.Type {
			okType = true
		}
	}
	return okForm && okType
}

// defaultResourceNodes is the mock world's geography: a handful of nodes and
// one lake around the Grasslands anchor the tier4-factory example uses, with
// every purity and one node already taken by a hand-built miner, so node
// selection in a config has something to filter and sort.
//
// Ids look like the real thing (an actor path) but are opaque to callers;
// the only promise is that they are stable.
func defaultResourceNodes() []api.ResourceNode {
	const ax, ay, az = -77737.0, 220338.0, -337.0
	id := func(name string) string {
		return "Persistent_Level:PersistentLevel." + name
	}
	node := func(name, resource, purity string, dx, dy, dz float64) api.ResourceNode {
		return api.ResourceNode{
			ID: id(name), Resource: resource, Purity: purity, Form: "solid", Type: "node",
			X: ax + dx, Y: ay + dy, Z: az + dz, Yaw: 0,
		}
	}
	nodes := []api.ResourceNode{
		node("BP_ResourceNode_Iron_1", "Desc_OreIron_C", "pure", -6000, -4000, 20),
		node("BP_ResourceNode_Iron_2", "Desc_OreIron_C", "normal", 5000, -6000, -40),
		node("BP_ResourceNode_Iron_3", "Desc_OreIron_C", "impure", 9000, 9000, 60),
		node("BP_ResourceNode_Iron_4", "Desc_OreIron_C", "normal", -3000, 7000, 0),
		node("BP_ResourceNode_Copper_1", "Desc_OreCopper_C", "normal", 8000, -2000, 10),
		node("BP_ResourceNode_Stone_1", "Desc_Stone_C", "pure", -9000, 3000, -20),
		node("BP_ResourceNode_Coal_1", "Desc_Coal_C", "normal", 12000, 4000, 30),
		node("BP_ResourceNode_Coal_2", "Desc_Coal_C", "normal", 13000, 6000, 30),
		node("BP_ResourceNode_Coal_3", "Desc_Coal_C", "pure", 15000, 9000, 50),
		node("BP_ResourceNode_Oil_1", "Desc_LiquidOil_C", "normal", -20000, -18000, -100),
	}
	nodes[9].Form = "liquid"
	// A node someone built a miner on by hand: occupied, but no tf_id.
	nodes[3].Occupied = true
	nodes = append(nodes,
		api.ResourceNode{
			ID: id("BP_ResourceNode_Geyser_1"), Resource: "Desc_Geyser_C", Form: "heat", Type: "geyser",
			X: ax - 15000, Y: ay - 15000, Z: az + 400,
		},
		api.ResourceNode{
			ID: id("Water_Lake_1"), Resource: "Desc_Water_C", Form: "liquid", Type: "water",
			X: ax + 2000, Y: ay + 14000, Z: az - 3,
			BoundsMin: &api.Vec3{X: ax - 2000, Y: ay + 10000, Z: az - 600},
			BoundsMax: &api.Vec3{X: ax + 6000, Y: ay + 18000, Z: az - 3},
		},
	)
	return nodes
}

// SeedResourceNodes replaces the mock's world features. Tests use it to make
// ordering and filtering assertions against a known set.
func (s *Server) SeedResourceNodes(nodes []api.ResourceNode) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nodes = map[string]*api.ResourceNode{}
	for i := range nodes {
		n := nodes[i]
		s.nodes[n.ID] = &n
	}
}

func (s *Server) listResourceNodes(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	var center *api.Vec3
	var radius *float64
	coords := map[string]float64{}
	for _, name := range []string{"x", "y", "z", "radius"} {
		raw := q.Get(name)
		if raw == "" {
			continue
		}
		v, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			writeErr(w, http.StatusUnprocessableEntity, name+" must be a number")
			return
		}
		coords[name] = v
	}
	_, hasX := coords["x"]
	_, hasY := coords["y"]
	_, hasZ := coords["z"]
	if hasX || hasY || hasZ {
		if !hasX || !hasY || !hasZ {
			writeErr(w, http.StatusUnprocessableEntity, "x, y and z go together")
			return
		}
		center = &api.Vec3{X: coords["x"], Y: coords["y"], Z: coords["z"]}
	}
	if v, ok := coords["radius"]; ok {
		if center == nil {
			writeErr(w, http.StatusUnprocessableEntity, "radius requires x, y and z")
			return
		}
		if v < 0 {
			writeErr(w, http.StatusUnprocessableEntity, "radius must be >= 0")
			return
		}
		radius = &v
	}
	purity := q.Get("purity")
	switch purity {
	case "", "impure", "normal", "pure":
	default:
		writeErr(w, http.StatusUnprocessableEntity, "purity must be one of impure, normal, pure")
		return
	}
	resource := q.Get("resource")
	includeOccupied := false
	if raw := q.Get("include_occupied"); raw != "" {
		v, err := strconv.ParseBool(raw)
		if err != nil {
			writeErr(w, http.StatusUnprocessableEntity, "include_occupied must be a boolean")
			return
		}
		includeOccupied = v
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	out := []api.ResourceNode{}
	for _, n := range s.nodes {
		if resource != "" && n.Resource != resource {
			continue
		}
		if purity != "" && n.Purity != purity {
			continue
		}
		if !includeOccupied && n.Occupied {
			continue
		}
		if radius != nil && distance(*center, *n) > *radius {
			continue
		}
		out = append(out, *n)
	}
	// Part of the contract, not a nicety: a config that takes "the nearest
	// free iron node" depends on this order being the same on every plan.
	sort.Slice(out, func(i, j int) bool {
		if center != nil {
			di, dj := distance(*center, out[i]), distance(*center, out[j])
			if di != dj {
				return di < dj
			}
		}
		return out[i].ID < out[j].ID
	})
	writeJSON(w, http.StatusOK, out)
}

func distance(c api.Vec3, n api.ResourceNode) float64 {
	dx, dy, dz := n.X-c.X, n.Y-c.Y, n.Z-c.Z
	return math.Sqrt(dx*dx + dy*dy + dz*dz)
}

// resolveExtractorPlacement applies the mod's node-binding rules to a
// buildable (or preview) request and returns the transform it lands at.
// It does not mutate anything; the caller decides whether to occupy the
// node. The returned status is 0 on success. Must hold s.mu.
func (s *Server) resolveExtractorPlacement(class, nodeID string, placement *api.Placement) (api.Transform, *api.ResourceNode, int, string) {
	rule, isExtractor := extractorRuleFor(class)
	if !isExtractor {
		if nodeID != "" {
			return api.Transform{}, nil, http.StatusUnprocessableEntity,
				class + " is placed at a transform, not on a node: remove node_id"
		}
		return api.Transform{}, nil, 0, ""
	}
	if nodeID == "" {
		return api.Transform{}, nil, http.StatusUnprocessableEntity,
			"extractor classes must be placed on a node: set node_id (see GET /world/resource-nodes)"
	}
	n, ok := s.nodes[nodeID]
	if !ok {
		return api.Transform{}, nil, http.StatusUnprocessableEntity, "unknown node_id " + nodeID
	}
	if !rule.accepts(*n) {
		return api.Transform{}, nil, http.StatusUnprocessableEntity,
			class + " cannot extract " + n.Resource + " (" + n.Form + " from a " + n.Type + ")"
	}
	t := api.Transform{X: n.X, Y: n.Y, Z: n.Z, Yaw: n.Yaw}
	if placement != nil {
		if n.Type == "water" {
			if placement.X != nil {
				t.X = *placement.X
			}
			if placement.Y != nil {
				t.Y = *placement.Y
			}
			if n.BoundsMin != nil && n.BoundsMax != nil &&
				(t.X < n.BoundsMin.X || t.X > n.BoundsMax.X || t.Y < n.BoundsMin.Y || t.Y > n.BoundsMax.Y) {
				return api.Transform{}, nil, http.StatusUnprocessableEntity,
					"placement is outside the water volume's bounds"
			}
		}
		if placement.Yaw != nil {
			t.Yaw = *placement.Yaw
		}
	}
	return t, n, 0, ""
}

// --- Preview holograms ------------------------------------------------------

type mockPreview struct {
	result  api.PreviewResult
	expires time.Time
}

func (s *Server) purgeExpiredPreviews() {
	now := s.now()
	for k, p := range s.previews {
		if !p.expires.After(now) {
			delete(s.previews, k)
		}
	}
}

func (s *Server) listPreviews(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.purgeExpiredPreviews()
	out := make([]api.PreviewResult, 0, len(s.previews))
	now := s.now()
	for _, p := range s.previews {
		res := p.result
		res.ExpiresIn = int64(math.Ceil(p.expires.Sub(now).Seconds()))
		out = append(out, res)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].TFID < out[j].TFID })
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) createPreview(w http.ResponseWriter, r *http.Request) {
	var p api.Preview
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if p.TFID == "" {
		writeErr(w, http.StatusUnprocessableEntity, "tf_id is required")
		return
	}
	if !validClass(p.Class, "Build_") {
		writeErr(w, http.StatusUnprocessableEntity, "class must be a buildable class name like Build_ConstructorMk1_C")
		return
	}
	ttl := p.TTLSeconds
	if ttl == 0 {
		ttl = 120
	}
	if ttl < 1 || ttl > 600 {
		writeErr(w, http.StatusUnprocessableEntity, "ttl_seconds must be between 1 and 600")
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.purgeExpiredPreviews()

	var t api.Transform
	valid := true
	disqualifiers := []string{}
	if isExtractorClass(p.Class) || p.NodeID != "" {
		placed, node, status, msg := s.resolveExtractorPlacement(p.Class, p.NodeID, p.Placement)
		if status != 0 {
			writeErr(w, status, msg)
			return
		}
		t = placed
		if node.Occupied {
			// The one node rule a hologram would flag rather than the API
			// refusing: disqualifier names here are illustrative of the
			// game's CD_* classes, not read from it.
			valid = false
			disqualifiers = append(disqualifiers, "CD_ResourceNodeOccupied_C")
		}
	} else {
		if p.Transform == nil {
			writeErr(w, http.StatusUnprocessableEntity, "transform is required")
			return
		}
		t = *p.Transform
		// Mirror the single placement rule the mock knows: the game's own
		// hologram would show this red, and POST /buildables would 409.
		if isLightweightClass(p.Class) {
			for _, existing := range s.buildables {
				if existing.Class == p.Class && sameSpot(existing.Transform, t) {
					valid = false
					disqualifiers = append(disqualifiers, "CD_Occupied_C")
					break
				}
			}
		}
	}

	res := api.PreviewResult{
		TFID: p.TFID, Class: p.Class, Valid: &valid, Disqualifiers: disqualifiers,
		X: t.X, Y: t.Y, Z: t.Z, Yaw: t.Yaw, ExpiresIn: ttl,
	}
	s.previews[p.TFID] = mockPreview{result: res, expires: s.now().Add(time.Duration(ttl) * time.Second)}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) deletePreviews(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.purgeExpiredPreviews()
	n := len(s.previews)
	s.previews = map[string]mockPreview{}
	writeJSON(w, http.StatusOK, map[string]int{"removed": n})
}

// getPreview re-reports one standing hologram. The mock's verdict never
// changes between reads; the real mod re-runs the placement check here,
// which is why the provider reads back after an "initializing" POST.
func (s *Server) getPreview(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.purgeExpiredPreviews()
	p, ok := s.previews[r.PathValue("tf_id")]
	if !ok {
		writeErr(w, http.StatusNotFound, "no preview with that tf_id (expired or never shown)")
		return
	}
	res := p.result
	res.ExpiresIn = int64(math.Ceil(p.expires.Sub(s.now()).Seconds()))
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) deletePreview(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.previews, r.PathValue("tf_id"))
	w.WriteHeader(http.StatusNoContent)
}

// PreviewCount reports how many holograms are standing. Tests use it to
// prove an apply cleared what its plan previewed.
func (s *Server) PreviewCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.purgeExpiredPreviews()
	return len(s.previews)
}
