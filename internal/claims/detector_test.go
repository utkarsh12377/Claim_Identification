package claims_test

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/utkarsh/claim-identification/internal/claims"
	"github.com/utkarsh/claim-identification/internal/model"
)

func sequentialIDs() func() string {
	n := 0
	return func() string {
		n++
		return "claim-" + strconv.Itoa(n)
	}
}

func loadSampleProduct(t *testing.T) *model.Product {
	t.Helper()

	raw, err := os.ReadFile(filepath.Join("..", "..", "seed", "product.json"))
	if err != nil {
		t.Fatalf("read seed product: %v", err)
	}
	p, err := model.ParseProductDocument(raw)
	if err != nil {
		t.Fatalf("parse seed product: %v", err)
	}
	return p
}

func TestDetectSampleProduct(t *testing.T) {
	p := loadSampleProduct(t)
	got := claims.New(claims.WithIDFunc(sequentialIDs())).Detect(p)

	want := []struct {
		claimType  model.Category
		claimValue string
		source     string
	}{
		{model.CategoryNutritional, "High Proteins", "title"},
		{model.CategoryNutritional, "Source of Fibre & Iron", "aboutItems[2]"},
		{model.CategorySafety, "Safe for children", "shortDescription"},
		{model.CategoryOrigin, "Made in India", "complianceInfo.country_of_origin"},
		{model.CategoryCertification, "FSSAI certified", "complianceInfo.fssai_no"},
	}

	if len(got) != len(want) {
		for _, c := range got {
			t.Logf("detected %-34s %-24q from %s", c.ClaimType, c.ClaimValue, c.Source)
		}
		t.Fatalf("claim count = %d, want %d", len(got), len(want))
	}

	for i, w := range want {
		c := got[i]
		if c.ClaimType != w.claimType || c.ClaimValue != w.claimValue || c.Source != w.source {
			t.Errorf("claim[%d] = (%s, %q, %s), want (%s, %q, %s)",
				i, c.ClaimType, c.ClaimValue, c.Source, w.claimType, w.claimValue, w.source)
		}
		if c.Status != model.ClaimStatusIdentified {
			t.Errorf("claim[%d].Status = %q, want IDENTIFIED", i, c.Status)
		}
		if c.ID == "" || c.RuleID == "" {
			t.Errorf("claim[%d] missing id (%q) or ruleId (%q)", i, c.ID, c.RuleID)
		}
	}
}

func TestNoFalsePositives(t *testing.T) {
	texts := []string{
		"Masala slow roasted to perfection",
		"Appetizing Aroma & Delicious Taste",
		"Made with 20 Spices & Herbs",
		"Pack contains 1 serve",
		"Maggi Nutri-licious Masala Veg Atta Noodles",
		"instant noodles made with whole wheat flour and blended with 20 spices and herbs",
		"Cook for 3 minutes",
		"STORE IN A COOL, DRY & HYGIENIC PLACE TO PROTECT FROM INSECTS, PESTS & STRONG ODOURS.",
		"Per 100g: Energy 433 kcal, Protein 8.0g, Total Sugars 2.7g, Fiber 5.0g, Iron 3.70mg",

		"A source of inspiration for home cooks",
		"Made in minutes, enjoyed all day",
		"Delicious treats the whole family will love",
		"Not eco-friendly packaging is a thing of the past",
		"Serve hot with a high bowl of soup",
		"Nestle India Limited, 100/101, World Trade Centre, New Delhi",

		"Best before 12 months from the date of packaging",
		"Contains 12 sachets of 5g each",
		"Rich chocolate flavour with a hint of vanilla",
		"Free shipping on orders above Rs 499",
		"No artificial colours or flavours added",
		"Designed for the whole family to enjoy",
		"Low heat setting recommended for best results",
		"Keep away from direct sunlight",
	}

	detector := claims.New(claims.WithIDFunc(sequentialIDs()))
	for _, text := range texts {
		t.Run(text, func(t *testing.T) {
			got := detector.Detect(&model.Product{Title: text})
			for _, c := range got {
				t.Errorf("false positive: %q classified as %s (rule %s)", c.ClaimValue, c.ClaimType, c.RuleID)
			}
		})
	}
}

