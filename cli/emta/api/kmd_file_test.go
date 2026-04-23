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
