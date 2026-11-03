// Package provider implements the Terraform provider for RAIL.
package provider

import (
	"context"
	"os"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/HelgeSverre/terraform-provider-rail/internal/rail"
)

var _ provider.Provider = (*railProvider)(nil)

type railProvider struct {
	version string
}

// New returns a constructor for the provider, as providerserver expects.
func New(version string) func() provider.Provider {
	return func() provider.Provider { return &railProvider{version: version} }
}

type providerModel struct {
	Endpoint types.String `tfsdk:"endpoint"`
}

func (p *railProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "rail"
	resp.Version = p.version
}

func (p *railProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Declaratively provision Link Resilience Units on a " +
			"[RAIL](https://github.com/HelgeSverre/rail-rs) (Redundant Array of Inexpensive Links) " +
			"deployment. Infrastructure as Code, for URLs, for redundancy.",
		Attributes: map[string]schema.Attribute{
			"endpoint": schema.StringAttribute{
				MarkdownDescription: "Base URL of the RAIL tier to provision against, usually the top of " +
					"the pyramid. Falls back to the `RAIL_ENDPOINT` environment variable.",
				Optional: true,
			},
		},
	}
}

func (p *railProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var config providerModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if config.Endpoint.IsUnknown() {
		resp.Diagnostics.AddAttributeError(path.Root("endpoint"), "Unknown RAIL endpoint",
			"The endpoint must be known at plan time. RAIL cannot be provisioned against a RAIL that "+
				"does not exist yet. (Yet.)")
		return
	}

	endpoint := os.Getenv("RAIL_ENDPOINT")
	if !config.Endpoint.IsNull() {
		endpoint = config.Endpoint.ValueString()
	}
	if endpoint == "" {
		resp.Diagnostics.AddAttributeError(path.Root("endpoint"), "Missing RAIL endpoint",
			"Set `endpoint` in the provider block or the RAIL_ENDPOINT environment variable.")
		return
	}

	client, err := rail.New(endpoint)
	if err != nil {
		resp.Diagnostics.AddAttributeError(path.Root("endpoint"), "Invalid RAIL endpoint", err.Error())
		return
	}
	resp.ResourceData = client
	resp.DataSourceData = client
}

func (p *railProvider) Resources(context.Context) []func() resource.Resource {
	return []func() resource.Resource{newLinkResource}
}

func (p *railProvider) DataSources(context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{newTopologyDataSource, newTraceDataSource}
}
