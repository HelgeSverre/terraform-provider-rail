package provider

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/float64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/HelgeSverre/terraform-provider-rail/internal/rail"
)

var (
	_ resource.Resource                = (*linkResource)(nil)
	_ resource.ResourceWithConfigure   = (*linkResource)(nil)
	_ resource.ResourceWithImportState = (*linkResource)(nil)
)

func newLinkResource() resource.Resource { return &linkResource{} }

type linkResource struct {
	client *rail.Client
}

type linkModel struct {
	ID               types.String  `tfsdk:"id"`
	URL              types.String  `tfsdk:"url"`
	Target           types.String  `tfsdk:"target"`
	ShortURL         types.String  `tfsdk:"short_url"`
	Degraded         types.Bool    `tfsdk:"degraded"`
	CompressionRatio types.Float64 `tfsdk:"compression_ratio"`
	CreatedAt        types.Int64   `tfsdk:"created_at"`
	Mirrors          types.List    `tfsdk:"mirrors"`
}

type mirrorModel struct {
	Upstream types.String `tfsdk:"upstream"`
	ShortURL types.String `tfsdk:"short_url"`
	Healthy  types.Bool   `tfsdk:"healthy"`
}

var mirrorType = types.ObjectType{AttrTypes: map[string]attr.Type{
	"upstream":  types.StringType,
	"short_url": types.StringType,
	"healthy":   types.BoolType,
}}

func (r *linkResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_link"
}

func (r *linkResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	keep := []planmodifier.String{stringplanmodifier.UseStateForUnknown()}
	resp.Schema = schema.Schema{
		MarkdownDescription: "A Link Resilience Unit: one destination URL, shortened by every upstream " +
			"vendor of the RAIL tier and load-balanced across them.\n\n" +
			"~> **Links are permanent.** RAIL does not support deleting links, in keeping with " +
			"industry-standard data retention practices. Destroying this resource removes it from " +
			"Terraform state only; the link remains provisioned and continues to resolve.\n\n" +
			"Provisioning is idempotent: a second `rail_link` for the same destination adopts the " +
			"existing link.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "The link id, as in `<endpoint>/<id>`.",
				Computed:            true,
				PlanModifiers:       keep,
			},
			"url": schema.StringAttribute{
				MarkdownDescription: "Destination URL. Changing it to a different destination provisions " +
					"a new link; a change RAIL normalizes away (host case, a missing root slash) only " +
					"updates state.",
				Required: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplaceIf(
						func(_ context.Context, req planmodifier.StringRequest, resp *stringplanmodifier.RequiresReplaceIfFuncResponse) {
							resp.RequiresReplace = normalize(req.StateValue.ValueString()) != normalize(req.PlanValue.ValueString())
						},
						"Replace only when the normalized destination changes.",
						"Replace only when the normalized destination changes.",
					),
				},
			},
			"target": schema.StringAttribute{
				MarkdownDescription: "The destination as RAIL normalized it.",
				Computed:            true,
				PlanModifiers:       keep,
			},
			"short_url": schema.StringAttribute{
				MarkdownDescription: "The short URL to distribute.",
				Computed:            true,
				PlanModifiers:       keep,
			},
			"degraded": schema.BoolAttribute{
				MarkdownDescription: "True when no mirror is healthy and the tier redirects straight to " +
					"the destination (Degraded Mode). Observed at the last refresh.",
				Computed:      true,
				PlanModifiers: []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()},
			},
			"compression_ratio": schema.Float64Attribute{
				MarkdownDescription: "Short URL length divided by destination length. Values above 1.0 " +
					"are expected in multi-tier deployments.",
				Computed:      true,
				PlanModifiers: []planmodifier.Float64{float64planmodifier.UseStateForUnknown()},
			},
			"created_at": schema.Int64Attribute{
				MarkdownDescription: "Unix time the link was provisioned.",
				Computed:            true,
				PlanModifiers:       []planmodifier.Int64{int64planmodifier.UseStateForUnknown()},
			},
			"mirrors": schema.ListNestedAttribute{
				MarkdownDescription: "One entry per vendor copy of the link, as of the last refresh.",
				Computed:            true,
				PlanModifiers:       []planmodifier.List{listplanmodifier.UseStateForUnknown()},
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"upstream":  schema.StringAttribute{Computed: true, MarkdownDescription: "Vendor identifier."},
						"short_url": schema.StringAttribute{Computed: true, MarkdownDescription: "The vendor-issued short URL."},
						"healthy":   schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether the mirror is in rotation."},
					},
				},
			},
		},
	}
}

func (r *linkResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	client, ok := req.ProviderData.(*rail.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data", fmt.Sprintf("got %T", req.ProviderData))
		return
	}
	r.client = client
}

func (r *linkResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan linkModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	link, err := r.client.Shorten(ctx, plan.URL.ValueString())
	if err != nil {
		resp.Diagnostics.AddAttributeError(path.Root("url"), "Could not provision link", err.Error())
		return
	}
	for _, f := range link.Failed {
		resp.Diagnostics.AddWarning(
			fmt.Sprintf("Upstream %s did not provision a mirror", f.Upstream),
			f.Error,
		)
	}
	resp.Diagnostics.Append(apply(ctx, &plan, link)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *linkResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state linkModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	link, err := r.client.Link(ctx, state.ID.ValueString())
	if errors.Is(err, rail.ErrNotFound) {
		// The tier lost it (new database, different tier). Terraform will provision it again.
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Could not read link", err.Error())
		return
	}
	if state.URL.IsNull() {
		// Imported: the destination is all RAIL knows.
		state.URL = types.StringValue(link.Target)
	}
	resp.Diagnostics.Append(apply(ctx, &state, link)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update only happens when `url` changed in a way RAIL normalizes away: the link is the same, so
// only the configured spelling is recorded.
func (r *linkResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state linkModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	state.URL = plan.URL
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *linkResource) Delete(_ context.Context, _ resource.DeleteRequest, resp *resource.DeleteResponse) {
	resp.Diagnostics.AddWarning(
		"Link retained",
		"RAIL links are permanent, in keeping with industry-standard data retention practices. "+
			"The link has been removed from Terraform state and remains provisioned. "+
			"It will continue to resolve.",
	)
}

func (r *linkResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}

func apply(ctx context.Context, m *linkModel, link *rail.Link) diag.Diagnostics {
	m.ID = types.StringValue(link.ID)
	m.Target = types.StringValue(link.Target)
	m.ShortURL = types.StringValue(link.ShortURL)
	m.Degraded = types.BoolValue(link.Degraded)
	m.CompressionRatio = types.Float64Value(link.CompressionRatio)
	m.CreatedAt = types.Int64Value(link.CreatedAt)
	mirrors := make([]mirrorModel, 0, len(link.Mirrors))
	for _, mi := range link.Mirrors {
		mirrors = append(mirrors, mirrorModel{
			Upstream: types.StringValue(mi.Upstream),
			ShortURL: types.StringValue(mi.ShortURL),
			Healthy:  types.BoolValue(mi.Healthy),
		})
	}
	list, d := types.ListValueFrom(ctx, mirrorType, mirrors)
	m.Mirrors = list
	return d
}

// normalize mirrors the URL normalization RAIL applies to destinations, for the common cases.
func normalize(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return raw
	}
	u.Scheme = strings.ToLower(u.Scheme)
	u.Host = strings.ToLower(u.Host)
	if u.Path == "" {
		u.Path = "/"
	}
	return u.String()
}
