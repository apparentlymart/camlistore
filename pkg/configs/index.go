package configs

import (
	"github.com/hashicorp/hcl2/hcl"
)

type Index struct {
	Type        string
	Config      hcl.Body
	EvalContext *hcl.EvalContext

	TypeRange hcl.Range // in case the name turns out invalid
	DeclRange hcl.Range
}

func decodeIndexBlock(block *hcl.Block, ctx *hcl.EvalContext) (*Index, hcl.Diagnostics) {
	return &Index{
		Type:        block.Labels[0],
		Config:      block.Body,
		EvalContext: ctx,

		TypeRange: block.LabelRanges[0],
		DeclRange: block.DefRange,
	}, nil
}
