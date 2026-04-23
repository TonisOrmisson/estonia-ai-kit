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
