package claims

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/utkarsh/claim-identification/internal/model"
	"github.com/utkarsh/claim-identification/internal/uid"
)

const maxEvidenceRunes = 220

type Detector struct {
	rules             []compiledRule
	includeStructured bool
	newID             func() string
}

type Option func(*Detector)

func WithStructuredFields(enabled bool) Option {
	return func(d *Detector) { d.includeStructured = enabled }
}

func WithIDFunc(fn func() string) Option {
	return func(d *Detector) { d.newID = fn }
}

func New(opts ...Option) *Detector {
	d := &Detector{
		rules:             compiledRules,
		includeStructured: true,
		newID:             uid.NewUUID,
	}
	for _, opt := range opts {
		opt(d)
	}
	return d
}

type candidate struct {
	ruleID   string
	category model.Category
	value    string
	source   string
	evidence string
}

func (d *Detector) Detect(p *model.Product) []model.Claim {
	if p == nil {
		return nil
	}

	var candidates []candidate
	for _, field := range scanFields(p) {
		candidates = append(candidates, d.scanText(field)...)
	}
	if d.includeStructured {
		candidates = append(candidates, structuredClaims(p)...)
	}

	claims := make([]model.Claim, 0, len(candidates))
	seen := make(map[string]bool, len(candidates))
	for _, c := range candidates {
		key := dedupeKey(c)
		if seen[key] {
			continue
		}
		seen[key] = true

		claims = append(claims, model.Claim{
			ID:         d.newID(),
			ClaimType:  c.category,
			ClaimValue: c.value,
			Status:     model.ClaimStatusIdentified,
			Source:     c.source,
			Evidence:   c.evidence,
			RuleID:     c.ruleID,
		})
	}
	return claims
}

type scanField struct {
	path string
	text string
}

// only marketing copy is scanned, never ingredients or the nutrition table
func scanFields(p *model.Product) []scanField {
	fields := make([]scanField, 0, len(p.AboutItems)+3)

	add := func(path, text string) {
		if strings.TrimSpace(text) != "" {
			fields = append(fields, scanField{path: path, text: text})
		}
	}

	add("title", p.Title)
	for i, item := range p.AboutItems {
		add("aboutItems["+strconv.Itoa(i)+"]", item)
	}
	add("shortDescription", p.ShortDescription)
	add("longDescription", p.LongDescription)

	return fields
}

type match struct {
	start, end int
	ruleIdx    int
	ruleID     string
	category   model.Category
}

func (d *Detector) scanText(field scanField) []candidate {
	var matches []match

	for ruleIdx, r := range d.rules {
		for _, re := range r.patterns {
			for _, loc := range re.FindAllStringIndex(field.text, -1) {
				start, end := loc[0], loc[1]
				matched := field.text[start:end]

				if excludedBy(r.excludes, matched) {
					continue
				}
				if isNegated(field.text, start) {
					continue
				}
				matches = append(matches, match{
					start:    start,
					end:      end,
					ruleIdx:  ruleIdx,
					ruleID:   r.id,
					category: r.category,
				})
			}
		}
	}

	accepted := resolveOverlaps(matches)

	out := make([]candidate, 0, len(accepted))
	for _, m := range accepted {
		value := cleanValue(field.text[m.start:m.end])
		if value == "" {
			continue
		}
		out = append(out, candidate{
			ruleID:   m.ruleID,
			category: m.category,
			value:    value,
			source:   field.path,
			evidence: excerpt(field.text),
		})
	}
	return out
}

// longest match wins, so "authentic recipe" beats "authentic"
func resolveOverlaps(matches []match) []match {
	if len(matches) < 2 {
		return matches
	}

	sort.SliceStable(matches, func(i, j int) bool {
		a, b := matches[i], matches[j]
		if a.start != b.start {
			return a.start < b.start
		}
		if a.end != b.end {
			return a.end > b.end
		}
		return a.ruleIdx < b.ruleIdx
	})

	accepted := make([]match, 0, len(matches))
	for _, m := range matches {
		overlaps := false
		for _, a := range accepted {
			if m.start < a.end && a.start < m.end {
				overlaps = true
				break
			}
		}
		if !overlaps {
			accepted = append(accepted, m)
		}
	}

	sort.SliceStable(accepted, func(i, j int) bool { return accepted[i].start < accepted[j].start })
	return accepted
}

func excludedBy(excludes []*regexp.Regexp, matched string) bool {
	for _, re := range excludes {
		if re.MatchString(matched) {
			return true
		}
	}
	return false
}

var negationRe = regexp.MustCompile(`(?i)\b(?:not|never|isn't|aren't|doesn't|don't|without|free from)\s+(?:\w+\s+){0,2}$`)

const negationWindow = 32

func isNegated(text string, start int) bool {
	lo := start - negationWindow
	if lo < 0 {
		lo = 0
	}
	for lo < start && !utf8.RuneStart(text[lo]) {
		lo++
	}
	return negationRe.MatchString(text[lo:start])
}

var whitespaceRe = regexp.MustCompile(`\s+`)

func cleanValue(s string) string {
	s = whitespaceRe.ReplaceAllString(s, " ")
	return strings.Trim(s, " \t\n\r.,;:!-&")
}

func excerpt(s string) string {
	s = strings.TrimSpace(whitespaceRe.ReplaceAllString(s, " "))
	if utf8.RuneCountInString(s) <= maxEvidenceRunes {
		return s
	}
	runes := []rune(s)
	return strings.TrimSpace(string(runes[:maxEvidenceRunes])) + "..."
}

var nonAlphanumericRe = regexp.MustCompile(`[^a-z0-9]+`)

func dedupeKey(c candidate) string {
	s := strings.ToLower(c.value)
	s = strings.ReplaceAll(s, "&", " and ")
	s = nonAlphanumericRe.ReplaceAllString(s, " ")

	tokens := strings.Fields(s)
	for i, t := range tokens {
		if len(t) > 3 && strings.HasSuffix(t, "s") && !strings.HasSuffix(t, "ss") {
			tokens[i] = strings.TrimSuffix(t, "s")
		}
	}
	return string(c.category) + "|" + strings.Join(tokens, " ")
}
