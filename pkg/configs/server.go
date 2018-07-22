package configs

import (
	"github.com/hashicorp/hcl2/gohcl"
	"github.com/hashicorp/hcl2/hcl"
)

type Server struct {
	ShareHandler  bool
	UIHandler     bool
	SearchHandler bool

	BlobWrites []string

	DeclRange hcl.Range
}

func decodeServerBlock(block *hcl.Block, ctx *hcl.EvalContext) (*Server, hcl.Diagnostics) {
	type serverRaw struct {
		ShareHandler  *bool `hcl:"share_handler"`
		UIHandler     *bool `hcl:"ui_handler"`
		SearchHandler *bool `hcl:"search_handler"`

		BlobWrites hcl.Expression `hcl:"blob_writes"` // contains references, so we'll decode separately
	}

	ret := &Server{
		DeclRange: block.DefRange,
	}

	var raw serverRaw
	diags := gohcl.DecodeBody(block.Body, ctx, &raw)
	if diags.HasErrors() {
		return ret, diags
	}

	if raw.ShareHandler != nil {
		ret.ShareHandler = *raw.ShareHandler
	} else {
		ret.ShareHandler = true
	}

	if raw.UIHandler != nil {
		ret.UIHandler = *raw.UIHandler
	} else {
		ret.UIHandler = true
	}

	if raw.SearchHandler != nil {
		ret.SearchHandler = *raw.SearchHandler
	} else {
		ret.SearchHandler = true
	}

	if raw.BlobWrites != nil {
		names, moreDiags := decodeRefListExpr(raw.BlobWrites)
		diags = append(diags, moreDiags...)
		ret.BlobWrites = names
	}

	return ret, diags
}
