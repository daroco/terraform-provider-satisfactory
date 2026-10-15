// Package api defines the wire types shared by the Terraform provider client
// and the mock server. The authoritative contract is api/openapi.yaml; the
// SatisfactoTerraform UE mod implements the same shapes in C++.
package api

// Transform is a world position in centimetres (Unreal units) plus yaw in degrees.
type Transform struct {
	X   float64 `json:"x"`
	Y   float64 `json:"y"`
	Z   float64 `json:"z"`
	Yaw float64 `json:"yaw"`
}

// Buildable is a machine, foundation or extractor placed in the world.
//
// Coordinate-placed classes carry Transform on input. Extractor classes
// (miners, water extractors, ...) carry NodeID instead - they are placed on a
// world feature, not at a point - and get their Transform back from the
// response. Placement optionally tunes where on the feature they go.
type Buildable struct {
	TFID       string     `json:"tf_id"`
	Class      string     `json:"class"`
	Transform  Transform  `json:"transform"`
	NodeID     string     `json:"node_id,omitempty"`
	Placement  *Placement `json:"placement,omitempty"`
	Recipe     string     `json:"recipe,omitempty"`
	ClockSpeed float64    `json:"clock_speed,omitempty"`
}

// Placement is where on a world feature an extractor goes. Pointers because
// "not set" (use the feature's own placement) differs from 0.
type Placement struct {
	X   *float64 `json:"x,omitempty"`
	Y   *float64 `json:"y,omitempty"`
	Yaw *float64 `json:"yaw,omitempty"`
}

// ResourceNode is one entry from GET /world/resource-nodes: a resource node,
// geyser or water volume an extractor can bind to.
type ResourceNode struct {
	// ID is the feature's stable actor path - what Buildable.NodeID takes.
	ID       string `json:"id"`
	Resource string `json:"resource"`
	// Purity is impure/normal/pure for resource nodes and "" for water.
	Purity string `json:"purity"`
	// Form is solid/liquid/gas/heat: what decides which extractor fits.
	Form string `json:"form"`
	// Type is node/geyser/water/fracking/fracking_satellite. Not closed.
	Type     string `json:"type"`
	Occupied bool   `json:"occupied"`
	// OccupantTFID is set when the extractor on this feature is one the mod
	// tracks, so a config can keep selecting the nodes it already owns.
	OccupantTFID string  `json:"occupant_tf_id,omitempty"`
	X            float64 `json:"x"`
	Y            float64 `json:"y"`
	Z            float64 `json:"z"`
	Yaw          float64 `json:"yaw"`
	// BoundsMin/Max are set for water volumes only: the box the volume
	// occupies, so a config can spread extractors across it.
	BoundsMin *Vec3 `json:"bounds_min,omitempty"`
	BoundsMax *Vec3 `json:"bounds_max,omitempty"`
}

// ResourceNodeQuery is the filter for GET /world/resource-nodes. Center and
// Radius are pointers because "unset" is meaningful: without a centre the
// result is sorted by id, and a radius needs a centre.
type ResourceNodeQuery struct {
	Resource        string
	Purity          string
	Center          *Vec3
	Radius          *float64
	IncludeOccupied bool
}

// Preview is the body of POST /preview: a would-be buildable to show as an
// in-game hologram, with the same placement fields as Buildable.
type Preview struct {
	TFID       string     `json:"tf_id"`
	Class      string     `json:"class"`
	Transform  *Transform `json:"transform,omitempty"`
	NodeID     string     `json:"node_id,omitempty"`
	Placement  *Placement `json:"placement,omitempty"`
	TTLSeconds int64      `json:"ttl_seconds,omitempty"`
}

// PreviewResult is what the game's hologram said about a Preview. Valid is a
// pointer because nil means the mod could not run the validation at all,
// which is distinct from invalid.
type PreviewResult struct {
	TFID          string   `json:"tf_id"`
	Class         string   `json:"class"`
	Valid         *bool    `json:"valid"`
	Disqualifiers []string `json:"disqualifiers"`
	X             float64  `json:"x"`
	Y             float64  `json:"y"`
	Z             float64  `json:"z"`
	Yaw           float64  `json:"yaw"`
	ExpiresIn     int64    `json:"expires_in,omitempty"`
}

// BuildablePatch carries the in-place-mutable fields of a Buildable.
type BuildablePatch struct {
	Recipe     *string  `json:"recipe,omitempty"`
	ClockSpeed *float64 `json:"clock_speed,omitempty"`
}

// ConnectionEndpoint identifies one end of a belt or power line by the
// buildable it attaches to and the index of the connection component on it.
type ConnectionEndpoint struct {
	BuildableTFID string `json:"buildable_tf_id"`
	Connector     int64  `json:"connector"`
}

// Connection is a belt or power line between two buildables.
type Connection struct {
	TFID  string             `json:"tf_id"`
	Class string             `json:"class"`
	From  ConnectionEndpoint `json:"from"`
	To    ConnectionEndpoint `json:"to"`
}

// Error is the body of every non-2xx response.
type Error struct {
	Message string `json:"message"`
}

