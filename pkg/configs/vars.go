package configs

import (
	"fmt"

	"github.com/hashicorp/hcl2/gohcl"
	"github.com/hashicorp/hcl2/hcl"
)

// extractVars reads just the "vars" blocks from a body -- if any are present --
// and produces a map of the defined values along with a body representing
// the remaining configuration.
func extractVars(body hcl.Body, envs []string) (map[string]string, hcl.Body, hcl.Diagnostics) {
	vs := make(map[string]string)
	content, remain, diags := body.PartialContent(rootSchemaVars)

	decls := make(map[string]hcl.Range)
	ctx := evalContext(envs, nil) // no user vars allowed here

	for _, block := range content.Blocks {
		if block.Type != "vars" {
			// Impossible, since our schema only includes this block type
			panic(fmt.Sprintf("bad block type %q during variable processing", block.Type))
		}

		attrs, moreDiags := block.Body.JustAttributes()
		diags = append(diags, moreDiags...)

	Vars:
		for name, attr := range attrs {
			if rng, ok := decls[name]; ok {
				diags = diags.Append(&hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "Duplicate variable definition",
					Detail:   fmt.Sprintf("The variable %q was already defined at %s.", name, rng),
					Subject:  &attr.NameRange,
				})
				continue
			}

			// Variable expressions are not allowed to reference other variables.
			// (Could potentially do a topological sort here and allow inter-references
			// but just forbidding that now for simplicity.)
			for _, ref := range attr.Expr.Variables() {
				if ref.RootName() == "var" {
					diags = diags.Append(&hcl.Diagnostic{
						Severity: hcl.DiagError,
						Summary:  "Invalid reference in user variable",
						Detail:   "The definition of a user variable cannot reference another user variable.",
						Subject:  ref.SourceRange().Ptr(),
						Context:  &attr.Range,
					})
					continue Vars
				}
			}

			var val string
			moreDiags = gohcl.DecodeExpression(attr.Expr, ctx, &val)
			diags = append(diags, moreDiags...)
			decls[name] = attr.NameRange
			if !moreDiags.HasErrors() {
				vs[name] = val
			}
		}
	}

	return vs, remain, diags
}

// The "vars" block is processed separately so that values from it can then
// be referenced elsewhere in the configuration.
var rootSchemaVars = &hcl.BodySchema{
	Blocks: []hcl.BlockHeaderSchema{
		{Type: "vars"},
	},
}
