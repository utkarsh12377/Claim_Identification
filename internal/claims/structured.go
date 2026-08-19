package claims

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/utkarsh/claim-identification/internal/model"
)

type structuredRule struct {
	id       string
	section  string
	key      string
	category model.Category
	accept   func(raw string) bool
	value    func(raw string) string
	evidence func(raw string) string
}

// claims asserted by compliance data rather than by marketing copy
var structuredRules = []structuredRule{
	{
		id:       "origin.country_of_origin_field",
		section:  "complianceInfo",
		key:      "country_of_origin",
		category: model.CategoryOrigin,
		accept:   isPlaceName,
		value:    func(raw string) string { return "Made in " + titleCase(raw) },
		evidence: func(raw string) string { return "country_of_origin: " + raw },
	},
	{
		id:       "certification.fssai_licence_field",
		section:  "complianceInfo",
		key:      "fssai_no",
		category: model.CategoryCertification,
		accept:   isLicenceNumber,
		value:    func(string) string { return "FSSAI certified" },
		evidence: func(raw string) string { return "fssai_no: " + raw },
	},
}

func structuredClaims(p *model.Product) []candidate {
	sections := map[string]map[string]any{
		"complianceInfo": p.ComplianceInfo,
		"attributes":     p.Attributes,
	}

	out := make([]candidate, 0, len(structuredRules))
	for _, r := range structuredRules {
		section, ok := sections[r.section]
		if !ok || section == nil {
			continue
		}
		raw, ok := stringValue(section[r.key])
		if !ok {
			continue
		}
		if r.accept != nil && !r.accept(raw) {
			continue
		}
		out = append(out, candidate{
			ruleID:   r.id,
			category: r.category,
			value:    r.value(raw),
			source:   r.section + "." + r.key,
			evidence: excerpt(r.evidence(raw)),
		})
	}
	return out
}

func stringValue(v any) (string, bool) {
	var s string
	switch typed := v.(type) {
	case string:
		s = typed
	case float64:
		s = fmt.Sprintf("%.0f", typed)
	case fmt.Stringer:
		s = typed.String()
	default:
		return "", false
	}

	s = strings.TrimSpace(s)
	return s, s != ""
}

func isPlaceName(raw string) bool {
	if len(raw) < 3 {
		return false
	}
	for _, r := range raw {
		if !unicode.IsLetter(r) && r != ' ' && r != '-' && r != '.' && r != '\'' {
			return false
		}
	}
	switch strings.ToLower(raw) {
	case "n/a", "na", "none", "unknown", "not applicable":
		return false
	}
	return true
}

func isLicenceNumber(raw string) bool {
	digits := 0
	for _, r := range raw {
		if !unicode.IsDigit(r) {
			return false
		}
		digits++
	}
	return digits >= 10
}

func titleCase(s string) string {
	words := strings.Fields(strings.ToLower(s))
	for i, w := range words {
		runes := []rune(w)
		runes[0] = unicode.ToUpper(runes[0])
		words[i] = string(runes)
	}
	return strings.Join(words, " ")
}
