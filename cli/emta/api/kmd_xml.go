package api

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

// KMDDeclarationInput is the single source of truth for a KMD draft: it is
// rendered to the official EMTA vatdeclaration.xsd XML for upload and is the
// expected state for live verification.
type KMDDeclarationInput struct {
	TaxPayerRegCode string            `json:"tax_payer_reg_code"`
	Year            int               `json:"year"`
	Month           int               `json:"month"`
	Main            map[string]string `json:"main"`
	INFA            []KMDINFARow      `json:"inf_a,omitempty"`
	INFB            []KMDINFBRow      `json:"inf_b,omitempty"`
}

// kmdMainLine maps an input key to its XSD element and the live form field.
type kmdMainLine struct {
	Key   string
	XML   string
	Field func(*KMDMainFields) *string
	Int   bool
}

// Order follows the DeclarationBody sequence in vatdeclaration.xsd (KMD6).
var kmdMainLines = []kmdMainLine{
	{"line_1", "transactions24", func(f *KMDMainFields) *string { return &f.TransactionsWithRate24 }, false},
	{"line_1_2", "transactions22", func(f *KMDMainFields) *string { return &f.TransactionsWithRate22 }, false},
	{"line_1_1", "transactions20", func(f *KMDMainFields) *string { return &f.TransactionsWithRate20 }, false},
	{"line_2", "transactions9", func(f *KMDMainFields) *string { return &f.TransactionsWithRate9 }, false},
	{"line_2_1", "transactions5", func(f *KMDMainFields) *string { return &f.TransactionsWithRate5 }, false},
	{"line_2_2", "transactions13", func(f *KMDMainFields) *string { return &f.TransactionsWithRate13 }, false},
	{"line_3", "transactionsZeroVat", func(f *KMDMainFields) *string { return &f.TransactionsZeroVAT }, false},
	{"line_3_1", "euSupplyInclGoodsAndServicesZeroVat", func(f *KMDMainFields) *string { return &f.EUSupplyInclGoods }, false},
	{"line_3_1_1", "euSupplyGoodsZeroVat", func(f *KMDMainFields) *string { return &f.EUSupplyGoods }, false},
	{"line_3_2", "exportZeroVat", func(f *KMDMainFields) *string { return &f.ExportZeroVAT }, false},
	{"line_3_2_1", "salePassengersWithReturnVat", func(f *KMDMainFields) *string { return &f.SalePassengersReturn }, false},
	{"line_5", "inputVatTotal", func(f *KMDMainFields) *string { return &f.InputVATTotal }, false},
	{"line_5_1", "importVat", func(f *KMDMainFields) *string { return &f.ImportVAT }, false},
	{"line_5_2", "fixedAssetsVat", func(f *KMDMainFields) *string { return &f.FixedAssetsVAT }, false},
	{"line_5_3", "carsVat", func(f *KMDMainFields) *string { return &f.CarsVAT }, false},
	{"line_5_3_cars", "numberOfCars", func(f *KMDMainFields) *string { return &f.NumberOfCars }, true},
	{"line_5_4", "carsPartialVat", func(f *KMDMainFields) *string { return &f.CarsPartialVAT }, false},
	{"line_5_4_cars", "numberOfCarsPartial", func(f *KMDMainFields) *string { return &f.NumberOfCarsPartial }, true},
	{"line_6", "euAcquisitionsGoodsAndServicesTotal", func(f *KMDMainFields) *string { return &f.EUAcquisitionsTotal }, false},
	{"line_6_1", "euAcquisitionsGoods", func(f *KMDMainFields) *string { return &f.EUAcquisitionsGoods }, false},
	{"line_7", "acquisitionOtherGoodsAndServicesTotal", func(f *KMDMainFields) *string { return &f.OtherGoodsTotal }, false},
	{"line_7_1", "acquisitionImmovablesAndScrapMetalAndGold", func(f *KMDMainFields) *string { return &f.ImmovablesAndMetal }, false},
	{"line_8", "supplyExemptFromTax", func(f *KMDMainFields) *string { return &f.ExemptSupply }, false},
	{"line_9", "supplySpecialArrangements", func(f *KMDMainFields) *string { return &f.SpecialArrangements }, false},
	{"line_10", "adjustmentsPlus", func(f *KMDMainFields) *string { return &f.AdjustmentsPlus }, false},
	{"line_11", "adjustmentsMinus", func(f *KMDMainFields) *string { return &f.AdjustmentsMinus }, false},
}

