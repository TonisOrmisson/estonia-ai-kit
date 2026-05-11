package api

import "testing"

func TestParseTSDXMLExportAction(t *testing.T) {
	html := `
	<div class="actions">
	  <a href="/tsd2/client/declaration/14808598/summary/show/?doXmlExport">Export XML</a>
	</div>`

	link, err := parseTSDXMLExportAction(html)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if link != "/tsd2/client/declaration/14808598/summary/show/?doXmlExport" {
		t.Fatalf("unexpected link: %q", link)
	}
}

func TestParseTSDXMLImportForm(t *testing.T) {
	html := `
	<form data-name="import-form" method="post" enctype="multipart/form-data">
	  <input type="file" name="datafile"/>
	  <input type="checkbox" name="withSums"/>
	</form>`

	action, err := parseTSDXMLImportForm(html)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if action.FileFieldName != "datafile" {
		t.Fatalf("unexpected file field: %q", action.FileFieldName)
	}
}

func TestParseTSDImportMessages(t *testing.T) {
	html := `
	<ul class="feedbackPanel">
	  <li class="feedbackPanelERROR">Schema validation failed</li>
	  <li class="feedbackPanelINFO">Draft saved</li>
	</ul>`

	messages := parseFeedbackMessages(html)
	if len(messages) != 2 {
		t.Fatalf("expected two messages, got %d", len(messages))
	}
}

func TestParseTSDSubmitAction(t *testing.T) {
	html := `
	<form action="/tsd2/client/declaration/12345678/summary/modify/" id="SummaryForm">
	  <button type="button" id="summaryConfirmDeclaration" name="doConfirmation">Esita</button>
	  <button type="button" id="summaryCheckDeclaration">Uuenda ja kontrolli</button>
	  <input type="hidden" name="CSRFToken" value="token-1"/>
	</form>`

	action, err := parseTSDSubmitAction(html)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if action.FormAction != "/tsd2/client/declaration/12345678/summary/modify/" {
		t.Fatalf("unexpected form action: %q", action.FormAction)
	}
	if action.SubmitFieldName != "doConfirmation" {
		t.Fatalf("unexpected submit field: %q", action.SubmitFieldName)
	}
}
