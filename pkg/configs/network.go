package configs

import (
	"fmt"
	"net"
	"net/url"

	"github.com/hashicorp/hcl2/gohcl"
	"github.com/hashicorp/hcl2/hcl"
)

type Network struct {
	BaseURL    *url.URL
	ListenAddr *net.TCPAddr
	HTTPS      bool
	Auths      []*Auth

	DeclRange hcl.Range
}

type Auth struct {
	Type   string
	Config hcl.Body

	TypeRange hcl.Range // to report if "Type" is not a valid auth type
	DeclRange hcl.Range
}

func decodeNetworkBlock(block *hcl.Block, ctx *hcl.EvalContext) (*Network, hcl.Diagnostics) {
	type networkRaw struct {
		BaseURL    hcl.Expression `hcl:"base_url"`
		ListenAddr hcl.Expression `hcl:"listen"`
		HTTPS      *bool          `hcl:"https"`
		Remain     hcl.Body       `hcl:",remain"`
	}

	ret := &Network{
		DeclRange: block.DefRange,
	}

	var raw networkRaw
	diags := gohcl.DecodeBody(block.Body, ctx, &raw)
	if diags.HasErrors() {
		return ret, diags
	}

	if raw.BaseURL != nil {
		var urlStr string
		moreDiags := gohcl.DecodeExpression(raw.BaseURL, ctx, &urlStr)
		diags = append(diags, moreDiags...)
		if !moreDiags.HasErrors() {
			url, err := url.Parse(urlStr)
			if err != nil {
				diags = diags.Append(&hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "Invalid \"base_url\" argument",
					Detail:   fmt.Sprintf("The \"base_url\" argument is not a valid URL: %s.", err),
					Subject:  raw.BaseURL.Range().Ptr(),
				})
			}
			ret.BaseURL = url
		}
	}

	if raw.ListenAddr != nil {
		var addrStr string
		moreDiags := gohcl.DecodeExpression(raw.ListenAddr, ctx, &addrStr)
		diags = append(diags, moreDiags...)
		if !moreDiags.HasErrors() {
			addr, err := net.ResolveTCPAddr("tcp", addrStr)
			if err != nil {
				diags = diags.Append(&hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "Invalid \"listen\" argument",
					Detail:   fmt.Sprintf("The \"listen\" argument is not a valid TCP address: %s.", err),
					Subject:  raw.ListenAddr.Range().Ptr(),
				})
			}
			ret.ListenAddr = addr
		}
	}

	if raw.HTTPS != nil {
		ret.HTTPS = *raw.HTTPS
	} else {
		ret.HTTPS = true
	}

	content, moreDiags := raw.Remain.Content(&hcl.BodySchema{
		Blocks: []hcl.BlockHeaderSchema{
			{Type: "auth", LabelNames: []string{"type"}},
		},
	})
	diags = append(diags, moreDiags...)
	for _, authBlock := range content.Blocks {
		auth, moreDiags := decodeAuthBlock(authBlock, ctx)
		diags = append(diags, moreDiags...)
		if auth != nil {
			ret.Auths = append(ret.Auths, auth)
		}
	}

	return ret, diags
}

func decodeAuthBlock(block *hcl.Block, ctx *hcl.EvalContext) (*Auth, hcl.Diagnostics) {
	return &Auth{
		Type:   block.Labels[0],
		Config: block.Body,

		TypeRange: block.LabelRanges[0],
		DeclRange: block.DefRange,
	}, nil
}
