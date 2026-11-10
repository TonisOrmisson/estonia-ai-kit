package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strings"
	"unicode/utf8"
)

// TSDPayrollCSVInput describes resident payments for the transitional TSD CSV import.
// The period and taxpayer are retained for the operator; the CSV itself does not contain them.
type TSDPayrollCSVInput struct {
	TaxPayerRegCode string             `json:"tax_payer_reg_code"`
	Year            int                `json:"year"`
	Month           int                `json:"month"`
	Payments        []TSDPayrollCSVRow `json:"payments"`
}

type TSDPayrollCSVRow struct {
	PersonCode               string `json:"person_code"`
	Name                     string `json:"name"`
	PaymentType              string `json:"payment_type"`
	Amount                   string `json:"amount"`
	SocialTaxBase            string `json:"social_tax_base,omitempty"` // column 1060; empty means EMTA calculates no social tax
	A1Country                string `json:"a1_country,omitempty"`
	StateSocialTaxDeduction  string `json:"state_social_tax_deduction,omitempty"`
	PriorSocialTaxDeduction  string `json:"prior_social_tax_deduction,omitempty"`
	MinimumSocialTaxIncrease string `json:"minimum_social_tax_increase,omitempty"`
	Allowance610             string `json:"allowance_610,omitempty"`
	Allowance640             string `json:"allowance_640,omitempty"`
	Allowance650             string `json:"allowance_650,omitempty"`
}

func ParseTSDPayrollCSVInput(data []byte) (*TSDPayrollCSVInput, error) {
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	var in TSDPayrollCSVInput
	if err := d.Decode(&in); err != nil {
		return nil, fmt.Errorf("payroll input: %w", err)
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return nil, fmt.Errorf("payroll input: unexpected content after JSON")
	}
	return &in, nil
}

var tsdRegCodeRe = regexp.MustCompile(`^[0-9]{8}$`)
var tsdPersonCodeRe = regexp.MustCompile(`^[0-9]{11}$`)
var tsdTypeRe = regexp.MustCompile(`^[A-Za-z0-9]{1,50}$`)
var tsdCountryRe = regexp.MustCompile(`^[A-Z]{2}$`)

// BuildTSDPayrollCSV creates the official appendix 1, part 1a CSV. Tax amounts
// are intentionally not supplied: EMTA calculates them on import.
func BuildTSDPayrollCSV(in *TSDPayrollCSVInput) ([]byte, error) {
	if in == nil || !tsdRegCodeRe.MatchString(in.TaxPayerRegCode) {
		return nil, fmt.Errorf("payroll input: tax_payer_reg_code must contain 8 digits")
	}
	if in.Month < 1 || in.Month > 12 || in.Year*100+in.Month < 202610 || in.Year*100+in.Month > 202712 {
		return nil, fmt.Errorf("payroll input: CSV transition supports periods 2026-10 through 2027-12")
	}
	if len(in.Payments) == 0 {
		return nil, fmt.Errorf("payroll input: payments must not be empty")
	}
	var b bytes.Buffer
	b.WriteString("\xef\xbb\xbf1000;1010;1020;1030;1050;1060;1070;1080;1090;1100;1110;1120;1130;1140;1170;1160_610;1160_640;1160_650\r\n")
	text := func(s string) string { return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) + `"` }
	seen := make(map[string]bool)
	for i, p := range in.Payments {
		if !tsdPersonCodeRe.MatchString(p.PersonCode) || !tsdTypeRe.MatchString(p.PaymentType) {
			return nil, fmt.Errorf("payment %d: invalid person_code or payment_type", i+1)
		}
		key := p.PersonCode + ":" + p.PaymentType
		if seen[key] {
			return nil, fmt.Errorf("payment %d: duplicate person_code and payment_type; combine monthly payments", i+1)
		}
		seen[key] = true
		if !utf8.ValidString(p.Name) || utf8.RuneCountInString(p.Name) > 500 || strings.TrimSpace(p.Name) == "" || strings.ContainsAny(p.Name, "\r\n\x00") {
			return nil, fmt.Errorf("payment %d: invalid name", i+1)
		}
		if p.A1Country != "" && !tsdCountryRe.MatchString(p.A1Country) {
			return nil, fmt.Errorf("payment %d: a1_country must be a two-letter country code", i+1)
		}
		amounts := []string{p.Amount, p.SocialTaxBase, p.StateSocialTaxDeduction, p.PriorSocialTaxDeduction, p.MinimumSocialTaxIncrease, p.Allowance610, p.Allowance640, p.Allowance650}
		for j, value := range amounts {
			n, err := normAmount(value)
			if err != nil || (j == 0 && n == "") || len(strings.ReplaceAll(strings.ReplaceAll(n, ".", ""), "-", "")) > 22 {
				return nil, fmt.Errorf("payment %d: invalid amount", i+1)
			}
			amounts[j] = strings.ReplaceAll(n, ".", ",")
		}
		fields := []string{text(p.PersonCode), text(p.Name), text(p.PaymentType), amounts[0], "", amounts[1], amounts[2], amounts[3], amounts[4], "", "", "", "", "", "", amounts[5], amounts[6], amounts[7]}
		if p.A1Country != "" {
			fields[4] = text(p.A1Country)
		}
		b.WriteString(strings.Join(fields, ";") + "\r\n")
	}
	return b.Bytes(), nil
}
