// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listdefault"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"os"
	"os/exec"
	path2 "path"
	"strings"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ resource.Resource = &BuildCommand{}
var _ resource.ResourceWithImportState = &BuildCommand{}

func NewBuildCommand() resource.Resource {
	return &BuildCommand{}
}

// BuildCommand defines the resource implementation.
type BuildCommand struct {
	executable string
}

// BuildCommandModel describes the resource data model.
type BuildCommandModel struct {
	SourcePath types.String `tfsdk:"source_path"`
	Commands   types.List   `tfsdk:"commands"`
	Data       types.List   `tfsdk:"data"`
}

func (r *BuildCommand) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_build_command"
}

func (r *BuildCommand) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		// This description is used by the documentation generator and the language server.
		MarkdownDescription: "Example resource",

		Attributes: map[string]schema.Attribute{
			"source_path": schema.StringAttribute{
				MarkdownDescription: "Example configurable attribute",
				Optional:            false,
				Required:            true,
			},
			"commands": schema.ListAttribute{
				MarkdownDescription: "Example configurable attribute with default value",
				Optional:            true,
				Computed:            true,
				Default:             listdefault.StaticValue(types.ListValueMust(types.StringType, []attr.Value{types.StringValue("install --cache .npm --prefer-offline"), types.StringValue("run-script build"), types.StringValue("run-script package")})),
				ElementType:         types.StringType,
			},
			"data": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Example identifier",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"path": schema.StringAttribute{
							MarkdownDescription: "Example path",
							Optional:            true,
							Computed:            true,
						},
						"sha256": schema.StringAttribute{
							MarkdownDescription: "Example sha256",
							Optional:            true,
							Computed:            true,
						},
						"function_name": schema.StringAttribute{
							MarkdownDescription: "Example function",
							Optional:            true,
							Computed:            true,
						},
					},
				},
			},
		},
	}
}

func (r *BuildCommand) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	// Prevent panic if the provider has not been configured.
	if req.ProviderData == nil {
		return
	}

	executable, ok := req.ProviderData.(string)

	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected string, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)

		return
	}

	r.executable = executable
}

func exists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

func safeSplit(s string) []string {
	split := strings.Split(s, " ")

	var result []string
	var inquote string
	var block string
	for _, i := range split {
		if inquote == "" {
			if strings.HasPrefix(i, "'") || strings.HasPrefix(i, "\"") {
				inquote = string(i[0])
				block = strings.TrimPrefix(i, inquote) + " "
			} else {
				result = append(result, i)
			}
		} else {
			if !strings.HasSuffix(i, inquote) {
				block += i + " "
			} else {
				block += strings.TrimSuffix(i, inquote)
				inquote = ""
				result = append(result, block)
				block = ""
			}
		}
	}

	return result
}

func (r *BuildCommand) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data BuildCommandModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	// Check Values: SourcePath
	if data.SourcePath.IsNull() || data.SourcePath.IsUnknown() {
		resp.Diagnostics.AddError("Source Path needed", "Source Path needed in order to create Functions")
		return
	}

	exist, err := exists(data.SourcePath.ValueString())
	if !exist || err != nil {
		resp.Diagnostics.AddError("Source Path is not Valid", "The Source Path either does not exist, or Terraform cannot access the Path.")
		return
	}

	// Execute Commands with Executable
	var elements []types.String
	diags := data.Commands.ElementsAs(ctx, &elements, false)
	if diags.HasError() {
		tflog.Error(ctx, fmt.Sprintf("Failed to construct Data. %s", diags.Errors()))
		return
	}

	for index, element := range elements {
		tflog.Trace(ctx, fmt.Sprintf("Executing Item-%d: %s", index, element.ValueString()))
		commandArgs := safeSplit(element.ValueString())
		command := exec.Command(r.executable, commandArgs...)
		command.Dir = data.SourcePath.ValueString()
		result, err := command.Output()
		if err != nil {
			tflog.Debug(ctx, fmt.Sprintf("Failed command output: %s", result))
			resp.Diagnostics.AddError(fmt.Sprintf("Could not execute Command: %s", command.String()), err.Error())
			return
		}
		tflog.Trace(ctx, fmt.Sprintf("Result from Command-%d: %s", index, result))
	}

	nextJSStateFile := path2.Join(data.SourcePath.ValueString(), ".serverless", "serverless-state.json")
	exist, err = exists(nextJSStateFile)
	if !exist || err != nil {
		resp.Diagnostics.AddError("Source Path is not Valid", "The Source Path either does not exist, or Terraform cannot access the Path.")
		return
	}

	plan, _ := os.ReadFile(nextJSStateFile)
	var nextJSState interface{}
	err = json.Unmarshal(plan, &nextJSState)
	if err != nil {
		resp.Diagnostics.AddError("Unable to parse NextJS State", fmt.Sprintf("The File: %s could not be read and the following error was produced: %s.", nextJSStateFile, err.Error()))
		return
	}

	tflog.Trace(ctx, fmt.Sprintf("Following nextJS State found: %v", nextJSState))

	mapElements := map[string]attr.Value{
		"path":          types.StringValue("value1"),
		"sha256":        types.StringValue("value2"),
		"function_name": types.StringValue("value3"),
	}
	elementTypes := map[string]attr.Type{
		"path":          types.StringType,
		"sha256":        types.StringType,
		"function_name": types.StringType,
	}
	mapValue, diags := types.ObjectValue(elementTypes, mapElements)

	if diags.HasError() {
		tflog.Error(ctx, fmt.Sprintf("Failed to construct Data. %s", diags.Errors()))
		return
	}

	listElements := []types.Object{mapValue}
	tflog.Trace(ctx, "Creating list Value...")
	listValue, diags := types.ListValueFrom(ctx, mapValue.Type(ctx), listElements)

	if diags.HasError() {
		tflog.Error(ctx, fmt.Sprintf("Failed to construct Data. %s", diags.Errors()))
		return
	}

	tflog.Trace(ctx, "Filling data...")
	data.Data = listValue

	tflog.Trace(ctx, fmt.Sprintf("Saved Data: %s", data.Data.String()))

	// Write logs using the tflog package
	// Documentation: https://terraform.io/plugin/log
	tflog.Trace(ctx, "created a resource")

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *BuildCommand) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data BuildCommandModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	// If applicable, this is a great opportunity to initialize any necessary
	// provider client data and make a call using it.
	// httpResp, err := r.client.Do(httpReq)
	// if err != nil {
	//     resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read example, got error: %s", err))
	//     return
	// }

	// Save updated data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *BuildCommand) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data BuildCommandModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	// If applicable, this is a great opportunity to initialize any necessary
	// provider client data and make a call using it.
	// httpResp, err := r.client.Do(httpReq)
	// if err != nil {
	//     resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to update example, got error: %s", err))
	//     return
	// }

	// Save updated data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *BuildCommand) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data BuildCommandModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	// If applicable, this is a great opportunity to initialize any necessary
	// provider client data and make a call using it.
	// httpResp, err := r.client.Do(httpReq)
	// if err != nil {
	//     resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to delete example, got error: %s", err))
	//     return
	// }
}

func (r *BuildCommand) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
