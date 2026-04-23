# TSD XML Workflow Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a shared EMTA XML file workflow foundation and use it to implement TSD XML export, XML import into a new or existing draft, and explicit TSD submit.

**Architecture:** Extract TSD CLI wiring out of `cmd/root.go`, add a shared `api/xml_workflow.go` transport layer for download/upload/submit form handling, then implement a `api/tsd_xml.go` adapter that uses the shared layer against TSD-specific Wicket/HTML pages. Keep existing KMD field-edit flow untouched for now, but shape the transport layer so KMD can migrate onto it later.

**Tech Stack:** Go 1.21+, Cobra CLI, `goquery` HTML parsing, EMTA Wicket/HTML endpoints, Go test fixtures under `cli/emta/testdata`.

---

### Task 1: Split TSD CLI Wiring Out of `cmd/root.go`

**Files:**
- Create: `cli/emta/cmd/tsd.go`
- Modify: `cli/emta/cmd/root.go`
- Modify: `cli/emta/cmd/root_test.go`
- Test: `cli/emta/cmd/root_test.go`

- [ ] **Step 1: Write the failing command registration test**

```go
func TestRootRegistersTSDSubcommands(t *testing.T) {
	cmd := rootCmd

	tsd, _, err := cmd.Find([]string{"tsd"})
	if err != nil {
		t.Fatalf("expected tsd command: %v", err)
	}

	subcommands := map[string]bool{}
	for _, child := range tsd.Commands() {
		subcommands[child.Name()] = true
	}

	for _, name := range []string{"list", "show"} {
		if !subcommands[name] {
			t.Fatalf("missing tsd subcommand %q", name)
		}
	}
}
```

- [ ] **Step 2: Run test to verify baseline still passes and protects behavior**

Run:

```bash
go test ./cmd -run TestRootRegistersTSDSubcommands -v
```

Expected:

```text
PASS
```

- [ ] **Step 3: Create `cmd/tsd.go` and move TSD command construction there**

```go
package cmd

import (
	"fmt"
	"strconv"
	"time"

	"github.com/fatih/color"
	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/spf13/cobra"
	"github.com/stefanoamorelli/estonia-ai-kit/cli/emta/api"
)

func newTSDCommand() *cobra.Command {
	tsdCmd := &cobra.Command{
		Use:   "tsd",
		Short: "Income and social tax return (TSD) operations",
	}

	var listYear string
	tsdListCmd := &cobra.Command{
		Use:   "list",
		Short: "List TSD declarations",
		RunE: func(cmd *cobra.Command, args []string) error {
			session, err := loadSession()
			if err != nil {
				return err
			}
			client := api.NewClient(session)

			if listYear == "" {
				listYear = strconv.Itoa(time.Now().Year())
			}

			result, err := client.GetTSDList(listYear)
			if err != nil {
				return fmt.Errorf("fetching TSD list: %w", err)
			}

			if len(result.Declarations) == 0 {
				fmt.Printf("No TSD declarations found for %s\n", listYear)
				return nil
			}

			bold := color.New(color.Bold)
			if result.Person != "" {
				bold.Printf("%s\n", result.Person)
			}
			bold.Printf("TSD Declarations for %s\n\n", listYear)

			t := newTable()
			t.AppendHeader(table.Row{"ID", "Reg.No", "Period", "Submitted", "Status", "Method", "Modified By"})
			for _, d := range result.Declarations {
				period := fmt.Sprintf("%s/%s", d.Year, d.Month)
				t.AppendRow(table.Row{d.DeclarationID, d.RegNo, period, d.SubmissionDate, d.Status, d.Method, d.ModifiedBy})
			}
			t.Render()
			return nil
		},
	}
	tsdListCmd.Flags().StringVar(&listYear, "year", "", "Year to list (defaults to current year)")

	tsdShowCmd := &cobra.Command{
		Use:   "show <declaration-id>",
		Short: "Show TSD summary (tax breakdown codes 110-119)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			session, err := loadSession()
			if err != nil {
				return err
			}
			client := api.NewClient(session)
			summary, err := client.GetTSDSummary(args[0])
			if err != nil {
				return fmt.Errorf("fetching TSD summary: %w", err)
			}

			cyan := color.New(color.FgCyan, color.Bold)
			cyan.Println("Income and social tax return (TSD)")
			fmt.Println()
			fmt.Printf("%s | %s | Status: %s | Currency: %s\n", summary.Form, summary.Period, summary.Status, summary.Currency)
			fmt.Println()

			t := newTable()
			t.AppendHeader(table.Row{"", "Code", "Amount (EUR)"})
			for _, line := range summary.Lines {
				t.AppendRow(table.Row{line.Label, line.Code, line.Amount})
			}
			t.Render()
			return nil
		},
	}

	tsdCmd.AddCommand(tsdListCmd, tsdShowCmd)
	return tsdCmd
}
```

