package api

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func sampleKMDInput() *KMDDeclarationInput {
	return &KMDDeclarationInput{
		TaxPayerRegCode: "11234567",
		Year:            2026,
		Month:           9,
		Main: map[string]string{
			"line_1":   "7999.65",
			"line_5":   "423,91",
			"line_6":   "602.77",
			"line_6_1": "400.88",
			"line_7":   "43.96",
		},
		INFA: []KMDINFARow{{
			PartnerCode: "10000000", PartnerName: "Example AS", InvoiceNumber: "INV-1",
			InvoiceDate: "2026-09-30", InvoiceSum: "7999.65", TaxRate: "24", SumForRateInPeriod: "7999.65",
		}},
	}
}

// validateAgainstXSD checks XML with the official EMTA schema via xmllint.
func validateAgainstXSD(t *testing.T, xmlBytes []byte) {
	t.Helper()
	if _, err := exec.LookPath("xmllint"); err != nil {
		t.Skip("xmllint not installed")
	}
	path := filepath.Join(t.TempDir(), "kmd.xml")
	if err := os.WriteFile(path, xmlBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command("xmllint", "--noout", "--schema", "../testdata/kmd/vatdeclaration.xsd", path).CombinedOutput()
	if err != nil {
		t.Fatalf("XSD validation failed: %v\n%s\n%s", err, out, xmlBytes)
	}
}

func TestOfficialExampleValidatesAgainstXSD(t *testing.T) {
	data, err := os.ReadFile("../testdata/kmd/vatdeclaration_example.xml")
	if err != nil {
		t.Fatal(err)
	}
	validateAgainstXSD(t, data)
}

func TestBuildKMDXMLValidatesAgainstXSD(t *testing.T) {
	xmlBytes, err := BuildKMDXML(sampleKMDInput())
	if err != nil {
		t.Fatal(err)
	}
	validateAgainstXSD(t, xmlBytes)
	s := string(xmlBytes)
	for _, want := range []string{
		"<version>KMD6</version>",
		"<month>09</month>",
		"<noSales>false</noSales>",
		"<noPurchases>true</noPurchases>",
		"<inputVatTotal>423.91</inputVatTotal>",
		"<euAcquisitionsGoodsAndServicesTotal>602.77</euAcquisitionsGoodsAndServicesTotal>",
		"<buyerRegCode>10000000</buyerRegCode>",
	} {
		if !strings.Contains(s, want) {
			t.Fatalf("missing %s in\n%s", want, s)
		}
	}
	if strings.Contains(s, "purchasesAnnex") {
		t.Fatal("empty INF B must not produce purchasesAnnex")
	}
}

func TestBuildKMDXMLWithINFBValidatesAgainstXSD(t *testing.T) {
	in := sampleKMDInput()
	in.INFB = []KMDINFBRow{{PartnerCode: "12345678", PartnerName: "Seller OÜ", InvoiceNumber: "B-1", InvoiceDate: "01.09.2026", InvoiceSumVAT: "1240", VATInPeriod: "240", CommentCodes: []string{"11"}}}
	xmlBytes, err := BuildKMDXML(in)
	if err != nil {
		t.Fatal(err)
	}
	validateAgainstXSD(t, xmlBytes)
	if !strings.Contains(string(xmlBytes), "<noPurchases>false</noPurchases>") || !strings.Contains(string(xmlBytes), "<invoiceDate>2026-09-01</invoiceDate>") {
		t.Fatalf("unexpected XML:\n%s", xmlBytes)
	}
}

func TestBuildKMDXMLRejectsBadInput(t *testing.T) {
	cases := map[string]func(*KMDDeclarationInput){
		"unknown key":     func(in *KMDDeclarationInput) { in.Main["line_6.1"] = "1" },
		"three decimals":  func(in *KMDDeclarationInput) { in.Main["line_1"] = "1.234" },
		"missing payer":   func(in *KMDDeclarationInput) { in.TaxPayerRegCode = "" },
		"old period":      func(in *KMDDeclarationInput) { in.Year = 2023 },
		"bad rate":        func(in *KMDDeclarationInput) { in.INFA[0].TaxRate = "abc" },
		"bad date":        func(in *KMDDeclarationInput) { in.INFA[0].InvoiceDate = "30/09/2026" },
		"negative adjust": func(in *KMDDeclarationInput) { in.Main["line_10"] = "-1" },
	}
	for name, mutate := range cases {
		in := sampleKMDInput()
		mutate(in)
		if _, err := BuildKMDXML(in); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
}

func TestParseKMDDeclarationInputRejectsUnknownFields(t *testing.T) {
	if _, err := ParseKMDDeclarationInput([]byte(`{"tax_payer_reg_code":"1","year":2026,"month":9,"main":{},"no_purchases":true}`)); err == nil {
		t.Fatal("expected unknown top-level field to fail")
	}
}

func TestKMDVersion(t *testing.T) {
	for p, want := range map[[2]int]string{{2025, 7}: "KMD6", {2025, 6}: "KMD5", {2024, 12}: "KMD4"} {
		if got, _ := KMDVersion(p[0], p[1]); got != want {
			t.Errorf("%v: got %s want %s", p, got, want)
		}
	}
}

// Mirrors the e-MTA declaration page header and upload error table.
const kmdPageWithUploadErrors = `<html><body>
<table><tr><td>Vorm KMD</td><td>Maksukohustuslane: 11234567 Example OÜ</td><td>Periood: Aasta: 2026 Kuu: 9</td><td>Staatus: Esitamata</td></tr></table>
<div><span>Failide laadimisel tekkinud vead</span>
<table>
<tr><th>Üles laaditud</th><th>Faili nimi</th><th>Vea kirjeldus</th><th></th></tr>
<tr><td>19.10.2026 14:55:02</td><td>kmd-main.csv</td><td>Lubatud on laadida ainult spetsifikatsioonile vastavaid xml-, csv- ja zip-faile</td><td><a href="#">x</a></td></tr>
<tr><td>19.10.2026 15:40:11</td><td>kmd-11234567-2026-09-154010.xml</td><td>Vigane väärtus</td><td></td></tr>
</table></div></body></html>`

func TestParseKMDUploadErrors(t *testing.T) {
	errs, err := parseKMDUploadErrors(kmdPageWithUploadErrors)
	if err != nil {
		t.Fatal(err)
	}
	if len(errs) != 2 || errs[0].FileName != "kmd-main.csv" || !strings.Contains(errs[0].Description, "spetsifikatsioonile") || errs[1].UploadedAt != "19.10.2026 15:40:11" {
		t.Fatalf("unexpected errors: %+v", errs)
	}
	if errs, _ := parseKMDUploadErrors(`<table><tr><td>a</td><td>b</td><td>c</td></tr></table>`); len(errs) != 0 {
		t.Fatalf("tables without the error header must be ignored: %+v", errs)
	}
}

func TestCheckKMDDeclarationHeader(t *testing.T) {
	in := sampleKMDInput()
	if err := checkKMDDeclarationHeader(kmdPageWithUploadErrors, in); err != nil {
		t.Fatalf("expected header to match: %v", err)
	}
	in.TaxPayerRegCode = "16773537"
	if err := checkKMDDeclarationHeader(kmdPageWithUploadErrors, in); err == nil {
		t.Fatal("expected taxpayer mismatch")
	}
	in = sampleKMDInput()
	in.Month = 8
	if err := checkKMDDeclarationHeader(kmdPageWithUploadErrors, in); err == nil {
		t.Fatal("expected period mismatch")
	}
	if err := checkKMDDeclarationHeader("<html></html>", sampleKMDInput()); err == nil {
		t.Fatal("missing header must fail closed")
	}
}

func TestCompareKMD(t *testing.T) {
	in := sampleKMDInput()
	live := &KMDMainSection{Flags: KMDFlags{NoPurchases: true}, Fields: KMDMainFields{
		TransactionsWithRate24: "7 999,65", InputVATTotal: "423,91", EUAcquisitionsTotal: "602,77", EUAcquisitionsGoods: "400,88", OtherGoodsTotal: "43,96", ImportVAT: "0,00",
	}}
	liveA := []KMDINFARow{{PartnerCode: "10000000", PartnerName: "EXAMPLE AS", InvoiceNumber: "INV-1", InvoiceDate: "30.09.2026", InvoiceSum: "7 999,65", TaxRate: "24%", SumForRateInPeriod: "7 999,65"}}
	if d := compareKMD(in, live, liveA, nil); len(d) != 0 {
		t.Fatalf("expected match, got %v", d)
	}

	// The 19.10.2026 failure: empty live draft must never look OK.
	if d := compareKMD(in, &KMDMainSection{}, nil, nil); len(d) < 6 {
		t.Fatalf("expected empty draft to fail on every line, got %v", d)
	}

	live.Fields.InputVATTotal = "424,23"
	live.Fields.ExemptSupply = "1,00"
	d := compareKMD(in, live, append(liveA, liveA[0]), nil)
	joined := strings.Join(d, "\n")
	for _, want := range []string{"line_5", "line_8", "inf_a: unexpected live row"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("expected %q in diffs:\n%s", want, joined)
		}
	}
}

func TestNormAmount(t *testing.T) {
	for in, want := range map[string]string{"7 999,65": "7999.65", "1": "1.00", "0,5": "0.50", "": "", " 1 000,00": "1000.00", "-2.1": "-2.10"} {
		if got, err := normAmount(in); err != nil || got != want {
			t.Errorf("normAmount(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
}
