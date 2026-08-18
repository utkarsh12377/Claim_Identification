package model

type ClaimStatus string

const (
	ClaimStatusIdentified ClaimStatus = "IDENTIFIED"
)

type Claim struct {
	ID         string      `json:"id"`
	ClaimType  Category    `json:"claimType"`
	ClaimValue string      `json:"claimValue"`
	Status     ClaimStatus `json:"status"`
	Source     string      `json:"source,omitempty"`
	Evidence   string      `json:"evidence,omitempty"`
	RuleID     string      `json:"ruleId,omitempty"`
}
