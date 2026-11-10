package api

import (
	"bytes"
	"encoding/csv"
	"strings"
	"testing"
)

func TestBuildTSDPayrollCSV(t *testing.T) {
	in := &TSDPayrollCSVInput{TaxPayerRegCode: "12345678", Year: 2026, Month: 10, Payments: []TSDPayrollCSVRow{{PersonCode: "12345678901", Name: "Example Person", PaymentType: "10", Amount: "1200.5", SocialTaxBase: "1200.5", Allowance610: "700"}}}
	data, err := BuildTSDPayrollCSV(in)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(data, []byte("\xef\xbb\xbf")) || !bytes.HasSuffix(data, []byte("\r\n")) {
		t.Fatal("missing BOM or CRLF")
	}
	r := csv.NewReader(strings.NewReader(string(data[3:])))
	r.Comma = ';'
	rows, err := r.ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || len(rows[1]) != 18 || rows[1][0] != "12345678901" || rows[1][3] != "1200,50" || rows[1][5] != "1200,50" || rows[1][15] != "700,00" || rows[1][9] != "" {
		t.Fatalf("unexpected records: %v", rows)
	}
}

func TestTSDPayrollInputFailsClosed(t *testing.T) {
	if _, err := ParseTSDPayrollCSVInput([]byte(`{"payments":[],"ammount":"10"}`)); err == nil {
		t.Fatal("unknown field accepted")
	}
	if _, err := ParseTSDPayrollCSVInput([]byte(`{} {}`)); err == nil {
		t.Fatal("trailing JSON accepted")
	}
	row := TSDPayrollCSVRow{PersonCode: "12345678901", Name: "Example", PaymentType: "10", Amount: "10.00"}
	in := &TSDPayrollCSVInput{TaxPayerRegCode: "12345678", Year: 2026, Month: 10, Payments: []TSDPayrollCSVRow{row, row}}
	if _, err := BuildTSDPayrollCSV(in); err == nil {
		t.Fatal("duplicate payment accepted")
	}
	in.Payments = in.Payments[:1]
	in.Payments[0].Amount = "1.234"
	if _, err := BuildTSDPayrollCSV(in); err == nil {
		t.Fatal("invalid precision accepted")
	}
	in.Payments[0].Amount = "10"
	in.Month = 9
	if _, err := BuildTSDPayrollCSV(in); err == nil {
		t.Fatal("old period accepted")
	}
	in.Year = 2028
	in.Month = 1
	if _, err := BuildTSDPayrollCSV(in); err == nil {
		t.Fatal("unsupported period accepted")
	}
}

func TestRejectLegacyPayrollXML(t *testing.T) {
	legacy := []byte(`<tsd_vorm><tsd_L1_0/></tsd_vorm>`)
	if rejectLegacyPayrollXML(2026, 9, legacy) != nil {
		t.Fatal("old period rejected")
	}
	if rejectLegacyPayrollXML(2026, 10, legacy) == nil {
		t.Fatal("old appendix accepted")
	}
	if rejectLegacyPayrollXML(2026, 10, []byte(`<tsd_vorm><tsd_L2_0/></tsd_vorm>`)) == nil {
		t.Fatal("old appendix 2 accepted")
	}
	if err := rejectLegacyPayrollXML(2026, 10, []byte(`<tsd_vorm><tsd_L3_0/></tsd_vorm>`)); err != nil {
		t.Fatal(err)
	}
	if rejectLegacyPayrollXML(2026, 10, []byte(`<tsd_vorm>`)) == nil {
		t.Fatal("malformed XML accepted")
	}
}