func TestCategoryMapping(t *testing.T) {
	cases := []struct {
		text string
		want model.Category
	}{
		{"Long-lasting freshness", model.CategoryPerformance},
		{"More effective than before", model.CategoryPerformance},
		{"High-strength formula", model.CategoryPerformance},
		{"Supports immunity every day", model.CategoryHealthWellness},
		{"Promotes digestion naturally", model.CategoryHealthWellness},
		{"High protein snack", model.CategoryNutritional},
		{"Low sugar treat", model.CategoryNutritional},
		{"Source of Fibre & Iron", model.CategoryNutritional},
		{"30% better than ordinary detergent", model.CategoryComparative},
		{"Uses less energy than regular bulbs", model.CategoryComparative},
		{"Eco-friendly packaging", model.CategoryEnvironmental},
		{"Recyclable carton", model.CategoryEnvironmental},
		{"Sustainable sourcing", model.CategoryEnvironmental},
		{"Safe for children", model.CategorySafety},
		{"Non-toxic crayons", model.CategorySafety},
		{"Dermatologically tested cream", model.CategorySafety},
		{"Premium quality cotton", model.CategoryQuality},
		{"High-grade materials", model.CategoryQuality},
		{"Made in India", model.CategoryOrigin},
		{"Handcrafted by weavers", model.CategoryOrigin},
		{"Artisan made pottery", model.CategoryOrigin},
		{"FSSAI certified kitchen", model.CategoryCertification},
		{"BIS approved helmet", model.CategoryCertification},
		{"ISO 9001 compliant plant", model.CategoryCertification},
		{"Award-winning blend", model.CategoryAwards},
		{"Recognized by Nutrition Council", model.CategoryAwards},
		{"Best value pack", model.CategoryPriceValue},
		{"Lowest price guaranteed", model.CategoryPriceValue},
		{"Big savings this season", model.CategoryPriceValue},
		{"100% original spices", model.CategoryAuthenticity},
		{"Authentic taste", model.CategoryAuthenticity},
		{"Traditional recipe from Kerala", model.CategoryAuthenticity},
		{"Cures diabetes in 30 days", model.CategoryMedical},
		{"Treats arthritis pain", model.CategoryMedical},
	}

	detector := claims.New(claims.WithIDFunc(sequentialIDs()))
	for _, tc := range cases {
		t.Run(tc.text, func(t *testing.T) {
			got := detector.Detect(&model.Product{Title: tc.text})
			if len(got) == 0 {
				t.Fatalf("no claim detected in %q", tc.text)
			}
			found := false
			for _, c := range got {
				if c.ClaimType == tc.want {
					found = true
				}
			}
			if !found {
				t.Errorf("detected %v, want a %s claim", categoriesOf(got), tc.want)
			}
		})
	}
}

func categoriesOf(cs []model.Claim) []string {
	out := make([]string, 0, len(cs))
	for _, c := range cs {
		out = append(out, string(c.ClaimType)+":"+c.ClaimValue)
	}
	return out
}

func TestDeduplication(t *testing.T) {
	p := &model.Product{
		AboutItems:       []string{"Source of Fibre & Iron"},
		ShortDescription: "It is a source of fibre and iron.",
	}

	got := claims.New(claims.WithIDFunc(sequentialIDs())).Detect(p)
	if len(got) != 1 {
		t.Fatalf("got %d claims, want 1: %v", len(got), categoriesOf(got))
	}
	if got[0].ClaimValue != "Source of Fibre & Iron" {
		t.Errorf("claimValue = %q, want %q", got[0].ClaimValue, "Source of Fibre & Iron")
	}
	if got[0].Source != "aboutItems[0]" {
		t.Errorf("source = %q, want aboutItems[0]", got[0].Source)
	}
}

func TestRestrictedClaimWording(t *testing.T) {
	got := claims.New(claims.WithIDFunc(sequentialIDs())).
		Detect(&model.Product{Title: "Clinically proven to cure diabetes and treats arthritis"})

	want := []string{"Clinically proven to cure diabetes", "treats arthritis"}
	if len(got) != len(want) {
		t.Fatalf("got %d claims, want %d: %v", len(got), len(want), categoriesOf(got))
	}
	for i, w := range want {
		if got[i].ClaimValue != w {
			t.Errorf("claim[%d] = %q, want %q", i, got[i].ClaimValue, w)
		}
		if !got[i].ClaimType.Restricted() {
			t.Errorf("claim[%d] type = %s, want a restricted category", i, got[i].ClaimType)
		}
	}
}

func TestOverlapResolution(t *testing.T) {
	got := claims.New(claims.WithIDFunc(sequentialIDs())).
		Detect(&model.Product{Title: "Made with an authentic recipe"})

	if len(got) != 1 {
		t.Fatalf("got %d claims, want 1: %v", len(got), categoriesOf(got))
	}
	if got[0].ClaimValue != "authentic recipe" {
		t.Errorf("claimValue = %q, want %q", got[0].ClaimValue, "authentic recipe")
	}
}

func TestStructuredFieldsDisabled(t *testing.T) {
	p := loadSampleProduct(t)
	got := claims.New(
		claims.WithIDFunc(sequentialIDs()),
		claims.WithStructuredFields(false),
	).Detect(p)

	if len(got) != 3 {
		t.Fatalf("got %d claims, want 3: %v", len(got), categoriesOf(got))
	}
	for _, c := range got {
		if c.ClaimType == model.CategoryOrigin || c.ClaimType == model.CategoryCertification {
			t.Errorf("structured claim %q leaked while disabled", c.ClaimValue)
		}
	}
}

func TestStructuredFieldsRejectPlaceholders(t *testing.T) {
	p := &model.Product{
		ComplianceInfo: map[string]any{
			"country_of_origin": "N/A",
			"fssai_no":          "",
		},
	}

	if got := claims.New().Detect(p); len(got) != 0 {
		t.Errorf("got %d claims from placeholder compliance data, want 0: %v", len(got), categoriesOf(got))
	}
}

func TestNilProduct(t *testing.T) {
	if got := claims.New().Detect(nil); got != nil {
		t.Errorf("Detect(nil) = %v, want nil", got)
	}
}

func TestRulesCoverEveryCategory(t *testing.T) {
	covered := make(map[model.Category]int)
	for _, r := range claims.Rules() {
		covered[r.Category]++
	}
	for _, c := range model.Categories() {
		if covered[c] == 0 {
			t.Errorf("no rule covers category %s", c)
		}
	}
}
