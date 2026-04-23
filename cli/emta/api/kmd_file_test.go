package api

import "testing"

func TestParseKMDFileUploadEntryForm(t *testing.T) {
	html := `
	<form id="id3d" method="post" action="./page?5-1.IFormSubmitListener-contentPanel-contentComponent-kmdForm">
	  <input type="hidden" name="id3d_hf_0" value=""/>
	  <input type="submit" value="Lisa andmed failist" name="uploadButton" id="id3e"/>
	</form>`

	action, err := parseKMDFileUploadEntryForm(html)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if action.FormAction != "./page?5-1.IFormSubmitListener-contentPanel-contentComponent-kmdForm" {
		t.Fatalf("unexpected form action: %q", action.FormAction)
	}
	if action.SubmitFieldName != "uploadButton" {
		t.Fatalf("unexpected submit field: %q", action.SubmitFieldName)
	}
}

func TestParseKMDFileUploadForm(t *testing.T) {
	html := `
	<form id="id45" method="post" action="./page?6-1.IFormSubmitListener-contentPanel-contentComponent-progressUpload" enctype="multipart/form-data">
	  <input type="hidden" name="id45_hf_0" value=""/>
	  <input type="file" name="fileInput"/>
	  <input type="submit" value="Lisa" name="uploadButton" id="id44"/>
	</form>`

	action, err := parseKMDFileUploadForm(html)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if action.FormAction != "./page?6-1.IFormSubmitListener-contentPanel-contentComponent-progressUpload" {
		t.Fatalf("unexpected form action: %q", action.FormAction)
	}
	if action.FileFieldName != "fileInput" {
		t.Fatalf("unexpected file field: %q", action.FileFieldName)
	}
	if action.SubmitFieldName != "uploadButton" {
		t.Fatalf("unexpected submit field: %q", action.SubmitFieldName)
	}
}

func TestParseKMDGeneratedFilesPage(t *testing.T) {
	html := `
	<table>
	  <tr>
	    <td>KMD põhivorm</td>
	    <td>Valmis</td>
	    <td>23.04.2026 14:52:54</td>
	    <td>23.04.2026 14:53:10</td>
	    <td><a href="./page?2-1.ILinkListener-contentPanel-contentComponent-reportList-0-download">kmd_2026_3.zip</a></td>
	    <td>2 KB</td>
	  </tr>
	</table>`

	rows, err := parseKMDGeneratedFiles(html)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected one row, got %d", len(rows))
	}
	if rows[0].DownloadHref == "" {
		t.Fatal("expected download href")
	}
}

func TestParseKMDReportRequestForm(t *testing.T) {
	html := `
	<form id="id11" method="post" action="./page?2-1.IFormSubmitListener-contentPanel-contentComponent-reportRequestForm">
	  <input type="hidden" name="id11_hf_0" value=""/>
	  <input type="radio" name="reportTypeChoiceGroup" value="radio30"/>
	  <input type="radio" name="reportTypeChoiceGroup" value="radio35"/>
	  <input type="submit" value="Genereeri fail" name="generateButton" id="id12"/>
	</form>`

	form, err := parseKMDReportRequestForm(html)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if form.FormAction != "./page?2-1.IFormSubmitListener-contentPanel-contentComponent-reportRequestForm" {
		t.Fatalf("unexpected action: %q", form.FormAction)
	}
	if len(form.Options) != 2 {
		t.Fatalf("expected two options, got %d", len(form.Options))
	}
}

func TestParseKMDMainCSV(t *testing.T) {
	data := []byte("Company;12345678;2026 / 03\n" +
		"KMD osa;24;22;20;9;5;13;0;eu all;eu goods;export;travellers;vat total;import vat;input vat;import input;fixed;cars100;cars100count;cars50;cars50count;eu acq total;eu acq;other total;metal;exempt;special;+;-;payable;overpaid\n" +
		"KMD põhivorm;6670,00;;;;;;;;;;;1600,80;0,00;317,03;;;;;;;218,99;;5,09;;;;;;1283,77;0,00\n")

	section, err := ParseKMDMainCSV(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if section.Fields.TransactionsWithRate24 != "6670,00" {
		t.Fatalf("unexpected line 1: %q", section.Fields.TransactionsWithRate24)
	}
	if section.Computed.VATPayable != "1283,77" {
		t.Fatalf("unexpected payable: %q", section.Computed.VATPayable)
	}
}

func TestParseKMDINFACSV(t *testing.T) {
	data := []byte("Company;12345678;2026 / 03\n" +
		"KMD osa;Tehingupartneri kood;Tehingupartneri nimi;Arve number;Arve kuupäev;Arve summa km-ta;Maksumäär;Maksustatav väärtus arvel;KMD-l dekl-d käive;Erisuse kood\n" +
		"A;10220984;EXAMPLE AS;202603311030;31.03.2026;6670,00;24;;6670,00;\n")

	rows, err := ParseKMDINFACSV(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rows.Rows) != 1 {
		t.Fatalf("expected one row, got %d", len(rows.Rows))
	}
	if rows.Rows[0].PartnerCode != "10220984" || rows.Rows[0].SumForRateInPeriod != "6670,00" {
		t.Fatalf("unexpected row: %+v", rows.Rows[0])
	}
}

func TestParseKMDINFBCSV(t *testing.T) {
	data := []byte("Company;12345678;2026 / 03\n" +
		"KMD osa;Tehingupartneri kood;Tehingupartneri nimi;Arve number;Arve kuupäev;Arve summa km-ga;Km summa arvel;KMD-l dekl-d sisendkm;Erisuse kood\n")

	rows, err := ParseKMDINFBCSV(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rows.Rows) != 0 {
		t.Fatalf("expected no rows, got %d", len(rows.Rows))
	}
}
