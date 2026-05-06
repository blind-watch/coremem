package memory

import (
	"errors"
	"fmt"
	"strings"
)

var validTypes = map[string]bool{
	TypeCoreDecision:   true,
	TypeCoreConstraint: true,
	TypeCoreNegative:   true,
	TypeCorePreference: true,
	TypeDerivedNote:    true,
	TypeAgentResult:    true,
}

var validScopes = map[string]bool{
	ScopeGlobal:    true,
	ScopeWorkspace: true,
	ScopeRepo:      true,
	ScopeUser:      true,
	ScopeSession:   true,
}

var validAuthorities = map[string]bool{
	AuthorityUserTagged:     true,
	AuthorityAgentTagged:    true,
	AuthoritySystemObserved: true,
}

var validLinkRelations = map[string]bool{
	"supports":      true,
	"contradicts":   true,
	"supersedes":    true,
	"related_to":    true,
	"applies_to":    true,
	"blocks":        true,
	"depends_on":    true,
	"superseded_by": true,
}

func ValidateType(t string) error {
	if validTypes[t] {
		return nil
	}
	return fmt.Errorf("invalid memory type %q", t)
}

func ValidateScope(scope string) error {
	if validScopes[scope] {
		return nil
	}
	return fmt.Errorf("invalid memory scope %q", scope)
}

func validateAddInput(in AddInput) error {
	if err := ValidateType(in.Type); err != nil {
		return err
	}
	if err := ValidateScope(in.Scope); err != nil {
		return err
	}
	if strings.TrimSpace(in.Title) == "" {
		return errors.New("title is required")
	}
	if strings.TrimSpace(in.Body) == "" {
		return errors.New("body is required")
	}
	if in.Authority != "" && !validAuthorities[in.Authority] {
		return fmt.Errorf("invalid authority %q", in.Authority)
	}
	if in.Importance < 0 || in.Importance > 1 {
		return errors.New("importance must be between 0 and 1")
	}
	if in.Confidence < 0 || in.Confidence > 1 {
		return errors.New("confidence must be between 0 and 1")
	}
	return nil
}

func validateLinkInput(in LinkInput) error {
	if strings.TrimSpace(in.SrcNodeID) == "" || strings.TrimSpace(in.DstNodeID) == "" {
		return errors.New("src_node_id and dst_node_id are required")
	}
	if !validLinkRelations[in.Relation] {
		return fmt.Errorf("invalid relation %q", in.Relation)
	}
	if in.Weight < 0 || in.Weight > 1 {
		return errors.New("weight must be between 0 and 1")
	}
	return nil
}