var (
	amountRe  = regexp.MustCompile(`^-?\d+(\.\d{1,2})?$`)
	intRe     = regexp.MustCompile(`^\d+$`)
	taxRateRe = regexp.MustCompile(`^\d+(erikord)?$`)
	regCodeRe = regexp.MustCompile(`^[0-9A-Za-z]{1,20}$`)
)

// ParseKMDDeclarationInput decodes the input strictly so typos in keys fail.
func ParseKMDDeclarationInput(data []byte) (*KMDDeclarationInput, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var in KMDDeclarationInput
	if err := dec.Decode(&in); err != nil {
		return nil, fmt.Errorf("kmd input: %w", err)
	}
	if _, err := BuildKMDXML(&in); err != nil {
		return nil, err
	}
	return &in, nil
}

// KMDVersion returns the XSD version value required for the tax period.
func KMDVersion(year, month int) (string, error) {
	p := year*100 + month
	switch {
	case p >= 202507:
		return "KMD6", nil
	case p >= 202501:
		return "KMD5", nil
	case p >= 202401:
		return "KMD4", nil
	}
	return "", fmt.Errorf("kmd xml: period %04d-%02d not supported (KMD4+ only)", year, month)
}

// normAmount converts "7 999,65" / "7999.65" / "" to canonical "7999.65" ("" stays "").
func normAmount(s string) (string, error) {
	s = strings.NewReplacer(" ", "", " ", "", ",", ".").Replace(strings.TrimSpace(s))
	if s == "" {
		return "", nil
	}
	if !amountRe.MatchString(s) {
		return "", fmt.Errorf("invalid amount %q", s)
	}
	if !strings.Contains(s, ".") {
		s += ".00"
	} else if len(s)-strings.Index(s, ".") == 2 {
		s += "0"
	}
	return s, nil
}

func normDate(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", nil
	}
	for _, layout := range []string{"2006-01-02", "02.01.2006"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.Format("2006-01-02"), nil
		}
	}
	return "", fmt.Errorf("invalid date %q", s)
}

// normTaxRate maps form labels like "24%" or "Erikord 24%" to XSD values "24" / "24erikord".
func normTaxRate(s string) string {
	s = strings.ToLower(strings.NewReplacer("%", "", " ", "", " ", "").Replace(s))
	if strings.HasPrefix(s, "erikord") {
		s = strings.TrimPrefix(s, "erikord") + "erikord"
	}
	return s
}

func singleComment(codes []string) (string, error) {
	if len(codes) > 1 {
		return "", fmt.Errorf("only one comment code per line is supported, got %v", codes)
	}
	if len(codes) == 1 {
		return strings.TrimSpace(codes[0]), nil
	}
	return "", nil
}