- [ ] **Step 4: Replace inline TSD registration in `cmd/root.go` with a single call**

```go
func init() {
	// existing login/logout setup...

	rootCmd.AddCommand(newTSDCommand())
}
```

- [ ] **Step 5: Run command tests**

Run:

```bash
go test ./cmd -v
```

Expected:

```text
ok  	github.com/stefanoamorelli/estonia-ai-kit/cli/emta/cmd
```

- [ ] **Step 6: Commit**

```bash
git add cli/emta/cmd/root.go cli/emta/cmd/tsd.go cli/emta/cmd/root_test.go
git commit -m "refactor(emta): split tsd commands from root wiring"
```

### Task 2: Add Shared XML Workflow Primitives

**Files:**
- Create: `cli/emta/api/xml_workflow.go`
- Create: `cli/emta/api/xml_workflow_test.go`
- Create: `cli/emta/testdata/tsd/.gitkeep`
- Test: `cli/emta/api/xml_workflow_test.go`

- [ ] **Step 1: Write the failing parser tests for Wicket-style form and download action discovery**

```go
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
```

- [ ] **Step 2: Run parser tests and confirm failure**

Run:

```bash
go test ./api -run 'TestParseXML(Action|Download)' -v
```

Expected:

```text
FAIL
```

- [ ] **Step 3: Implement shared workflow types and parser helpers in `api/xml_workflow.go`**

```go
package api

import (
	"bytes"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"path"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

type XMLUploadAction struct {
	FormAction      string   `json:"form_action"`
	FileFieldName   string   `json:"file_field_name"`
	SubmitFieldName string   `json:"submit_field_name,omitempty"`
	SubmitValue     string   `json:"submit_value,omitempty"`
	HiddenFields    []KVPair `json:"hidden_fields,omitempty"`
}

type XMLExportResult struct {
	DeclarationID string   `json:"declaration_id,omitempty"`
	PageURL       string   `json:"page_url,omitempty"`
	DownloadURL   string   `json:"download_url,omitempty"`
	FileName      string   `json:"file_name,omitempty"`
	Messages      []string `json:"messages,omitempty"`
	Bytes         []byte   `json:"-"`
}

type XMLImportResult struct {
	DeclarationID string   `json:"declaration_id,omitempty"`
	PageURL       string   `json:"page_url,omitempty"`
	ActionURL     string   `json:"action_url,omitempty"`
	Messages      []string `json:"messages,omitempty"`
	RawRedirect   string   `json:"raw_redirect,omitempty"`
}

type KVPair struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

func parseXMLUploadForm(html string) (*XMLUploadAction, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil, err
	}

	var result *XMLUploadAction
	doc.Find("form").EachWithBreak(func(_ int, form *goquery.Selection) bool {
		if enctype, _ := form.Attr("enctype"); !strings.Contains(enctype, "multipart/form-data") {
			return true
		}

		action, _ := form.Attr("action")
		fileField := ""
		form.Find(`input[type="file"]`).Each(func(_ int, sel *goquery.Selection) {
			if fileField == "" {
				fileField, _ = sel.Attr("name")
			}
		})
		if action == "" || fileField == "" {
			return true
		}

		upload := &XMLUploadAction{
			FormAction:    action,
			FileFieldName: fileField,
		}
		form.Find(`input[type="hidden"]`).Each(func(_ int, sel *goquery.Selection) {
			name, _ := sel.Attr("name")
			value, _ := sel.Attr("value")
			if name != "" {
				upload.HiddenFields = append(upload.HiddenFields, KVPair{Name: name, Value: value})
			}
		})
		form.Find(`input[type="submit"], button[type="submit"]`).Each(func(_ int, sel *goquery.Selection) {
			if upload.SubmitFieldName != "" {
				return
			}
			upload.SubmitFieldName, _ = sel.Attr("name")
			upload.SubmitValue, _ = sel.Attr("value")
		})
		result = upload
		return false
	})

	if result == nil {
		return nil, fmt.Errorf("xml upload form not found")
	}
	return result, nil
}

func parseXMLDownloadLink(html string) (string, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return "", err
	}

	for _, selector := range []string{`a[href*="/xml/"]`, `a[href*="export"][href*="xml"]`} {
		if href, ok := doc.Find(selector).First().Attr("href"); ok && href != "" {
			return href, nil
		}
	}
	return "", fmt.Errorf("xml download link not found")
}

func buildMultipartBody(action *XMLUploadAction, fileName string, fileBytes []byte) (*bytes.Buffer, string, error) {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	for _, field := range action.HiddenFields {
		if err := writer.WriteField(field.Name, field.Value); err != nil {
			return nil, "", err
		}
	}
	part, err := writer.CreateFormFile(action.FileFieldName, fileName)
	if err != nil {
		return nil, "", err
	}
	if _, err := part.Write(fileBytes); err != nil {
		return nil, "", err
	}
	if action.SubmitFieldName != "" {
		if err := writer.WriteField(action.SubmitFieldName, action.SubmitValue); err != nil {
			return nil, "", err
		}
	}
	if err := writer.Close(); err != nil {
		return nil, "", err
	}
	return body, writer.FormDataContentType(), nil
}

func resolveAppURL(pageURL, href string) string {
	if href == "" {
		return pageURL
	}
	base, err := url.Parse(pageURL)
	if err != nil {
		return href
	}
	ref, err := url.Parse(href)
	if err != nil {
		return href
	}
	return base.ResolveReference(ref).String()
}

func exportedFileName(declarationID, rawURL string) string {
	ext := path.Ext(rawURL)
	if ext == "" {
		ext = ".xml"
	}
	return "declaration-" + declarationID + ext
}
```

