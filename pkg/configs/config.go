package configs

import (
	"fmt"
	"os"

	"github.com/hashicorp/hcl2/hcl"
	"github.com/hashicorp/hcl2/hclparse"
)

type Config struct {
	Network   *Network
	Variables map[string]string
	Identity  *Identity
	Server    *Server
	Index     *Index
	Stores    map[string]*Storage
	Syncs     []*Sync
}

// ReadConfig parses the given configuration source code into a new Config
// value.
//
// If an error is returned, the returned Config may be invalid or incomplete.
// The error may be an hcl.Diagnostics describing multiple problems with source
// location information for each problem.
func ReadConfig(src []byte, filename string) (*Config, *hcl.File, error) {
	parser := hclparse.NewParser()
	f, diags := parser.ParseHCL(src, filename)
	if diags.HasErrors() {
		return nil, nil, diags
	}

	envs := os.Environ()

	config, diags := decodeConfig(f.Body, envs)
	if diags.HasErrors() {
		return config, f, diags
	}
	return config, f, nil
}

func decodeConfig(body hcl.Body, envs []string) (*Config, hcl.Diagnostics) {
	vs, body, diags := extractVars(body, envs)

	content, moreDiags := body.Content(rootSchema)
	diags = append(diags, moreDiags...)
	ctx := evalContext(envs, vs)

	config := &Config{
		Variables: vs,
	}
	for _, block := range content.Blocks {
		switch block.Type {

		case "network":
			if config.Network != nil {
				diags = diags.Append(&hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "Duplicate network block",
					Detail:   fmt.Sprintf("The network settings were already configured at %s.", config.Network.DeclRange),
					Subject:  &block.TypeRange,
				})
				continue
			}

			net, moreDiags := decodeNetworkBlock(block, ctx)
			diags = append(diags, moreDiags...)
			config.Network = net

		case "identity":
			if config.Identity != nil {
				diags = diags.Append(&hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "Duplicate identity block",
					Detail:   fmt.Sprintf("The identity settings were already configured at %s.", config.Identity.DeclRange),
					Subject:  &block.TypeRange,
				})
				continue
			}

			ident, moreDiags := decodeIdentityBlock(block, ctx)
			diags = append(diags, moreDiags...)
			config.Identity = ident

		case "server":
			if config.Server != nil {
				diags = diags.Append(&hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "Duplicate server block",
					Detail:   fmt.Sprintf("The server settings were already configured at %s.", config.Server.DeclRange),
					Subject:  &block.TypeRange,
				})
				continue
			}

			server, moreDiags := decodeServerBlock(block, ctx)
			diags = append(diags, moreDiags...)
			config.Server = server

		case "index":
			if config.Index != nil {
				diags = diags.Append(&hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "Duplicate index block",
					Detail:   fmt.Sprintf("The index settings were already configured at %s.", config.Index.DeclRange),
					Subject:  &block.TypeRange,
				})
				continue
			}

			index, moreDiags := decodeIndexBlock(block, ctx)
			diags = append(diags, moreDiags...)
			config.Index = index

		case "storage":
			storage, moreDiags := decodeStorageBlock(block, ctx)
			diags = append(diags, moreDiags...)

			if prev, ok := config.Stores[storage.Type]; ok {
				diags = diags.Append(&hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "Duplicate storage handler",
					Detail:   fmt.Sprintf("A storage handler named %q was already configured at %s.", storage.Type, prev.DeclRange),
					Subject:  &block.TypeRange,
				})
				continue
			}

			if config.Stores == nil {
				config.Stores = make(map[string]*Storage)
			}
			config.Stores[storage.Type] = storage

		case "sync":
			sync, moreDiags := decodeSyncBlock(block, ctx)
			diags = append(diags, moreDiags...)
			config.Syncs = append(config.Syncs, sync)

		default:
			// Should never happen because the above is exhaustive
			panic(fmt.Sprintf("bad root block type %s", block.Type))
		}
	}
	return config, diags
}

var rootSchema = &hcl.BodySchema{
	Blocks: []hcl.BlockHeaderSchema{
		{Type: "network"},
		{Type: "identity"},
		{Type: "server"},
		{Type: "index", LabelNames: []string{"type"}},
		{Type: "storage", LabelNames: []string{"type"}},
		{Type: "sync"},
	},
}