// BuildKMDXML validates the input and renders it as vatdeclaration.xsd XML.
// noSales/noPurchases are derived from INF A/B rows so they cannot contradict them.
func BuildKMDXML(in *KMDDeclarationInput) ([]byte, error) {
	if !regCodeRe.MatchString(in.TaxPayerRegCode) {
		return nil, fmt.Errorf("kmd input: tax_payer_reg_code is required")
	}
	if in.Month < 1 || in.Month > 12 || in.Year < 1000 || in.Year > 9999 {
		return nil, fmt.Errorf("kmd input: invalid period %d-%d", in.Year, in.Month)
	}
	version, err := KMDVersion(in.Year, in.Month)
	if err != nil {
		return nil, err
	}
	known := map[string]bool{}
	for _, l := range kmdMainLines {
		known[l.Key] = true
	}
	for k := range in.Main {
		if !known[k] {
			return nil, fmt.Errorf("kmd input: unknown main key %q", k)
		}
	}

	var b bytes.Buffer
	el := func(indent, name, value string) {
		b.WriteString(indent + "<" + name + ">")
		xml.EscapeText(&b, []byte(value))
		b.WriteString("</" + name + ">\n")
	}
	b.WriteString(xml.Header + "<vatDeclaration>\n")
	el("  ", "taxPayerRegCode", in.TaxPayerRegCode)
	el("  ", "year", fmt.Sprintf("%04d", in.Year))
	el("  ", "month", fmt.Sprintf("%02d", in.Month))
	el("  ", "declarationType", "1")
	el("  ", "version", version)
	b.WriteString("  <declarationBody>\n")
	el("    ", "noSales", fmt.Sprint(len(in.INFA) == 0))
	el("    ", "noPurchases", fmt.Sprint(len(in.INFB) == 0))
	el("    ", "sumPerPartnerSales", "false")
	el("    ", "sumPerPartnerPurchases", "false")
	for _, l := range kmdMainLines {
		raw, ok := in.Main[l.Key]
		if !ok || strings.TrimSpace(raw) == "" {
			continue
		}
		v := strings.TrimSpace(raw)
		if l.Int {
			if !intRe.MatchString(v) {
				return nil, fmt.Errorf("kmd input: %s must be an integer, got %q", l.Key, raw)
			}
		} else if v, err = normAmount(v); err != nil {
			return nil, fmt.Errorf("kmd input: %s: %w", l.Key, err)
		}
		if (l.Key == "line_10" || l.Key == "line_11") && strings.HasPrefix(v, "-") {
			return nil, fmt.Errorf("kmd input: %s must not be negative", l.Key)
		}
		el("    ", l.XML, v)
	}
	b.WriteString("  </declarationBody>\n")

	if len(in.INFA) > 0 {
		b.WriteString("  <salesAnnex>\n")
		for i, r := range in.INFA {
			sum, err := normAmount(r.InvoiceSum)
			if err != nil || sum == "" {
				return nil, fmt.Errorf("kmd input: inf_a[%d].invoice_sum required: %v", i, err)
			}
			forRate, err := normAmount(r.SumForRateInPeriod)
			if err != nil {
				return nil, fmt.Errorf("kmd input: inf_a[%d].sum_for_rate_in_period: %w", i, err)
			}
			date, err := normDate(r.InvoiceDate)
			if err != nil {
				return nil, fmt.Errorf("kmd input: inf_a[%d]: %w", i, err)
			}
			rate := normTaxRate(r.TaxRate)
			if !taxRateRe.MatchString(rate) {
				return nil, fmt.Errorf("kmd input: inf_a[%d].tax_rate invalid %q", i, r.TaxRate)
			}
			if r.PartnerCode == "" && r.PartnerName == "" {
				return nil, fmt.Errorf("kmd input: inf_a[%d] needs partner_code or partner_name", i)
			}
			comment, err := singleComment(r.CommentCodes)
			if err != nil {
				return nil, fmt.Errorf("kmd input: inf_a[%d]: %w", i, err)
			}
			b.WriteString("    <saleLine>\n")
			for _, kv := range [][2]string{{"buyerRegCode", r.PartnerCode}, {"buyerName", r.PartnerName}, {"invoiceNumber", r.InvoiceNumber}, {"invoiceDate", date}, {"invoiceSum", sum}, {"taxRate", rate}, {"sumForRateInPeriod", forRate}, {"comments", comment}} {
				if kv[1] != "" {
					el("      ", kv[0], kv[1])
				}
			}
			b.WriteString("    </saleLine>\n")
		}
		b.WriteString("  </salesAnnex>\n")
	}

	if len(in.INFB) > 0 {
		b.WriteString("  <purchasesAnnex>\n")
		for i, r := range in.INFB {
			sum, err := normAmount(r.InvoiceSumVAT)
			if err != nil || sum == "" {
				return nil, fmt.Errorf("kmd input: inf_b[%d].invoice_sum_vat required: %v", i, err)
			}
			vat, err := normAmount(r.VATInPeriod)
			if err != nil || vat == "" {
				return nil, fmt.Errorf("kmd input: inf_b[%d].vat_in_period required: %v", i, err)
			}
			date, err := normDate(r.InvoiceDate)
			if err != nil {
				return nil, fmt.Errorf("kmd input: inf_b[%d]: %w", i, err)
			}
			if r.PartnerCode == "" && r.PartnerName == "" {
				return nil, fmt.Errorf("kmd input: inf_b[%d] needs partner_code or partner_name", i)
			}
			comment, err := singleComment(r.CommentCodes)
			if err != nil {
				return nil, fmt.Errorf("kmd input: inf_b[%d]: %w", i, err)
			}
			b.WriteString("    <purchaseLine>\n")
			for _, kv := range [][2]string{{"sellerRegCode", r.PartnerCode}, {"sellerName", r.PartnerName}, {"invoiceNumber", r.InvoiceNumber}, {"invoiceDate", date}, {"invoiceSumVat", sum}, {"vatInPeriod", vat}, {"comments", comment}} {
				if kv[1] != "" {
					el("      ", kv[0], kv[1])
				}
			}
			b.WriteString("    </purchaseLine>\n")
		}
		b.WriteString("  </purchasesAnnex>\n")
	}
	b.WriteString("</vatDeclaration>\n")
	return b.Bytes(), nil
}

