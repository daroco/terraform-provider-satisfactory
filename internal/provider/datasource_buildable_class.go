package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/daroco/terraform-provider-satisfactory/internal/client"
)

// buildableClassDataSource exposes the footprint a class reserves, so configs
// can size layouts instead of hard-coding spacing by trial and error. The mod
// reads this from the class default object: nothing is spawned, so it is safe
// during plan.
type buildableClassDataSource struct {
	client *client.Client
}

func newBuildableClassDataSource() datasource.DataSource { return &buildableClassDataSource{} }

type vec3Model struct {
	X types.Float64 `tfsdk:"x"`
	Y types.Float64 `tfsdk:"y"`
	Z types.Float64 `tfsdk:"z"`
}

type buildableClassModel struct {
	Class types.String `tfsdk:"class"`
	// Min/Max/Size are the union of the class's clearance boxes, relative to
	// the buildable's origin. All null when the class declares no clearance,
	// which is common - callers must handle that rather than assume a size.
	Min  *vec3Model `tfsdk:"min"`
	Max  *vec3Model `tfsdk:"max"`
	Size *vec3Model `tfsdk:"size"`
}

func (d *buildableClassDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_buildable_class"
}

func vec3Attr(desc string) schema.SingleNestedAttribute {
	return schema.SingleNestedAttribute{
		Computed:    true,
		Description: desc,
		Attributes: map[string]schema.Attribute{
			"x": schema.Float64Attribute{Computed: true},
			"y": schema.Float64Attribute{Computed: true},
			"z": schema.Float64Attribute{Computed: true},
		},
	}
}

func (d *buildableClassDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Footprint of a buildable class, read from the game without placing anything. " +
			"Use it to size layouts instead of guessing spacing; see the grid-2d module.",
		Attributes: map[string]schema.Attribute{
			"class": schema.StringAttribute{
				Required:    true,
				Description: "Buildable class name, e.g. Build_Foundation_8x1_01_C.",
			},
			"min": vec3Attr("Lowest corner of the clearance footprint, in centimetres relative to the buildable's origin. Null if the class declares no clearance."),
			"max": vec3Attr("Highest corner of the clearance footprint. Null if the class declares no clearance."),
			"size": vec3Attr("Footprint extent (max - min), in centimetres. Null if the class declares no clearance. " +
				"This is what you want for spacing: `size.x` is how much room a tile needs along X."),
		},
	}
}

func (d *buildableClassDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data", fmt.Sprintf("expected *client.Client, got %T", req.ProviderData))
		return
	}
	d.client = c
}

func (d *buildableClassDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg buildableClassModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	info, err := d.client.GetBuildableClass(ctx, cfg.Class.ValueString())
	if client.IsNotFound(err) {
		resp.Diagnostics.AddError(
			"Unknown buildable class",
			fmt.Sprintf("The game does not know a buildable class named %q. Class names come from the game's own "+
				"content (e.g. Build_ConstructorMk1_C) and are case-sensitive.", cfg.Class.ValueString()),
		)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Failed to read buildable class", err.Error())
		return
	}

	out := buildableClassModel{Class: types.StringValue(info.Class)}
	// Bounds is absent for classes that reserve no space - leave the three
	// attributes null rather than inventing a zero footprint, so a config that
	// depends on a size fails loudly instead of laying things out on top of
	// each other.
	if info.Bounds != nil {
		out.Min = &vec3Model{X: types.Float64Value(info.Bounds.Min.X), Y: types.Float64Value(info.Bounds.Min.Y), Z: types.Float64Value(info.Bounds.Min.Z)}
		out.Max = &vec3Model{X: types.Float64Value(info.Bounds.Max.X), Y: types.Float64Value(info.Bounds.Max.Y), Z: types.Float64Value(info.Bounds.Max.Z)}
		out.Size = &vec3Model{X: types.Float64Value(info.Bounds.Size.X), Y: types.Float64Value(info.Bounds.Size.Y), Z: types.Float64Value(info.Bounds.Size.Z)}
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, out)...)
}