- [ ] **Step 4: Add a small transport helper test for multipart construction**

```go
func TestBuildMultipartBodyIncludesHiddenFieldsAndFile(t *testing.T) {
	action := &XMLUploadAction{
		FormAction:    "/upload",
		FileFieldName: "xmlFile",
		SubmitFieldName: "importButton",
		SubmitValue:   "Import",
		HiddenFields: []KVPair{{Name: "csrf", Value: "abc123"}},
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
```

- [ ] **Step 5: Run API tests**

Run:

```bash
go test ./api -run 'Test(ParseXML|BuildMultipart)' -v
```

Expected:

```text
PASS
```

- [ ] **Step 6: Commit**

```bash
git add cli/emta/api/xml_workflow.go cli/emta/api/xml_workflow_test.go cli/emta/testdata/tsd/.gitkeep
git commit -m "feat(emta): add shared xml workflow primitives"
```

### Task 3: Implement TSD XML Export

**Files:**
- Create: `cli/emta/api/tsd_xml.go`
- Create: `cli/emta/api/tsd_xml_test.go`
- Modify: `cli/emta/cmd/tsd.go`
- Test: `cli/emta/api/tsd_xml_test.go`

- [ ] **Step 1: Write the failing TSD export parser test**

```go
func TestParseTSDXMLExportAction(t *testing.T) {
	html := `
	<div class="actions">
	  <a href="/tsd2/client/declaration/14808598/export/xml/">Export XML</a>
	</div>`

	link, err := parseTSDXMLExportAction(html)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if link != "/tsd2/client/declaration/14808598/export/xml/" {
		t.Fatalf("unexpected link: %q", link)
	}
}
```

- [ ] **Step 2: Run TSD XML tests to confirm failure**

Run:

```bash
go test ./api -run TestParseTSDXMLExportAction -v
```

Expected:

```text
FAIL
```

- [ ] **Step 3: Implement TSD XML export API**

