package parser

import (
	"fmt"
	"regexp"
	"strings"

	"coremem/internal/memory"
)

type Block struct {
	Type  string
	Scope string
	Title string
	Body  string
}

var blockRE = regexp.MustCompile(`(?s)\[coremem:([^\]]+)\](.*?)\[/coremem\]`)
var attrRE = regexp.MustCompile(`([A-Za-z_]+)=("[^"]*"|[^\s]+)`)

func ParseBlocks(text string) ([]Block, error) {
	matches := blockRE.FindAllStringSubmatch(text, -1)
	if len(matches) == 0 {
		return nil, nil
	}
	out := make([]Block, 0, len(matches))
	for _, m := range matches {
		attrs := parseAttrs(m[1])
		typ := attrs["type"]
		scope := attrs["scope"]
		if scope == "" {
			scope = memory.ScopeRepo
		}
		title := attrs["title"]
		body := strings.TrimSpace(m[2])
		if typ == "" {
			return nil, fmt.Errorf("coremem block missing type")
		}
		if title == "" {
			return nil, fmt.Errorf("coremem block missing title")
		}
		if err := memory.ValidateType(typ); err != nil {
			return nil, err
		}
		if err := memory.ValidateScope(scope); err != nil {
			return nil, err
		}
		if body == "" {
			return nil, fmt.Errorf("coremem block %q has empty body", title)
		}
		out = append(out, Block{Type: typ, Scope: scope, Title: title, Body: body})
	}
	return out, nil
}

func parseAttrs(raw string) map[string]string {
	out := map[string]string{}
	for _, m := range attrRE.FindAllStringSubmatch(raw, -1) {
		val := strings.TrimSpace(m[2])
		val = strings.Trim(val, `"`)
		out[strings.ToLower(m[1])] = val
	}
	return out
}
