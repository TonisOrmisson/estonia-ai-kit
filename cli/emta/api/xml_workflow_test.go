package api

import (
	"strings"
	"testing"
)

func TestParseXMLActionFindsMultipartForm(t *testing.T) {
	html := `
	<form action="./declaration?1-1.IFormSubmitListener-uploadPanel-uploadForm"
	      enctype="multipart/form-data">
	  <input type="file" name="fileUpload"/>
	  <input type="submit" name="importButton" value="Import"/>
	</form>`

	action, err := parseXMLUploadForm(html)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if action.FormAction != "./declaration?1-1.IFormSubmitListener-uploadPanel-uploadForm" {
		t.Fatalf("unexpected form action: %q", action.FormAction)
	}
	if action.FileFieldName != "fileUpload" {
		t.Fatalf("unexpected file field: %q", action.FileFieldName)
	}
	if action.SubmitFieldName != "importButton" {
		t.Fatalf("unexpected submit field: %q", action.SubmitFieldName)
	}
}

func TestParseXMLDownloadLinkFindsXMLHref(t *testing.T) {
	html := `<a href="/tsd2/client/declaration/123/export/xml/">Laadi XML alla</a>`

	link, err := parseXMLDownloadLink(html)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if link != "/tsd2/client/declaration/123/export/xml/" {
		t.Fatalf("unexpected link: %q", link)
	}
}

func TestBuildMultipartBodyIncludesHiddenFieldsAndFile(t *testing.T) {
	action := &XMLUploadAction{
		FormAction:      "/upload",
		FileFieldName:   "xmlFile",
		SubmitFieldName: "importButton",
		SubmitValue:     "Import",
		HiddenFields:    []KVPair{{Name: "csrf", Value: "abc123"}},
	}

	body, contentType, err := buildMultipartBody(action, "tsd.xml", []byte("<xml/>"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	raw := body.String()
	if !strings.Contains(contentType, "multipart/form-data") {
		t.Fatalf("unexpected content type: %q", contentType)
	}
	for _, want := range []string{"name=\"csrf\"", "abc123", "filename=\"tsd.xml\"", "<xml/>", "name=\"importButton\""} {
		if !strings.Contains(raw, want) {
			t.Fatalf("multipart body missing %q", want)
		}
	}
}
