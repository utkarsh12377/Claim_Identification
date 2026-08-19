package claims

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/utkarsh/claim-identification/internal/model"
)

// word lists the patterns below plug in via {{NUT}}, {{COND}} etc.
const (
	nutrients = `(?:proteins?|fibres?|fibers?|iron|calcium|zinc|magnesium|potassium|folate|vitamins?(?:\s+[abcdek]\d?)?|omega[\s-]?3|antioxidants?|whole\s?grains?|sugars?|salt|sodium|fats?|calories?|carbs?|carbohydrates?|cholesterol|gluten|lactose|energy)`

	conditions = `(?:diabetes|diabetic|arthritis|cancers?|tumou?rs?|asthma|obesity|hypertension|blood\s+pressure|blood\s+sugar|cholesterol|infections?|diseases?|illness(?:es)?|depression|anxiety|insomnia|migraines?|ulcers?|thyroid|pcos|pcod|constipation|acidity|joint\s+pain|covid(?:[\s-]?19)?|flu|colds?)`

	functions = `(?:immunity|immune\s+system|immune\s+health|digestion|digestive\s+health|gut\s+health|metabolism|bone\s+health|joint\s+health|heart\s+health|eye\s+health|skin\s+health|hair\s+health|brain\s+(?:health|function)|energy\s+levels|hydration|overall\s+wellness|well[\s-]?being|healthy\s+growth|muscle\s+growth|weight\s+management|sleep)`

	countries = `(?:india|bharat|china|usa|u\.s\.a\.|united\s+states|america|japan|germany|italy|france|switzerland|uk|united\s+kingdom|korea|taiwan|vietnam|thailand|indonesia|malaysia|bangladesh|nepal|bhutan|sri\s+lanka|spain|portugal|turkey|brazil|mexico|canada|australia|new\s+zealand|netherlands|belgium|sweden|denmark|norway|poland|israel|singapore|uae)`

	baselines = `(?:ordinary|regular|normal|standard|conventional|traditional|others?|other\s+brands?|leading\s+brands?|the\s+leading\s+brand|the\s+rest|the\s+market)`
)

type rule struct {
	id            string
	category      model.Category
	patterns      []string
	excludes      []string
	caseSensitive bool
}