// World is the response of GET /world.
type World struct {
	SessionName string `json:"session_name"`
	GameVersion string `json:"game_version"`
	ModVersion  string `json:"mod_version"`
}

// Vec3 is a point or extent in Unreal centimetres.
type Vec3 struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	Z float64 `json:"z"`
}

// ClearanceBox is one clearance volume declared by a buildable class,
// already transformed by its relative transform and expressed relative to
// the buildable's origin.
type ClearanceBox struct {
	// Type is "default", "soft", or "block_everything". Vanilla placement
	// refuses default-vs-default overlaps, warns on soft, and refuses
	// everything for block_everything. The mod does not enforce any of it -
	// placing through the API deliberately bypasses hologram rules - so this
	// is advisory data for callers that want to reason about space.
	Type string `json:"type"`
	Min  Vec3   `json:"min"`
	Max  Vec3   `json:"max"`
}

// Bounds is the union of a class's clearance boxes: its whole footprint.
type Bounds struct {
	Min  Vec3 `json:"min"`
	Max  Vec3 `json:"max"`
	Size Vec3 `json:"size"`
}

// BuildableClass is the response of GET /classes/{class}: static facts read
// from the class default object, so asking costs nothing and places nothing.
type BuildableClass struct {
	Class     string         `json:"class"`
	Clearance []ClearanceBox `json:"clearance"`
	// Bounds is nil when the class declares no clearance, which is common.
	Bounds *Bounds `json:"bounds,omitempty"`
}

// WorldBuildable is one entry from GET /world/buildables: the world as it
// actually is, not as Terraform believes it to be. TFID is empty for anything
// built in-game, which is the whole point of the endpoint.
type WorldBuildable struct {
	// Index is the only handle an untracked buildable has. It is valid within
	// one response and nowhere else.
	Index     int64     `json:"index"`
	TFID      string    `json:"tf_id,omitempty"`
	Class     string    `json:"class"`
	Transform Transform `json:"transform"`
	// NodeID is set for extractors: the feature they are bound to.
	NodeID      string  `json:"node_id,omitempty"`
	Lightweight bool    `json:"lightweight"`
	Recipe      string  `json:"recipe,omitempty"`
	ClockSpeed  float64 `json:"clock_speed,omitempty"`
	// Connects is set for belts and wires whose two ends are both inside the
	// queried radius. A partially known connection is omitted rather than
	// guessed: a wrong connector index yields config that applies cleanly and
	// wires the wrong port.
	Connects *WorldConnection `json:"connects,omitempty"`
}

// WorldConnection is the pair of buildables a belt or wire joins, by their
// index within the same response.
type WorldConnection struct {
	From WorldEndpoint `json:"from"`
	To   WorldEndpoint `json:"to"`
}

// WorldEndpoint identifies one end of a connection.
type WorldEndpoint struct {
	Index int64 `json:"index"`
	// Connector uses the same ordering POST /connections accepts, so an
	// exported connection can be re-created against the same port.
	Connector int64 `json:"connector"`
}

// Player is one entry from GET /players.
type Player struct {
	Name     string  `json:"name,omitempty"`
	Location Vec3    `json:"location"`
	Yaw      float64 `json:"yaw"`
}

// ClassInfo is one entry from GET /classes: a buildable class the game ships,
// classified by the mechanism the provider needs in order to place it.
type ClassInfo struct {
	Class       string `json:"class"`
	DisplayName string `json:"display_name"`
	// Mechanism groups classes by what placing them requires. Supported ones
	// name a resource; the rest name an engineering gap, and it is the gaps
	// that the coverage report is for.
	Mechanism string `json:"mechanism"`
	Supported bool   `json:"supported"`
	Resource  string `json:"resource,omitempty"`
	// WhyUnsupported explains the mechanism for classes with no Resource.
	WhyUnsupported string `json:"why_unsupported,omitempty"`
	// SettingsNotModelled names state the contract cannot express for a class
	// it can otherwise place - Terraform can put it down but not configure it.
	SettingsNotModelled string `json:"settings_not_modelled,omitempty"`
}

// SessionSave is the body of POST /session/save and the save part of
// POST /session/quit. An empty Name means "<session name>_terraform".
type SessionSave struct {
	Name string `json:"name,omitempty"`
}

// SessionSaveResult answers a save or quit: the save name written, empty
// when a quit skipped saving.
type SessionSaveResult struct {
	Saved string `json:"saved"`
}

// SessionQuit is the body of POST /session/quit. Save defaults to true.
type SessionQuit struct {
	Save *bool  `json:"save,omitempty"`
	Name string `json:"name,omitempty"`
}

// InventoryGrant is the body of POST /players/{name}/inventory: put Count of
// the item descriptor class Item (Desc_Coal_C) into a player's inventory.
type InventoryGrant struct {
	Item  string `json:"item"`
	Count int    `json:"count,omitempty"`
}

// InventoryGrantResult reports how many items actually went in.
type InventoryGrantResult struct {
	Player    string `json:"player"`
	Item      string `json:"item"`
	Requested int    `json:"requested"`
	Added     int    `json:"added"`
}