```go
package api

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

func parseTSDXMLExportAction(html string) (string, error) {
	return parseXMLDownloadLink(html)
}

func (c *Client) getTSDDeclarationPage(declarationID string) (string, string, error) {
	if err := c.ensureSession(); err != nil {
		return "", "", err
	}
	pageURL := baseURL + "/tsd2/client/declaration/" + declarationID + "/summary/show/"
	body, err := c.doGet(pageURL)
	if err != nil {
		return "", "", err
	}
	return pageURL, string(body), nil
}

func (c *Client) ExportTSDXML(declarationID string) (*XMLExportResult, error) {
	pageURL, html, err := c.getTSDDeclarationPage(declarationID)
	if err != nil {
		return nil, err
	}

	href, err := parseTSDXMLExportAction(html)
	if err != nil {
		return nil, err
	}

	downloadURL := resolveAppURL(pageURL, href)
	req, err := http.NewRequest("GET", downloadURL, nil)
	if err != nil {
		return nil, err
	}
	c.setAuthHeaders(req)

	resp, err := c.session.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("xml export failed (%d): %s", resp.StatusCode, string(raw))
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	fileName := exportedFileName(declarationID, downloadURL)
	if cd := resp.Header.Get("Content-Disposition"); strings.Contains(cd, "filename=") {
		if parts := strings.Split(cd, "filename="); len(parts) == 2 {
			fileName = strings.Trim(parts[1], `"`)
		}
	}

	return &XMLExportResult{
		DeclarationID: declarationID,
		PageURL:       pageURL,
		DownloadURL:   downloadURL,
		FileName:      fileName,
		Bytes:         data,
	}, nil
}
```

- [ ] **Step 4: Add `tsd xml export` CLI command**

```go
func newTSDXMLCommand() *cobra.Command {
	tsdXMLCmd := &cobra.Command{
		Use:   "xml",
		Short: "TSD XML import/export operations",
	}

	var declarationID string
	var outputPath string

	exportCmd := &cobra.Command{
		Use:   "export",
		Short: "Export TSD declaration XML",
		RunE: func(cmd *cobra.Command, args []string) error {
			if declarationID == "" || outputPath == "" {
				return fmt.Errorf("--declaration-id and --output are required")
			}
			session, err := loadSession()
			if err != nil {
				return err
			}
			client := api.NewClient(session)
			result, err := client.ExportTSDXML(declarationID)
			if err != nil {
				return err
			}
			if err := os.WriteFile(outputPath, result.Bytes, 0o600); err != nil {
				return err
			}
			return printJSON(map[string]string{
				"declaration_id": result.DeclarationID,
				"output":         outputPath,
				"file_name":      result.FileName,
			})
		},
	}
	exportCmd.Flags().StringVar(&declarationID, "declaration-id", "", "Stable declaration id from tsd list")
	exportCmd.Flags().StringVar(&outputPath, "output", "", "Path to write XML file")

	tsdXMLCmd.AddCommand(exportCmd)
	return tsdXMLCmd
}
```

- [ ] **Step 5: Wire `tsd xml` under the TSD parent command and run tests**

Run:

```bash
go test ./api ./cmd -v
```

Expected:

```text
ok  	github.com/stefanoamorelli/estonia-ai-kit/cli/emta/api
ok  	github.com/stefanoamorelli/estonia-ai-kit/cli/emta/cmd
```

- [ ] **Step 6: Commit**

```bash
git add cli/emta/api/tsd_xml.go cli/emta/api/tsd_xml_test.go cli/emta/cmd/tsd.go
git commit -m "feat(emta): add tsd xml export command"
```

### Task 4: Implement TSD XML Import Into a New Draft

**Files:**
- Modify: `cli/emta/api/tsd_xml.go`
- Modify: `cli/emta/api/tsd_xml_test.go`
- Modify: `cli/emta/cmd/tsd.go`
- Test: `cli/emta/api/tsd_xml_test.go`

- [ ] **Step 1: Write the failing upload-form parser and result-message tests**

```go
func TestParseTSDXMLImportForm(t *testing.T) {
	html := `
	<form action="./declaration?1-1.IFormSubmitListener-content-uploadForm"
	      enctype="multipart/form-data">
	  <input type="hidden" name="csrf" value="token-1"/>
	  <input type="file" name="xmlFile"/>
	  <input type="submit" name="importButton" value="Import"/>
	</form>`

	action, err := parseTSDXMLImportForm(html)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if action.FileFieldName != "xmlFile" {
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
```

- [ ] **Step 2: Run tests to confirm failure**

Run:

```bash
go test ./api -run 'TestParseTSDXMLImportForm|TestParseTSDImportMessages' -v
```

Expected:

```text
FAIL
```

- [ ] **Step 3: Implement draft creation + XML import workflow**

```go
func parseFeedbackMessages(html string) []string {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil
	}

	var messages []string
	doc.Find(".feedbackPanel li, .feedbackPanelERROR, .feedbackPanelINFO").Each(func(_ int, sel *goquery.Selection) {
		text := strings.TrimSpace(sel.Text())
		if text != "" {
			messages = append(messages, text)
		}
	})
	return messages
}

func parseTSDXMLImportForm(html string) (*XMLUploadAction, error) {
	return parseXMLUploadForm(html)
}

func (c *Client) CreateTSDDraft(year, month int) (string, string, error) {
	if err := c.ensureSession(); err != nil {
		return "", "", err
	}

	// Start from declarations page for the target year.
	listURL := baseURL + "/tsd2/client/declarations/" + strconv.Itoa(year)
	body, err := c.doGet(listURL)
	if err != nil {
		return "", "", err
	}

	// Placeholder for actual Wicket draft-creation flow:
	// parse create form, submit year/month, follow redirect to draft page.
	pageURL, declarationID, err := createTSDDraftFromDeclarationsPage(c, listURL, string(body), year, month)
	if err != nil {
		return "", "", err
	}
	return pageURL, declarationID, nil
}

func (c *Client) ImportTSDXMLIntoPage(pageURL, declarationID, fileName string, xmlBytes []byte) (*XMLImportResult, error) {
	body, err := c.doGet(pageURL)
	if err != nil {
		return nil, err
	}

	action, err := parseTSDXMLImportForm(string(body))
	if err != nil {
		return nil, err
	}

	multipartBody, contentType, err := buildMultipartBody(action, fileName, xmlBytes)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", resolveAppURL(pageURL, action.FormAction), multipartBody)
	if err != nil {
		return nil, err
	}
	c.setAuthHeaders(req)
	req.Header.Set("Content-Type", contentType)

	resp, err := c.session.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("xml import failed (%d): %s", resp.StatusCode, string(raw))
	}

	return &XMLImportResult{
		DeclarationID: declarationID,
		PageURL:       pageURL,
		ActionURL:     resolveAppURL(pageURL, action.FormAction),
		Messages:      parseFeedbackMessages(string(raw)),
	}, nil
}