// KMDUploadError is one row of the "Failide laadimisel tekkinud vead" table.
type KMDUploadError struct {
	UploadedAt  string `json:"uploaded_at"`
	FileName    string `json:"file_name"`
	Description string `json:"description"`
}

func parseKMDUploadErrors(html string) ([]KMDUploadError, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil, err
	}
	var out []KMDUploadError
	doc.Find("table").Each(func(_ int, table *goquery.Selection) {
		if table.Find("table").Length() > 0 || !strings.Contains(table.Text(), "Vea kirjeldus") {
			return
		}
		table.Find("tr").Each(func(_ int, row *goquery.Selection) {
			cells := row.Find("td")
			if cells.Length() < 3 {
				return
			}
			out = append(out, KMDUploadError{
				UploadedAt:  cleanText(cells.Eq(0).Text()),
				FileName:    cleanText(cells.Eq(1).Text()),
				Description: cleanText(cells.Eq(2).Text()),
			})
		})
	})
	return out, nil
}

var (
	kmdTaxPayerRe = regexp.MustCompile(`Maksukohustuslane:\s*([0-9A-Za-z]+)`)
	kmdYearRe     = regexp.MustCompile(`Aasta:\s*(\d{4})`)
	kmdMonthRe    = regexp.MustCompile(`Kuu:\s*(\d{1,2})`)
)

// checkKMDDeclarationHeader refuses to touch a declaration of another taxpayer or period.
func checkKMDDeclarationHeader(html string, in *KMDDeclarationInput) error {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return err
	}
	text := cleanText(doc.Text())
	get := func(re *regexp.Regexp) string {
		if m := re.FindStringSubmatch(text); m != nil {
			return m[1]
		}
		return ""
	}
	payer, year, month := get(kmdTaxPayerRe), get(kmdYearRe), get(kmdMonthRe)
	if payer == "" || year == "" || month == "" {
		return fmt.Errorf("kmd declaration header (taxpayer/period) not found on page")
	}
	if payer != in.TaxPayerRegCode || year != fmt.Sprint(in.Year) || strings.TrimLeft(month, "0") != fmt.Sprint(in.Month) {
		return fmt.Errorf("kmd declaration is for taxpayer %s period %s-%s, input is %s %d-%d", payer, year, month, in.TaxPayerRegCode, in.Year, in.Month)
	}
	return nil
}

