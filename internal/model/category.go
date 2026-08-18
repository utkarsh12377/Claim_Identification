package model

type Category string

const (
	CategoryPerformance    Category = "Performance / Efficacy"
	CategoryHealthWellness Category = "Health & Wellness (Non-Medicinal)"
	CategoryNutritional    Category = "Nutritional Claims"
	CategoryComparative    Category = "Comparative Claims"
	CategoryEnvironmental  Category = "Environmental / Sustainability"
	CategorySafety         Category = "Safety Claims"
	CategoryQuality        Category = "Quality Claims"
	CategoryOrigin         Category = "Manufacturing / Origin"
	CategoryCertification  Category = "Certification Claims"
	CategoryAwards         Category = "Awards & Endorsements"
	CategoryPriceValue     Category = "Price / Value Claims"
	CategoryAuthenticity   Category = "Authenticity Claims"
	CategoryMedical        Category = "Medical / Therapeutic (Restricted)"
)

func Categories() []Category {
	return []Category{
		CategoryPerformance,
		CategoryHealthWellness,
		CategoryNutritional,
		CategoryComparative,
		CategoryEnvironmental,
		CategorySafety,
		CategoryQuality,
		CategoryOrigin,
		CategoryCertification,
		CategoryAwards,
		CategoryPriceValue,
		CategoryAuthenticity,
		CategoryMedical,
	}
}

func (c Category) Restricted() bool {
	return c == CategoryMedical
}

func (c Category) Valid() bool {
	for _, known := range Categories() {
		if c == known {
			return true
		}
	}
	return false
}