func (c *Client) CreateTSDDraftFromXML(year, month int, fileName string, xmlBytes []byte) (*XMLImportResult, error) {
	pageURL, declarationID, err := c.CreateTSDDraft(year, month)
	if err != nil {
		return nil, err
	}
	return c.ImportTSDXMLIntoPage(pageURL, declarationID, fileName, xmlBytes)
}
```

- [ ] **Step 4: Add `tsd xml import --year --month` CLI wiring**

```go
var year int
var month int
var inputPath string
var declarationID string

importCmd := &cobra.Command{
	Use:   "import",
	Short: "Import TSD XML into a new or existing draft",
	RunE: func(cmd *cobra.Command, args []string) error {
		if inputPath == "" {
			return fmt.Errorf("--input is required")
		}

		xmlBytes, err := os.ReadFile(inputPath)
		if err != nil {
			return err
		}

		session, err := loadSession()
		if err != nil {
			return err
		}
		client := api.NewClient(session)

		var result *api.XMLImportResult
		switch {
		case year != 0 && month != 0:
			result, err = client.CreateTSDDraftFromXML(year, month, filepath.Base(inputPath), xmlBytes)
		case declarationID != "":
			pageURL := baseURL + "/tsd2/client/declaration/" + declarationID + "/summary/show/"
			result, err = client.ImportTSDXMLIntoPage(pageURL, declarationID, filepath.Base(inputPath), xmlBytes)
		default:
			return fmt.Errorf("either --year and --month or --declaration-id is required")
		}
		if err != nil {
			return err
		}
		return printJSON(result)
	},
}
importCmd.Flags().IntVar(&year, "year", 0, "Tax year")
importCmd.Flags().IntVar(&month, "month", 0, "Tax month (1-12)")
importCmd.Flags().StringVar(&declarationID, "declaration-id", "", "Existing draft declaration id")
importCmd.Flags().StringVar(&inputPath, "input", "", "Path to XML input file")
```

- [ ] **Step 5: Run tests**

Run:

```bash
go test ./api ./cmd -v
```

Expected:

```text
PASS
```

- [ ] **Step 6: Commit**

```bash
git add cli/emta/api/tsd_xml.go cli/emta/api/tsd_xml_test.go cli/emta/cmd/tsd.go
git commit -m "feat(emta): add tsd xml import workflow"
```

### Task 5: Implement TSD Submit From Draft ID

**Files:**
- Modify: `cli/emta/api/tsd_xml.go`
- Modify: `cli/emta/api/tsd_xml_test.go`
- Modify: `cli/emta/cmd/tsd.go`
- Test: `cli/emta/api/tsd_xml_test.go`

- [ ] **Step 1: Write the failing submit action parser test**

```go
func TestParseTSDSubmitAction(t *testing.T) {
	html := `
	<form action="./declaration?1-1.IFormSubmitListener-submitPanel-submitForm">
	  <input type="hidden" name="csrf" value="token-1"/>
	  <input type="submit" name="submitButton" value="Submit"/>
	</form>`

	action, err := parseTSDSubmitAction(html)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if action.FormAction == "" || action.SubmitFieldName == "" {
		t.Fatalf("incomplete submit action: %#v", action)
	}
}
```

- [ ] **Step 2: Run the parser test and confirm failure**

Run:

```bash
go test ./api -run TestParseTSDSubmitAction -v
```

Expected:

```text
FAIL
```

- [ ] **Step 3: Implement TSD submit**

```go
func parseTSDSubmitAction(html string) (*XMLUploadAction, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil, err
	}

	var result *XMLUploadAction
	doc.Find("form").EachWithBreak(func(_ int, form *goquery.Selection) bool {
		submit := form.Find(`input[type="submit"], button[type="submit"]`).First()
		if submit.Length() == 0 {
			return true
		}
		value, _ := submit.Attr("value")
		name, _ := submit.Attr("name")
		if !strings.Contains(strings.ToLower(value), "submit") && !strings.Contains(strings.ToLower(name), "submit") {
			return true
		}

		action, _ := form.Attr("action")
		result = &XMLUploadAction{FormAction: action, SubmitFieldName: name, SubmitValue: value}
		form.Find(`input[type="hidden"]`).Each(func(_ int, sel *goquery.Selection) {
			hiddenName, _ := sel.Attr("name")
			hiddenValue, _ := sel.Attr("value")
			if hiddenName != "" {
				result.HiddenFields = append(result.HiddenFields, KVPair{Name: hiddenName, Value: hiddenValue})
			}
		})
		return false
	})

	if result == nil {
		return nil, fmt.Errorf("tsd submit action not found")
	}
	return result, nil
}