// ImportKMDXML uploads the declaration XML into an existing draft and fails
// unless EMTA's upload error table stays free of this upload.
func (c *Client) ImportKMDXML(declarationID string, in *KMDDeclarationInput) (*XMLImportResult, error) {
	xmlBytes, err := BuildKMDXML(in)
	if err != nil {
		return nil, err
	}
	page, err := c.openKMDDeclaration(declarationID)
	if err != nil {
		return nil, err
	}
	if err := checkKMDDeclarationHeader(page.HTML, in); err != nil {
		return nil, err
	}
	if status := parseKMDStatus(page.HTML); strings.EqualFold(status, "Esitatud") {
		return nil, fmt.Errorf("kmd declaration %s is already submitted", declarationID)
	}

	// Unique name so an error row can be attributed to this upload.
	fileName := fmt.Sprintf("kmd-%s-%04d-%02d-%s.xml", in.TaxPayerRegCode, in.Year, in.Month, time.Now().Format("150405"))
	result, err := c.ImportKMDFile(declarationID, fileName, xmlBytes)
	if err != nil {
		return nil, err
	}

	after, err := c.waitKMDDeclaration(declarationID, in.Year, in.Month)
	if err != nil {
		return nil, fmt.Errorf("kmd upload sent but result could not be checked: %w", err)
	}
	errs, err := parseKMDUploadErrors(after.HTML)
	if err != nil {
		return nil, fmt.Errorf("kmd upload sent but error table could not be parsed: %w", err)
	}
	var mine []string
	for _, e := range errs {
		if e.FileName == fileName {
			mine = append(mine, e.Description)
		}
	}
	if len(mine) > 0 {
		return nil, fmt.Errorf("EMTA rejected %s: %s", fileName, strings.Join(mine, "; "))
	}
	result.RawRedirect = fileName
	return result, nil
}

var kmdProcessingPoll = 3 * time.Second

// waitKMDDeclaration waits until EMTA has processed the upload queue: while a
// file is processed the draft is listed under another status, and upload
// errors appear only afterwards.
func (c *Client) waitKMDDeclaration(declarationID string, year, month int) (*kmdPage, error) {
	var last string
	for i := 0; i < 40; i++ {
		page, err := c.openKMDDeclaration(declarationID)
		if err == nil {
			return page, nil
		}
		last = err.Error()
		if items, listErr := c.ListKMDDeclarations(); listErr == nil {
			for _, item := range items {
				if item.Year == year && item.Month == month {
					last += fmt.Sprintf("; listed as %q", item.Status)
				}
			}
		}
		time.Sleep(kmdProcessingPoll)
	}
	return nil, fmt.Errorf("declaration not back in list after %s: %s", 40*kmdProcessingPoll, last)
}

// KMDVerifyResult compares a live draft against the expected input.
type KMDVerifyResult struct {
	DeclarationID string          `json:"declaration_id"`
	OK            bool            `json:"ok"`
	Differences   []string        `json:"differences,omitempty"`
	Main          *KMDMainSection `json:"main,omitempty"`
	INFA          []KMDINFARow    `json:"inf_a,omitempty"`
	INFB          []KMDINFBRow    `json:"inf_b,omitempty"`
}

// VerifyKMD reads main form, INF A and INF B live from EMTA (never from a cache).
func (c *Client) VerifyKMD(declarationID string, in *KMDDeclarationInput) (*KMDVerifyResult, error) {
	if _, err := BuildKMDXML(in); err != nil {
		return nil, err
	}
	page, err := c.openKMDDeclaration(declarationID)
	if err != nil {
		return nil, err
	}
	if err := checkKMDDeclarationHeader(page.HTML, in); err != nil {
		return nil, err
	}
	main, err := c.ReadKMDMain(declarationID)
	if err != nil {
		return nil, fmt.Errorf("live main read: %w", err)
	}
	infA, err := c.ReadKMDINFA(declarationID)
	if err != nil {
		return nil, fmt.Errorf("live INF A read: %w", err)
	}
	infB, err := c.ReadKMDINFB(declarationID)
	if err != nil {
		return nil, fmt.Errorf("live INF B read: %w", err)
	}
	res := &KMDVerifyResult{DeclarationID: declarationID, Main: main, INFA: infA.Rows, INFB: infB.Rows}
	res.Differences = compareKMD(in, main, infA.Rows, infB.Rows)
	res.OK = len(res.Differences) == 0
	return res, nil
}

