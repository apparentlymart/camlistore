package configs

import (
	"github.com/hashicorp/hcl2/gohcl"
	"github.com/hashicorp/hcl2/hcl"
)

type Identity struct {
	ID          string
	KeyringPath string

	DeclRange hcl.Range
}

func decodeIdentityBlock(block *hcl.Block, ctx *hcl.EvalContext) (*Identity, hcl.Diagnostics) {
	type identityRaw struct {
		ID          string `hcl:"id"`
		KeyringPath string `hcl:"ring"`
	}

	ret := &Identity{
		DeclRange: block.DefRange,
	}

	var raw identityRaw
	diags := gohcl.DecodeBody(block.Body, ctx, &raw)
	if diags.HasErrors() {
		return ret, diags
	}

	ret.ID = raw.ID
	ret.KeyringPath = raw.KeyringPath

	return ret, diags
}
