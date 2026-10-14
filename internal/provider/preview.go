package provider

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"

	"github.com/daroco/terraform-provider-satisfactory/internal/api"
	"github.com/daroco/terraform-provider-satisfactory/internal/client"
)

// Plan-time holograms.
//
// With the provider's `preview` setting on, every placeable resource's
// ModifyPlan sends its planned placement to POST /preview. The mod spawns
// the game's own hologram there and leaves it standing, so the whole
// pending change is visible in the world before apply - and reports whether
// the build gun would accept that placement, which becomes a plan warning.
//
// The hologram's verdict is advisory. POST /buildables deliberately bypasses
// hologram rules (clearance, floor contact, ...) because a config may want
// exactly that; a red ghost says "look at this", not "this will fail".

// previewKey identifies a hologram for a resource that may not have a tf_id
// yet (a create has no id until apply). It is derived from what is being
// placed, so the same plan re-run replaces its ghosts instead of stacking
// them, and Create can find and remove its own ghost by recomputing it.
func previewKey(p api.Preview) string {
	var b strings.Builder
	b.WriteString(p.Class)
	if p.Transform != nil {
		fmt.Fprintf(&b, "|%g|%g|%g|%g", p.Transform.X, p.Transform.Y, p.Transform.Z, p.Transform.Yaw)
	}
	b.WriteString("|" + p.NodeID)
	if p.Placement != nil {
		for _, v := range []*float64{p.Placement.X, p.Placement.Y, p.Placement.Yaw} {
			if v != nil {
				fmt.Fprintf(&b, "|%g", *v)
			} else {
				b.WriteString("|-")
			}
		}
	}
	sum := sha1.Sum([]byte(b.String()))
	return "plan:" + hex.EncodeToString(sum[:8])
}

// previewPlacement shows one planned placement as a hologram and turns an
// invalid verdict into a warning. Never an error: a preview that cannot be
// shown must not stop a plan, and neither must a placement the build gun
// dislikes. Create recomputes the same key to clear the ghost.
func previewPlacement(ctx context.Context, c *client.Client, what string, p api.Preview, diags *diag.Diagnostics) {
	p.TFID = previewKey(p)
	res, err := c.PreviewBuildable(ctx, p)
	if err != nil {
		diags.AddWarning(
			"Could not preview "+what,
			"The mod did not show a hologram for this placement: "+err.Error()+
				". The plan is unaffected; set preview = false to silence this.",
		)
		return
	}
	if res.Valid == nil {
		diags.AddWarning(
			"Placement of "+what+" could not be validated",
			fmt.Sprintf("The game could not run its placement check for %s (%s). It is still shown as a hologram at (%g, %g, %g).",
				what, strings.Join(res.Disqualifiers, ", "), res.X, res.Y, res.Z),
		)
		return
	}
	if !*res.Valid {
		diags.AddWarning(
			"Placement of "+what+" would be refused by the build gun",
			fmt.Sprintf("The game's hologram at (%g, %g, %g) reports: %s. Apply will still place it - the API does not "+
				"enforce hologram rules - but it may overlap, float, or fail to snap. Look at the red ghost in-game.",
				res.X, res.Y, res.Z, strings.Join(res.Disqualifiers, ", ")),
		)
	}
}

// clearPreview removes the ghost a plan left for this placement, right before
// the real thing is built. Best-effort: a missing ghost is not an error.
func clearPreview(ctx context.Context, c *client.Client, p api.Preview) {
	if c == nil || !c.PreviewEnabled() {
		return
	}
	_ = c.ClearPreview(ctx, previewKey(p))
}