func amountOrZero(s string) string {
	v, err := normAmount(s)
	if err != nil {
		return "invalid:" + s
	}
	if v == "" || v == "0.00" || v == "-0.00" {
		return "0.00"
	}
	return v
}

func compareKMD(in *KMDDeclarationInput, main *KMDMainSection, infA []KMDINFARow, infB []KMDINFBRow) []string {
	var diffs []string
	if want := len(in.INFA) == 0; main.Flags.NoSales != want {
		diffs = append(diffs, fmt.Sprintf("no_sales: want %v, live %v", want, main.Flags.NoSales))
	}
	if want := len(in.INFB) == 0; main.Flags.NoPurchases != want {
		diffs = append(diffs, fmt.Sprintf("no_purchases: want %v, live %v", want, main.Flags.NoPurchases))
	}
	live := main.Fields
	for _, l := range kmdMainLines {
		want, got := in.Main[l.Key], *l.Field(&live)
		if l.Int {
			w, g := strings.TrimLeft(strings.TrimSpace(want), "0"), strings.TrimLeft(strings.TrimSpace(got), "0")
			if w != g {
				diffs = append(diffs, fmt.Sprintf("%s: want %q, live %q", l.Key, want, got))
			}
			continue
		}
		if amountOrZero(want) != amountOrZero(got) {
			diffs = append(diffs, fmt.Sprintf("%s: want %q, live %q", l.Key, want, got))
		}
	}

	keyA := func(r KMDINFARow) string {
		d, _ := normDate(r.InvoiceDate)
		c, _ := singleComment(r.CommentCodes)
		return strings.Join([]string{r.PartnerCode, r.InvoiceNumber, d, amountOrZero(r.InvoiceSum), normTaxRate(r.TaxRate), amountOrZero(r.SumForRateInPeriod), c}, "|")
	}
	keyB := func(r KMDINFBRow) string {
		d, _ := normDate(r.InvoiceDate)
		c, _ := singleComment(r.CommentCodes)
		return strings.Join([]string{r.PartnerCode, r.InvoiceNumber, d, amountOrZero(r.InvoiceSumVAT), amountOrZero(r.VATInPeriod), c}, "|")
	}
	diffs = append(diffs, diffRows("inf_a", mapRows(in.INFA, keyA), mapRows(infA, keyA))...)
	diffs = append(diffs, diffRows("inf_b", mapRows(in.INFB, keyB), mapRows(infB, keyB))...)
	return diffs
}

func mapRows[T any](rows []T, key func(T) string) []string {
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, key(r))
	}
	sort.Strings(out)
	return out
}

func diffRows(name string, want, got []string) []string {
	count := map[string]int{}
	for _, k := range want {
		count[k]++
	}
	for _, k := range got {
		count[k]--
	}
	var diffs []string
	for k, n := range count {
		switch {
		case n > 0:
			diffs = append(diffs, fmt.Sprintf("%s: missing live row %s (x%d)", name, k, n))
		case n < 0:
			diffs = append(diffs, fmt.Sprintf("%s: unexpected live row %s (x%d)", name, k, -n))
		}
	}
	sort.Strings(diffs)
	return diffs
}

// ReadKMDUploadErrors lists EMTA's file upload error table for a declaration.
func (c *Client) ReadKMDUploadErrors(declarationID string) ([]KMDUploadError, error) {
	page, err := c.openKMDDeclaration(declarationID)
	if err != nil {
		return nil, err
	}
	return parseKMDUploadErrors(page.HTML)
}
