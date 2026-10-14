// Package client is the HTTP client for the SatisfactoTerraform mod API,
// used by the Terraform provider.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/daroco/terraform-provider-satisfactory/internal/api"
)

// NotFoundError is returned when the world has no object with the given tf_id
// (e.g. the player dismantled it). The provider maps this to state removal.
type NotFoundError struct{ TFID string }

func (e *NotFoundError) Error() string { return "not found: " + e.TFID }

// APIError is any other non-2xx response.
type APIError struct {
	Status  int
	Message string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("mod API returned %d: %s", e.Status, e.Message)
}

// Client talks to one running game session.
type Client struct {
	baseURL string
	token   string
	http    *http.Client
	// preview makes the provider show pending placements as in-game
	// holograms during plan. Off by default: plan is supposed to be free of
	// side effects, and holograms - transient as they are - are one.
	preview bool
}

// SetPreview turns plan-time hologram previews on or off.
func (c *Client) SetPreview(on bool) { c.preview = on }

// PreviewEnabled reports whether plan-time hologram previews are on.
func (c *Client) PreviewEnabled() bool { return c.preview }

// New returns a client for the given endpoint, e.g. "http://localhost:8090".
func New(endpoint, token string) *Client {
	return &Client{
		baseURL: strings.TrimSuffix(endpoint, "/") + "/api/v1",
		token:   token,
		// Writes block until the game thread applies them; be generous.
		http: &http.Client{Timeout: 60 * time.Second},
	}
}

// Health verifies the mod is reachable.
func (c *Client) Health(ctx context.Context) error {
	return c.do(ctx, http.MethodGet, "/health", nil, nil)
}

// CreateBuildable spawns a machine or foundation.
// GetBuildableClass reads static, spawn-free facts about a buildable class
// (its clearance footprint). Nothing is placed and no save is touched, so this
// is safe to call during plan.
// ListClasses reads the game's whole buildable-class catalog, classified by
// how each class has to be placed. The first call is slow on the mod's side
// (it loads every class); callers should allow well over the default timeout.
func (c *Client) ListClasses(ctx context.Context) ([]api.ClassInfo, error) {
	var out []api.ClassInfo
	err := c.do(ctx, http.MethodGet, "/classes", nil, &out)
	return out, err
}

func (c *Client) GetBuildableClass(ctx context.Context, class string) (api.BuildableClass, error) {
	var out api.BuildableClass
	err := c.do(ctx, http.MethodGet, "/classes/"+url.PathEscape(class), nil, &out)
	return out, err
}

// ListWorldBuildables reports every buildable within radius centimetres of
// (x,y,z), including ones no Terraform config created. Those come back with an
// empty TFID and are identified only by their index within the response.
func (c *Client) ListWorldBuildables(ctx context.Context, x, y, z, radius float64) ([]api.WorldBuildable, error) {
	q := url.Values{}
	q.Set("x", strconv.FormatFloat(x, 'f', -1, 64))
	q.Set("y", strconv.FormatFloat(y, 'f', -1, 64))
	q.Set("z", strconv.FormatFloat(z, 'f', -1, 64))
	q.Set("radius", strconv.FormatFloat(radius, 'f', -1, 64))
	var out []api.WorldBuildable
	err := c.do(ctx, http.MethodGet, "/world/buildables?"+q.Encode(), nil, &out)
	return out, err
}

// ListResourceNodes reports resource nodes, geysers and water volumes an
// extractor can bind to, filtered and ordered per the query (see
// api.ResourceNodeQuery). Read-only, so safe during plan.
func (c *Client) ListResourceNodes(ctx context.Context, query api.ResourceNodeQuery) ([]api.ResourceNode, error) {
	q := url.Values{}
	if query.Resource != "" {
		q.Set("resource", query.Resource)
	}
	if query.Purity != "" {
		q.Set("purity", query.Purity)
	}
	if query.Center != nil {
		q.Set("x", strconv.FormatFloat(query.Center.X, 'f', -1, 64))
		q.Set("y", strconv.FormatFloat(query.Center.Y, 'f', -1, 64))
		q.Set("z", strconv.FormatFloat(query.Center.Z, 'f', -1, 64))
	}
	if query.Radius != nil {
		q.Set("radius", strconv.FormatFloat(*query.Radius, 'f', -1, 64))
	}
	if query.IncludeOccupied {
		q.Set("include_occupied", "true")
	}
	path := "/world/resource-nodes"
	if len(q) > 0 {
		path += "?" + q.Encode()
	}
	var out []api.ResourceNode
	err := c.do(ctx, http.MethodGet, path, nil, &out)
	return out, err
}

