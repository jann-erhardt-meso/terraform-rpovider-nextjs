// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"os/exec"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Ensure NextJSProvider satisfies various provider interfaces.
var _ provider.Provider = &NextJSProvider{}

// NextJSProvider defines the provider implementation.
type NextJSProvider struct {
	// version is set to the provider version on release, "dev" when the
	// provider is built and ran locally, and "test" when running acceptance
	// testing.
	version string
}

// NextJSProviderModel describes the provider data model.
type NextJSProviderModel struct {
	Executable types.String `tfsdk:"executable"`
}

func (p *NextJSProvider) Metadata(ctx context.Context, req provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "nextjs"
	resp.Version = p.version
}

func (p *NextJSProvider) Schema(ctx context.Context, req provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"executable": schema.StringAttribute{
				MarkdownDescription: "The 'npm' executable used to run the build commands for nextjs with.",
				Optional:            true,
			},
		},
	}
}

func detectExecutableVersion(executable string) (error, string) {
	command := exec.Command(executable, "-v")
	result, err := command.Output()
	if err != nil {
		return err, ""
	}
	return nil, string(result)
}

func (p *NextJSProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var data NextJSProviderModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	// Configuration values are now available.
	tflog.Trace(ctx, "Configuration init...")
	if data.Executable.IsNull() {
		err, version := detectExecutableVersion("npm")
		if err != nil {
			resp.Diagnostics.AddError("Error detecting NPM!", "Error detecting NPM version: "+err.Error())
			return
		}
		data.Executable = types.StringValue("npm")
		tflog.Trace(ctx, fmt.Sprintf("Executable Version detected: %s", version))
	} else {
		err, version := detectExecutableVersion(data.Executable.ValueString())
		if err != nil {
			resp.Diagnostics.AddError(fmt.Sprintf("Error detecting %s!", data.Executable.ValueString()), "Error detecting NPM version: "+err.Error())
			return
		}
		tflog.Trace(ctx, fmt.Sprintf("Executable Version detected: %s", version))
	}

	resp.ResourceData = data.Executable.ValueString()
}

func (p *NextJSProvider) Resources(ctx context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewBuildCommand,
	}
}

func (p *NextJSProvider) DataSources(ctx context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewBuildOutput,
	}
}

func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &NextJSProvider{
			version: version,
		}
	}
}
