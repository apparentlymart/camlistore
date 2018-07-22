package configs

import (
	"fmt"
	"time"

	"github.com/hashicorp/hcl2/gohcl"
	"github.com/hashicorp/hcl2/hcl"
)

type Sync struct {
	From           string
	To             string
	VerifyInterval time.Duration

	FromRange hcl.Range
	ToRange   hcl.Range
	DeclRange hcl.Range
}

func decodeSyncBlock(block *hcl.Block, ctx *hcl.EvalContext) (*Sync, hcl.Diagnostics) {
	type syncRaw struct {
		From           hcl.Expression `hcl:"from"`
		To             hcl.Expression `hcl:"to"`
		VerifyInterval hcl.Expression `hcl:"verify_interval"`
	}

	ret := &Sync{
		DeclRange: block.DefRange,
	}

	var raw syncRaw
	diags := gohcl.DecodeBody(block.Body, ctx, &raw)
	if diags.HasErrors() {
		return ret, diags
	}

	if raw.From != nil {
		name, moreDiags := decodeRefExpr(raw.From)
		diags = append(diags, moreDiags...)
		ret.From = name
	}

	if raw.To != nil {
		name, moreDiags := decodeRefExpr(raw.To)
		diags = append(diags, moreDiags...)
		ret.To = name
	}

	if raw.VerifyInterval != nil {
		var durStr string
		moreDiags := gohcl.DecodeExpression(raw.VerifyInterval, ctx, &durStr)
		diags = append(diags, moreDiags...)
		if !moreDiags.HasErrors() {
			dur, err := time.ParseDuration(durStr)
			if err != nil {
				diags = append(diags, &hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "Invalid \"verify_interval\" argument",
					Detail:   fmt.Sprintf("The \"verify_interval\" value is not a valid duration string: %s.", err),
					Subject:  raw.VerifyInterval.Range().Ptr(),
				})
			}
			ret.VerifyInterval = dur
		}
	}

	return ret, diags
}