// PreviewBuildable shows a would-be buildable as an in-game hologram and
// returns the game's verdict on its placement. Transient: nothing is built.
func (c *Client) PreviewBuildable(ctx context.Context, p api.Preview) (api.PreviewResult, error) {
	var out api.PreviewResult
	err := c.do(ctx, http.MethodPost, "/preview", p, &out)
	return out, err
}

// ListPreviews reports the holograms still standing.
func (c *Client) ListPreviews(ctx context.Context) ([]api.PreviewResult, error) {
	var out []api.PreviewResult
	err := c.do(ctx, http.MethodGet, "/preview", nil, &out)
	return out, err
}

// ClearPreview removes one hologram. Already gone is not an error.
func (c *Client) ClearPreview(ctx context.Context, tfID string) error {
	err := c.do(ctx, http.MethodDelete, "/preview/"+url.PathEscape(tfID), nil, nil)
	if asNotFound(err) {
		return nil
	}
	return err
}

// ClearPreviews removes every standing hologram and reports how many there were.
func (c *Client) ClearPreviews(ctx context.Context) (int, error) {
	var out struct {
		Removed int `json:"removed"`
	}
	err := c.do(ctx, http.MethodDelete, "/preview", nil, &out)
	return out.Removed, err
}

// ListPlayers reports connected players that have spawned in.
func (c *Client) ListPlayers(ctx context.Context) ([]api.Player, error) {
	var out []api.Player
	err := c.do(ctx, http.MethodGet, "/players", nil, &out)
	return out, err
}

func (c *Client) CreateBuildable(ctx context.Context, b api.Buildable) (api.Buildable, error) {
	var out api.Buildable
	err := c.do(ctx, http.MethodPost, "/buildables", b, &out)
	return out, err
}

// GetBuildable reads one buildable; returns *NotFoundError if it is gone.
func (c *Client) GetBuildable(ctx context.Context, tfID string) (api.Buildable, error) {
	var out api.Buildable
	err := c.do(ctx, http.MethodGet, "/buildables/"+tfID, nil, &out)
	return out, err
}

// PatchBuildable updates recipe and/or clock speed in place.
func (c *Client) PatchBuildable(ctx context.Context, tfID string, p api.BuildablePatch) (api.Buildable, error) {
	var out api.Buildable
	err := c.do(ctx, http.MethodPatch, "/buildables/"+tfID, p, &out)
	return out, err
}

// DeleteBuildable dismantles a buildable. Deleting an already-gone object is
// not an error so destroy stays idempotent.
func (c *Client) DeleteBuildable(ctx context.Context, tfID string) error {
	err := c.do(ctx, http.MethodDelete, "/buildables/"+tfID, nil, nil)
	if asNotFound(err) {
		return nil
	}
	return err
}

// CreateConnection builds a belt or power line.
func (c *Client) CreateConnection(ctx context.Context, conn api.Connection) (api.Connection, error) {
	var out api.Connection
	err := c.do(ctx, http.MethodPost, "/connections", conn, &out)
	return out, err
}

// GetConnection reads one connection; returns *NotFoundError if it is gone.
func (c *Client) GetConnection(ctx context.Context, tfID string) (api.Connection, error) {
	var out api.Connection
	err := c.do(ctx, http.MethodGet, "/connections/"+tfID, nil, &out)
	return out, err
}

// DeleteConnection dismantles a connection, tolerating already-gone objects.
func (c *Client) DeleteConnection(ctx context.Context, tfID string) error {
	err := c.do(ctx, http.MethodDelete, "/connections/"+tfID, nil, nil)
	if asNotFound(err) {
		return nil
	}
	return err
}

// IsNotFound reports whether err means the object no longer exists in-world.
func IsNotFound(err error) bool {
	return asNotFound(err)
}

func asNotFound(err error) bool {
	var nf *NotFoundError
	return errors.As(err, &nf)
}

func (c *Client) do(ctx context.Context, method, path string, in, out any) error {
	var body io.Reader
	if in != nil {
		buf, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(buf)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return err
	}
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("cannot reach the SatisfactoTerraform mod at %s (is the game running with the mod loaded?): %w", c.baseURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return &NotFoundError{TFID: path[strings.LastIndex(path, "/")+1:]}
	}
	if resp.StatusCode >= 300 {
		var e api.Error
		msg := resp.Status
		if json.NewDecoder(resp.Body).Decode(&e) == nil && e.Message != "" {
			msg = e.Message
		}
		return &APIError{Status: resp.StatusCode, Message: msg}
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}