func (c *Client) SubmitTSD(declarationID string) (*XMLImportResult, error) {
	pageURL, html, err := c.getTSDDeclarationPage(declarationID)
	if err != nil {
		return nil, err
	}

	action, err := parseTSDSubmitAction(html)
	if err != nil {
		return nil, err
	}

	values := url.Values{}
	for _, field := range action.HiddenFields {
		values.Set(field.Name, field.Value)
	}
	if action.SubmitFieldName != "" {
		values.Set(action.SubmitFieldName, action.SubmitValue)
	}

	req, err := http.NewRequest("POST", resolveAppURL(pageURL, action.FormAction), strings.NewReader(values.Encode()))
	if err != nil {
		return nil, err
	}
	c.setAuthHeaders(req)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.session.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)

	return &XMLImportResult{
		DeclarationID: declarationID,
		PageURL:       pageURL,
		ActionURL:     resolveAppURL(pageURL, action.FormAction),
		Messages:      parseFeedbackMessages(string(raw)),
	}, nil
}
```

- [ ] **Step 4: Add `tsd submit --declaration-id --confirm` CLI command in `cmd/tsd.go`**

```go
var submitDeclarationID string
var submitConfirm bool

submitCmd := &cobra.Command{
	Use:   "submit",
	Short: "Submit a TSD draft by declaration id",
	RunE: func(cmd *cobra.Command, args []string) error {
		if submitDeclarationID == "" {
			return fmt.Errorf("--declaration-id is required")
		}
		if !submitConfirm {
			return fmt.Errorf("--confirm is required for submit")
		}

		session, err := loadSession()
		if err != nil {
			return err
		}
		client := api.NewClient(session)
		result, err := client.SubmitTSD(submitDeclarationID)
		if err != nil {
			return err
		}
		return printJSON(result)
	},
}
submitCmd.Flags().StringVar(&submitDeclarationID, "declaration-id", "", "Stable declaration id from tsd list")
submitCmd.Flags().BoolVar(&submitConfirm, "confirm", false, "Actually submit the declaration")
```

- [ ] **Step 5: Run tests**

Run:

```bash
go test ./api ./cmd -v
```

Expected:

```text
PASS
```

- [ ] **Step 6: Commit**

```bash
git add cli/emta/api/tsd_xml.go cli/emta/api/tsd_xml_test.go cli/emta/cmd/tsd.go
git commit -m "feat(emta): add tsd submit workflow"
```

### Task 6: Document the XML Workflow and Leave KMD Migration Hooks

**Files:**
- Modify: `cli/emta/README.md`
- Modify: `cli/emta/CLAUDE.md`
- Modify: `cli/emta/api/kmd.go`
- Test: `cli/emta/api/kmd_test.go`

- [ ] **Step 1: Add README command documentation for TSD XML**

```md
### TSD XML Export

