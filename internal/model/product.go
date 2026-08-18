package model

import (
	"encoding/json"
	"errors"
	"fmt"
)

type Media struct {
	ID        string `json:"id"`
	URL       string `json:"url"`
	MediaType string `json:"mediaType"`
	Path      string `json:"path"`
	SortOrder int    `json:"sortOrder"`
	AltText   string `json:"altText"`
}

type Product struct {
	ID               string         `json:"id"`
	McrID            string         `json:"mcrId"`
	SKU              string         `json:"sku"`
	GtinEan          string         `json:"gtinEan"`
	Title            string         `json:"title"`
	Brand            string         `json:"brand"`
	CategoryID       int            `json:"categoryId"`
	CategoryName     string         `json:"categoryName"`
	ShortDescription string         `json:"shortDescription"`
	LongDescription  string         `json:"longDescription,omitempty"`
	AboutItems       []string       `json:"aboutItems"`
	Status           string         `json:"status"`
	ValidationScores map[string]any `json:"validationScores"`
	ShippingInfo     map[string]any `json:"shippingInfo"`
	ComplianceInfo   map[string]any `json:"complianceInfo"`
	Attributes       map[string]any `json:"attributes"`
	CreatedAt        string         `json:"createdAt"`
	UpdatedAt        string         `json:"updatedAt"`
	Media            []Media        `json:"media"`
	Claims           []Claim        `json:"claims"`
}

var ErrNoProductInDocument = errors.New("document contains no product")

type productEnvelope struct {
	Data struct {
		GetProduct *Product `json:"getProduct"`
	} `json:"data"`
}

func ParseProductDocument(raw []byte) (*Product, error) {
	var env productEnvelope
	if err := json.Unmarshal(raw, &env); err == nil && env.Data.GetProduct != nil {
		return env.Data.GetProduct, nil
	}

	var p Product
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, fmt.Errorf("decode product: %w", err)
	}
	if p.ID == "" {
		return nil, ErrNoProductInDocument
	}
	return &p, nil
}

func (p *Product) Clone() *Product {
	if p == nil {
		return nil
	}
	raw, err := json.Marshal(p)
	if err != nil {
		panic(fmt.Sprintf("clone product: %v", err))
	}
	var out Product
	if err := json.Unmarshal(raw, &out); err != nil {
		panic(fmt.Sprintf("clone product: %v", err))
	}
	return &out
}
