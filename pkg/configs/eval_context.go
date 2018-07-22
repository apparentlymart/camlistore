package configs

import (
	"strings"

	"github.com/hashicorp/hcl2/hcl"
	"github.com/zclconf/go-cty/cty"
)

func evalContext(envs []string, vars map[string]string) *hcl.EvalContext {
	envVals := make(map[string]cty.Value)
	for _, env := range envs {
		eq := strings.Index(env, "=")
		if eq == -1 {
			continue // invalid
		}
		envVals[env[:eq]] = cty.StringVal(env[eq+1:])
	}

	varVals := make(map[string]cty.Value)
	for k, v := range vars {
		varVals[k] = cty.StringVal(v)
	}

	return &hcl.EvalContext{
		Variables: map[string]cty.Value{
			"env": cty.ObjectVal(envVals),
			"var": cty.ObjectVal(varVals),
		},
	}
}