```sh
./emta-cli tsd xml export --declaration-id <id> --output tsd.xml
```

### TSD XML Import Into New Draft

```sh
./emta-cli tsd xml import --year 2026 --month 3 --input tsd.xml
```

### TSD XML Import Into Existing Draft

```sh
./emta-cli tsd xml import --declaration-id <id> --input tsd.xml
```

### Submit TSD Draft

```sh
./emta-cli tsd submit --declaration-id <id> --confirm
```
```

- [ ] **Step 2: Update CLI guidance in `CLAUDE.md`**

```md
### Export TSD XML

```sh
./emta-cli tsd xml export --declaration-id <id> --output tsd.xml
```

### Import TSD XML

```sh
./emta-cli tsd xml import --year 2026 --month 3 --input tsd.xml
./emta-cli tsd xml import --declaration-id <id> --input tsd.xml
```

### Submit TSD draft

```sh
./emta-cli tsd submit --declaration-id <id> --confirm
```
```

- [ ] **Step 3: Add a no-op marker in `api/kmd.go` to point later migration at shared XML workflow**

```go
// KMD XML migration should reuse xml_workflow.go once TSD XML is stable.
```

- [ ] **Step 4: Run tests and smoke the CLI help output**

Run:

```bash
go test ./...
./emta-cli tsd --help
./emta-cli tsd xml --help
```

Expected:

```text
PASS
```

- [ ] **Step 5: Commit**

```bash
git add cli/emta/README.md cli/emta/CLAUDE.md cli/emta/api/kmd.go
git commit -m "docs(emta): document tsd xml workflow"
```

### Task 7: Manual Real-Session Smoke Validation

**Files:**
- Modify: `cli/emta/README.md`
- Create: `cli/emta/testdata/tsd/README.md`
- Test: real EMTA session, no committed secrets

- [ ] **Step 1: Create a fixture capture checklist document**

```md
# TSD Fixture Capture Notes

- Save sanitized HTML pages, not live secrets.
- Save:
  - declarations list page
  - declaration detail page with XML export link
  - draft page with XML upload form
  - import success page
  - import validation error page
- Remove tokens, cookies, personal ids, and company-sensitive payloads before commit.
```

- [ ] **Step 2: Run real-session smoke sequence**

Run:

```bash
./emta-cli tsd list --year 2026
./emta-cli tsd xml export --declaration-id <existing-id> --output /tmp/tsd-export.xml
./emta-cli tsd xml import --year 2026 --month 3 --input /tmp/tsd-export.xml
./emta-cli tsd submit --declaration-id <draft-id> --confirm
```

Expected:

```text
- export writes XML file
- import returns declaration id and messages
- submit returns final status/messages
```

- [ ] **Step 3: Update README with any real-world limitations discovered**

```md
## Known Limitations

- XML import requires an existing authenticated EMTA session.
- TSD upload/submit flows may change when EMTA changes Wicket markup.
- If EMTA rejects a file, inspect the returned `messages` field first.
```

- [ ] **Step 4: Commit**

```bash
git add cli/emta/README.md cli/emta/testdata/tsd/README.md
git commit -m "test(emta): document tsd xml smoke validation"
```
