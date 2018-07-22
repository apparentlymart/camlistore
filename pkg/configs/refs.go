package configs

import (
	"github.com/hashicorp/hcl2/hcl"
)

func decodeRefExpr(expr hcl.Expression) (string, hcl.Diagnostics) {
	name := hcl.ExprAsKeyword(expr)
	var diags hcl.Diagnostics
	if name == "" {
		diags = append(diags, &hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Invalid handler reference",
			Detail:   "A single identifier is required, giving the name of a handler defined elsewhere in the configuration.",
			Subject:  expr.Range().Ptr(),
		})
	}
	return name, diags
}

func decodeRefListExpr(expr hcl.Expression) ([]string, hcl.Diagnostics) {
	exprs, diags := hcl.ExprList(expr)
	if diags.HasErrors() {
		return nil, diags
	}

	ret := make([]string, 0, len(exprs))
	for _, refExpr := range exprs {
		name, moreDiags := decodeRefExpr(refExpr)
		diags = append(diags, moreDiags...)
		if moreDiags.HasErrors() {
			continue
		}
		ret = append(ret, name)
	}
	return ret, diags
}