var ruleSet = []rule{
	{
		id:       "performance.long_lasting",
		category: model.CategoryPerformance,
		patterns: []string{
			`\blong[\s-]?lasting\b`,
			`\blasts?\s+(?:up\s+to\s+)?\d+\s*(?:x|times|hours?|hrs?|days?|weeks?|months?)\b`,
		},
	},
	{
		id:       "performance.more_effective",
		category: model.CategoryPerformance,
		patterns: []string{
			`\b(?:more|highly|most)\s+effective\b`,
			`\bmaximum\s+effectiveness\b`,
		},
	},
	{
		id:       "performance.high_strength",
		category: model.CategoryPerformance,
		patterns: []string{
			`\b(?:high|extra|maximum|double|triple)[\s-]strength\b`,
			`\bextra[\s-]?strong\b`,
		},
	},
	{
		id:       "performance.fast_acting",
		category: model.CategoryPerformance,
		patterns: []string{
			`\bfast[\s-]?acting\b`,
			`\b(?:instant|quick|visible)\s+results?\b`,
		},
	},
	{
		id:       "performance.superior_performance",
		category: model.CategoryPerformance,
		patterns: []string{
			`\b(?:superior|maximum|powerful|unmatched|outstanding)\s+performance\b`,
		},
	},

	{
		id:       "health.supports_function",
		category: model.CategoryHealthWellness,
		patterns: []string{
			`\b(?:supports?|boosts?|strengthens?|improves?|promotes?|aids?|enhances?|maintains?)\s+(?:your\s+|healthy\s+|good\s+|better\s+|natural\s+)?{{FUNC}}\b`,
			`\bhelps?\s+(?:to\s+)?(?:support|boost|maintain|improve|promote)\s+(?:your\s+|healthy\s+|good\s+)?{{FUNC}}\b`,
		},
	},
	{
		id:       "health.good_for",
		category: model.CategoryHealthWellness,
		patterns: []string{
			`\bgood\s+for\s+(?:your\s+)?(?:heart|bones|digestion|immunity|gut|skin|eyes|health)\b`,
			`\bhelps?\s+build\s+(?:strong\s+)?(?:bones|muscles|immunity)\b`,
		},
	},

	{
		id:       "nutrition.source_of",
		category: model.CategoryNutritional,
		patterns: []string{
			`\bsource\s+of\s+{{NUT}}(?:\s*(?:,|&|and)\s*{{NUT}})*`,
		},
	},
	{
		id:       "nutrition.high_in",
		category: model.CategoryNutritional,
		patterns: []string{
			`\b(?:high|rich|packed|loaded)\s+(?:in\s+|with\s+)?{{NUT}}\b`,
			`\b{{NUT}}[\s-]rich\b`,
		},
	},
	{
		id:       "nutrition.low_in",
		category: model.CategoryNutritional,
		patterns: []string{
			`\b(?:low|reduced|zero)\s+(?:in\s+)?{{NUT}}\b`,
			`\bno\s+added\s+{{NUT}}\b`,
			`\b(?:{{NUT}}|gluten|lactose|msg)[\s-]free\b`,
		},
	},

	{
		id:       "comparative.percentage_better",
		category: model.CategoryComparative,
		patterns: []string{
			`\b\d+(?:\.\d+)?\s*%\s+(?:better|more|less|faster|stronger|longer|lighter|cheaper|healthier)(?:\s+\w+)?\s+than\s+{{BASE}}`,
			`\b\d+(?:\.\d+)?\s*(?:x|times)\s+(?:more|better|stronger|faster|longer|higher)\b`,
		},
	},
	{
		id:       "comparative.better_than",
		category: model.CategoryComparative,
		patterns: []string{
			`\b(?:better|stronger|faster|longer[\s-]lasting|more\s+effective|healthier|softer|cleaner|whiter)\s+than\s+{{BASE}}`,
			`\bcompared\s+to\s+{{BASE}}`,
		},
	},
	{
		id:       "comparative.uses_less",
		category: model.CategoryComparative,
		patterns: []string{
			`\buses?\s+(?:up\s+to\s+\d+\s*%\s+)?less\s+(?:energy|water|power|electricity|fuel|detergent|plastic)\b`,
			`\bsaves?\s+(?:up\s+to\s+\d+\s*%\s+)?(?:energy|water|power|electricity|fuel)\b`,
		},
	},

	{
		id:       "environment.eco_friendly",
		category: model.CategoryEnvironmental,
		patterns: []string{
			`\beco[\s-]?friendly\b`,
			`\benvironment(?:ally)?[\s-]friendly\b`,
			`\b(?:earth|planet)[\s-]friendly\b`,
		},
	},
	{
		id:       "environment.recyclable",
		category: model.CategoryEnvironmental,
		patterns: []string{
			`\b(?:100\s*%\s+)?recyclable\b`,
			`\bmade\s+from\s+recycled\s+\w+`,
			`\brecycled\s+(?:material|materials|paper|plastic|packaging)\b`,
		},
	},
	{
		id:       "environment.sustainable",
		category: model.CategoryEnvironmental,
		patterns: []string{
			`\bsustainabl[ey]\b`,
			`\bsustainably\s+sourced\b`,
			`\bethically\s+sourced\b`,
		},
	},
	{
		id:       "environment.biodegradable",
		category: model.CategoryEnvironmental,
		patterns: []string{
			`\bbiodegradable\b`,
			`\bcompostable\b`,
			`\bplastic[\s-]free\b`,
			`\bcarbon[\s-]neutral\b`,
			`\bzero[\s-]waste\b`,
		},
	},

	{
		id:       "safety.safe_for",
		category: model.CategorySafety,
		patterns: []string{
			`\bsafe\s+for\s+(?:children|kids|babies|infants|toddlers|pets|daily\s+use|everyday\s+use|sensitive\s+skin|all\s+skin\s+types|all\s+ages|the\s+whole\s+family)\b`,
			`\b(?:child|kid|baby|skin)[\s-]safe\b`,
		},
	},
	{
		id:       "safety.non_toxic",
		category: model.CategorySafety,
		patterns: []string{
			`\bnon[\s-]?toxic\b`,
			`\btoxin[\s-]free\b`,
			`\bchemical[\s-]free\b`,
			`\b(?:bpa|paraben|sulphate|sulfate|phthalate|lead|mercury)[\s-]free\b`,
		},
	},
	{
		id:       "safety.tested",
		category: model.CategorySafety,
		patterns: []string{
			`\bdermatologically\s+(?:tested|approved)\b`,
			`\b(?:clinically|lab|allergy|patch)\s+tested\b`,
			`\bhypo[\s-]?allergenic\b`,
			`\ballergen[\s-]free\b`,
		},
	},

	{
		id:       "quality.premium",
		category: model.CategoryQuality,
		patterns: []string{
			`\bpremium(?:\s+quality)?\b`,
			`\b(?:superior|finest|top|best|export|highest)[\s-]quality\b`,
			`\bquality\s+(?:assured|guaranteed)\b`,
		},
	},
	{
		id:       "quality.high_grade",
		category: model.CategoryQuality,
		patterns: []string{
			`\bhigh[\s-]grade(?:\s+\w+)?\b`,
			`\bbest[\s-]in[\s-]class\b`,
			`\bgrade[\s-]a\b`,
		},
	},

	{
		id:       "origin.made_in_country",
		category: model.CategoryOrigin,
		patterns: []string{
			`\bmade\s+in\s+(?:the\s+)?{{COUNTRY}}\b`,
			`\bproduct\s+of\s+(?:the\s+)?{{COUNTRY}}\b`,
			`\bproudly\s+(?:made|manufactured)\s+in\s+(?:the\s+)?{{COUNTRY}}\b`,
		},
	},
	{
		// case sensitive, so "made in minutes" is not an origin claim
		id:            "origin.made_in_place",
		category:      model.CategoryOrigin,
		caseSensitive: true,
		patterns: []string{
			`\bMade in [A-Z][a-z]{2,}(?: [A-Z][a-z]{2,})?\b`,
		},
		excludes: []string{
			`(?i)\bmade in (?:small|large|micro|our|house|batches|minutes|seconds)\b`,
			`(?i)\bmade in (?:the\s+)?{{COUNTRY}}\b`,
		},
	},
	{
		id:       "origin.handcrafted",
		category: model.CategoryOrigin,
		patterns: []string{
			`\bhand[\s-]?(?:crafted|made|picked|rolled|woven)\b`,
			`\bartisan(?:al)?(?:\s+made)?\b`,
			`\bcrafted\s+by\s+artisans\b`,
			`\blocally\s+sourced\b`,
			`\bfarm[\s-]fresh\b`,
		},
	},

	{
		id:       "certification.food_safety",
		category: model.CategoryCertification,
		patterns: []string{
			`\bfssai\s*(?:certified|approved|licensed|registered|compliant)\b`,
			`\b(?:haccp|fssc\s*22000|gmp|halal|kosher)\s*(?:certified|approved|compliant)\b`,
			`\b(?:usda|india)\s+organic\s+certified\b`,
			`\bcertified\s+(?:organic|vegan|cruelty[\s-]free|sustainable|halal)\b`,
		},
	},
	{
		id:       "certification.standards",
		category: model.CategoryCertification,
		patterns: []string{
			`\b(?:bis|isi)\s*(?:certified|approved|marked|mark)\b`,
			`\biso(?:\s*\d{4,5}(?::\d{4})?)?[\s-]*(?:certified|compliant|approved)\b`,
			`\bagmark\s*(?:certified|approved)?\b`,
			`\b(?:fda|ce)[\s-](?:approved|certified|marked)\b`,
		},
	},
	{
		id:            "certification.certified_by",
		category:      model.CategoryCertification,
		caseSensitive: true,
		patterns: []string{
			`\b[Cc]ertified by [A-Z][\w&.\-]*(?: [A-Z][\w&.\-]*){0,3}`,
		},
	},

	{
		id:       "awards.award_winning",
		category: model.CategoryAwards,
		patterns: []string{
			`\baward[\s-]winning\b`,
			`\bwinner\s+of\s+(?:the\s+)?[\w&.\-]+(?:\s+[\w&.\-]+){0,4}`,
			`\bwon\s+(?:the\s+)?[\w&.\-]+(?:\s+[\w&.\-]+){0,3}\s+award\b`,
		},
	},
	{
		id:            "awards.endorsed_by",
		category:      model.CategoryAwards,
		caseSensitive: true,
		patterns: []string{
			`\b[Rr]ecogni[sz]ed by [A-Z][\w&.\-]*(?: [A-Z][\w&.\-]*){0,3}`,
			`\b[Ee]ndorsed by [A-Z][\w&.\-]*(?: [A-Z][\w&.\-]*){0,3}`,
		},
	},
	{
		id:       "awards.expert_approved",
		category: model.CategoryAwards,
		patterns: []string{
			`\b(?:recommended|approved)\s+by\s+(?:\d+\s+out\s+of\s+\d+\s+)?(?:doctors|dentists|experts|nutritionists|dermatologists|paediatricians|pediatricians)\b`,
			`\brated\s+(?:#\s*1|no\.?\s*1|number\s+one)\b`,
		},
	},

	{
		id:       "price.best_value",
		category: model.CategoryPriceValue,
		patterns: []string{
			`\bbest\s+value\b`,
			`\bvalue\s+for\s+money\b`,
			`\bunbeatable\s+(?:price|value)\b`,
		},
	},
	{
		id:       "price.lowest_price",
		category: model.CategoryPriceValue,
		patterns: []string{
			`\blowest\s+prices?\b`,
			`\bbest\s+prices?\b`,
			`\bcheapest\b`,
		},
	},
	{
		id:       "price.savings",
		category: model.CategoryPriceValue,
		patterns: []string{
			`\b(?:big|huge|great|maximum)\s+savings?\b`,
			`\bsave\s+(?:up\s+to\s+)?(?:\d+\s*%|(?:rs\.?)\s*\d+)`,
		},
	},

	{
		id:       "authenticity.hundred_percent",
		category: model.CategoryAuthenticity,
		patterns: []string{
			`\b100\s*%\s+(?:original|authentic|genuine|pure|real|natural)\b`,
		},
	},
	{
		id:       "authenticity.authentic",
		category: model.CategoryAuthenticity,
		patterns: []string{
			`\bauthentic\b`,
			`\bgenuine\b`,
		},
	},
	{
		id:       "authenticity.traditional_recipe",
		category: model.CategoryAuthenticity,
		patterns: []string{
			`\b(?:traditional|original|authentic|age[\s-]old|secret|time[\s-]honou?red)\s+recipe\b`,
		},
	},

	{
		id:       "medical.cures_condition",
		category: model.CategoryMedical,
		patterns: []string{
			`\b(?:cures?|curing|treats?|treating|heals?|healing|reverses?|prevents?|relieves?|eliminates?|fights?)\s+(?:your\s+)?{{COND}}\b`,
			`\b(?:remedy|cure)\s+for\s+{{COND}}\b`,
		},
	},
	{
		id:       "medical.therapeutic",
		category: model.CategoryMedical,
		patterns: []string{
			`\b(?:medically|clinically)\s+proven\s+to\s+(?:cure|treat|prevent|reverse)(?:\s+(?:your\s+)?{{COND}})?\b`,
			`\b(?:reduces?|controls?|lowers?)\s+(?:blood\s+sugar|blood\s+pressure|cholesterol|diabetes)\b`,
			`\btherapeutic\s+(?:effect|benefits?|properties)\b`,
			`\bdoctor[\s-]prescribed\b`,
		},
	},
}

type compiledRule struct {
	id       string
	category model.Category
	patterns []*regexp.Regexp
	excludes []*regexp.Regexp
}

var vocabulary = strings.NewReplacer(
	"{{NUT}}", nutrients,
	"{{COND}}", conditions,
	"{{FUNC}}", functions,
	"{{COUNTRY}}", countries,
	"{{BASE}}", baselines,
)

var compiledRules = mustCompile(ruleSet)

func mustCompile(rules []rule) []compiledRule {
	out := make([]compiledRule, 0, len(rules))
	seen := make(map[string]bool, len(rules))

	for _, r := range rules {
		if seen[r.id] {
			panic(fmt.Sprintf("claims: duplicate rule id %q", r.id))
		}
		seen[r.id] = true

		if !r.category.Valid() {
			panic(fmt.Sprintf("claims: rule %q has unknown category %q", r.id, r.category))
		}

		c := compiledRule{id: r.id, category: r.category}
		for _, p := range r.patterns {
			expanded := vocabulary.Replace(p)
			if !r.caseSensitive {
				expanded = "(?i)" + expanded
			}
			re, err := regexp.Compile(expanded)
			if err != nil {
				panic(fmt.Sprintf("claims: rule %q pattern %q: %v", r.id, p, err))
			}
			c.patterns = append(c.patterns, re)
		}
		for _, e := range r.excludes {
			re, err := regexp.Compile(vocabulary.Replace(e))
			if err != nil {
				panic(fmt.Sprintf("claims: rule %q exclude %q: %v", r.id, e, err))
			}
			c.excludes = append(c.excludes, re)
		}
		out = append(out, c)
	}
	return out
}

type RuleInfo struct {
	ID       string         `json:"id"`
	Category model.Category `json:"category"`
	Patterns int            `json:"patterns"`
}

func Rules() []RuleInfo {
	out := make([]RuleInfo, 0, len(compiledRules))
	for _, r := range compiledRules {
		out = append(out, RuleInfo{ID: r.id, Category: r.category, Patterns: len(r.patterns)})
	}
	return out
}
