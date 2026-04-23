package api

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

type KMDGeneratedFile struct {
	Part          string `json:"part,omitempty"`
	Status        string `json:"status,omitempty"`
	RequestedAt   string `json:"requested_at,omitempty"`
	GeneratedAt   string `json:"generated_at,omitempty"`
	FileName      string `json:"file_name,omitempty"`
	FileSize      string `json:"file_size,omitempty"`
	DownloadHref  string `json:"download_href,omitempty"`
}

type KMDReportRequestForm struct {
	FormAction      string   `json:"form_action"`
	HiddenFields    []KVPair `json:"hidden_fields,omitempty"`
	Options         []string `json:"options,omitempty"`
	SubmitFieldName string   `json:"submit_field_name,omitempty"`
	SubmitValue     string   `json:"submit_value,omitempty"`
}

var kmdReportTypeAliases = map[string]string{
	"main":           "radio30",
	"inf-a":          "radio31",
	"inf-a-summary":  "radio32",
	"inf-b":          "radio33",
	"inf-b-summary":  "radio34",
	"all":            "radio35",
}

func parseKMDFileUploadEntryForm(html string) (*XMLUploadAction, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil, err
	}

	form := doc.Find(`form input[name="uploadButton"]`).First().Closest("form")
	if form.Length() == 0 {
		return nil, fmt.Errorf("kmd file upload entry form not found")
	}

	action, _ := form.Attr("action")
	result := &XMLUploadAction{
		FormAction:      action,
		SubmitFieldName: "uploadButton",
		SubmitValue:     "Lisa andmed failist",
	}
	form.Find(`input[type="hidden"]`).Each(func(_ int, sel *goquery.Selection) {
		name, _ := sel.Attr("name")
		value, _ := sel.Attr("value")
		if name != "" {
			result.HiddenFields = append(result.HiddenFields, KVPair{Name: name, Value: value})
		}
	})
	if submit := form.Find(`input[name="uploadButton"]`).First(); submit.Length() > 0 {
		if value, ok := submit.Attr("value"); ok && value != "" {
			result.SubmitValue = value
		}
	}
	return result, nil
}

func parseKMDFileUploadForm(html string) (*XMLUploadAction, error) {
	return parseXMLUploadForm(html)
}

func parseKMDGeneratedFiles(html string) ([]KMDGeneratedFile, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil, err
	}

	var rows []KMDGeneratedFile
	doc.Find("tr").Each(func(_ int, tr *goquery.Selection) {
		cells := tr.Find("td")
		if cells.Length() < 5 {
			return
		}
		values := make([]string, 0, cells.Length())
		cells.Each(func(_ int, td *goquery.Selection) {
			values = append(values, strings.TrimSpace(td.Text()))
		})
		if values[0] == "" {
			return
		}
		href, hasHref := cells.Eq(4).Find("a").Attr("href")
		if !hasHref && !strings.Contains(values[1], "Genereer") && !strings.Contains(values[1], "Valmis") {
			return
		}

		row := KMDGeneratedFile{
			Part:        values[0],
			Status:      values[1],
			RequestedAt: values[2],
			GeneratedAt: values[3],
		}
		if cells.Length() >= 5 {
			row.FileName = values[4]
			if hasHref {
				row.DownloadHref = href
			}
		}
		if cells.Length() >= 6 {
			row.FileSize = values[5]
		}
		rows = append(rows, row)
	})
	return rows, nil
}

func ParseKMDGeneratedFilesForCLI(html string) ([]KMDGeneratedFile, error) {
	return parseKMDGeneratedFiles(html)
}

func parseKMDReportRequestForm(html string) (*KMDReportRequestForm, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil, err
	}

	form := doc.Find(`form[action*="reportRequestForm"]`).First()
	if form.Length() == 0 {
		return nil, fmt.Errorf("kmd report request form not found")
	}

	action, _ := form.Attr("action")
	result := &KMDReportRequestForm{
		FormAction:      action,
		SubmitFieldName: "generateButton",
		SubmitValue:     "Genereeri fail",
	}
	form.Find(`input[type="hidden"]`).Each(func(_ int, sel *goquery.Selection) {
		name, _ := sel.Attr("name")
		value, _ := sel.Attr("value")
		if name != "" {
			result.HiddenFields = append(result.HiddenFields, KVPair{Name: name, Value: value})
		}
	})
	form.Find(`input[type="radio"][name="reportTypeChoiceGroup"]`).Each(func(_ int, sel *goquery.Selection) {
		if value, ok := sel.Attr("value"); ok && value != "" {
			result.Options = append(result.Options, value)
		}
	})
	if submit := form.Find(`input[name="generateButton"]`).First(); submit.Length() > 0 {
		if value, ok := submit.Attr("value"); ok && value != "" {
			result.SubmitValue = value
		}
	}
	return result, nil
}

func findKMDDraftID(items []KMDListItem, year, month int) string {
	for _, item := range items {
		if item.Year == year && item.Month == month && item.UpdateID != "" && !strings.EqualFold(item.Status, "Esitatud") {
			return item.DeclarationID
		}
	}
	for _, item := range items {
		if item.Year == year && item.Month == month && item.UpdateID != "" {
			return item.DeclarationID
		}
	}
	return ""
}

func findKMDItem(items []KMDListItem, stableID string) *KMDListItem {
	for _, item := range items {
		if item.DeclarationID == stableID {
			copy := item
			return &copy
		}
	}
	return nil
}

func (c *Client) openKMDFileUploadPage(declarationID string) (*kmdPage, error) {
	page, err := c.openKMDDeclaration(declarationID)
	if err != nil {
		return nil, err
	}

	action, err := parseKMDFileUploadEntryForm(page.HTML)
	if err != nil {
		return nil, err
	}

	values := url.Values{}
	for _, field := range action.HiddenFields {
		values.Set(field.Name, field.Value)
	}
	values.Set(action.SubmitFieldName, action.SubmitValue)

	return c.postKMDForm(resolveKMDURL(page.PageURL, action.FormAction), values)
}

func (c *Client) DeleteKMDDraft(declarationID string) error {
	basePage, err := c.getKMDPage("/customer-kmd2/declarations?1")
	if err != nil {
		return err
	}
	items, err := parseKMDList(basePage.HTML)
	if err != nil {
		return err
	}
	item := findKMDItem(items, declarationID)
	if item == nil {
		return fmt.Errorf("kmd declaration not found: %s", declarationID)
	}
	if item.DeleteID == "" {
		return fmt.Errorf("kmd declaration has no delete action: %s", declarationID)
	}
	confirmPage, err := c.getKMDPageWithReferer(resolveKMDURL(basePage.PageURL, item.DeleteID), basePage.PageURL)
	if err != nil {
		return err
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(confirmPage.HTML))
	if err != nil {
		return err
	}
	form := doc.Find(`form[action*="deleteForm"]`).First()
	if form.Length() == 0 {
		return fmt.Errorf("kmd delete confirmation form not found in %s", confirmPage.PageURL)
	}
	action, _ := form.Attr("action")
	values := url.Values{}
	form.Find(`input[type="hidden"]`).Each(func(_ int, sel *goquery.Selection) {
		name, _ := sel.Attr("name")
		value, _ := sel.Attr("value")
		if name != "" {
			values.Set(name, value)
		}
	})
	values.Set("delete", "Jah")

	req, err := http.NewRequest("POST", resolveKMDURL(confirmPage.PageURL, action), strings.NewReader(values.Encode()))
	if err != nil {
		return err
	}
	setKMDNavigationHeaders(req)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Referer", confirmPage.PageURL)
	resp, err := c.session.Client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		raw, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("kmd delete failed (%d): %s", resp.StatusCode, string(raw))
	}
	return nil
}

func (c *Client) ImportKMDFile(declarationID, fileName string, fileBytes []byte) (*XMLImportResult, error) {
	uploadPage, err := c.openKMDFileUploadPage(declarationID)
	if err != nil {
		return nil, err
	}

	action, err := parseKMDFileUploadForm(uploadPage.HTML)
	if err != nil {
		return nil, err
	}

	body, contentType, err := buildMultipartBody(action, filepath.Base(fileName), fileBytes)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", resolveKMDURL(uploadPage.PageURL, action.FormAction), body)
	if err != nil {
		return nil, err
	}
	setKMDNavigationHeaders(req)
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Origin", baseURL)
	req.Header.Set("Referer", uploadPage.PageURL)

	resp, err := c.session.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("kmd file import failed: status %d", resp.StatusCode)
	}

	return &XMLImportResult{
		DeclarationID: declarationID,
		PageURL:       resp.Request.URL.String(),
		ActionURL:     resolveKMDURL(uploadPage.PageURL, action.FormAction),
		Messages:      parseFeedbackMessages(string(raw)),
	}, nil
}

func (c *Client) CreateKMDDraftFromFile(year, month int, fileName string, fileBytes []byte) (*XMLImportResult, error) {
	if _, err := c.CreateKMDDraft(year, month); err != nil {
		return nil, err
	}
	items, err := c.ListKMDDeclarations()
	if err != nil {
		return nil, err
	}
	declarationID := findKMDDraftID(items, year, month)
	if declarationID == "" {
		return nil, fmt.Errorf("could not resolve created kmd draft for %04d-%02d", year, month)
	}
	return c.ImportKMDFile(declarationID, fileName, fileBytes)
}

func (c *Client) OpenKMDGeneratedFilesPage(declarationID string) (*kmdPage, error) {
	page, err := c.openKMDDeclaration(declarationID)
	if err != nil {
		return nil, err
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(page.HTML))
	if err != nil {
		return nil, err
	}
	href, ok := doc.Find(`a:contains("Genereeri fail")`).Attr("href")
	if !ok || href == "" {
		return nil, fmt.Errorf("kmd generate file link not found")
	}
	return c.getKMDPageWithReferer(resolveKMDURL(page.PageURL, href), page.PageURL)
}

func (c *Client) RequestKMDGeneratedFile(declarationID, reportType string) ([]KMDGeneratedFile, error) {
	page, err := c.OpenKMDGeneratedFilesPage(declarationID)
	if err != nil {
		return nil, err
	}

	form, err := parseKMDReportRequestForm(page.HTML)
	if err != nil {
		return nil, err
	}

	resolvedType, ok := kmdReportTypeAliases[reportType]
	if !ok {
		return nil, fmt.Errorf("unsupported kmd report type: %s", reportType)
	}

	values := url.Values{}
	for _, field := range form.HiddenFields {
		values.Set(field.Name, field.Value)
	}
	values.Set("reportTypeChoiceGroup", resolvedType)
	values.Set(form.SubmitFieldName, form.SubmitValue)

	nextPage, err := c.postKMDForm(resolveKMDURL(page.PageURL, form.FormAction), values)
	if err != nil {
		return nil, err
	}
	return parseKMDGeneratedFiles(nextPage.HTML)
}

func (c *Client) DownloadKMDGeneratedFile(pageURL, href string) (*XMLExportResult, error) {
	target := resolveKMDURL(pageURL, href)
	req, err := http.NewRequest("GET", target, nil)
	if err != nil {
		return nil, err
	}
	setKMDNavigationHeaders(req)
	req.Header.Set("Referer", pageURL)

	resp, err := c.session.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("kmd file download failed (%d): %s", resp.StatusCode, string(raw))
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	fileName := filepath.Base(target)
	if cd := resp.Header.Get("Content-Disposition"); strings.Contains(cd, "filename=") {
		if parts := strings.Split(cd, "filename="); len(parts) == 2 {
			fileName = strings.Trim(parts[1], `"`)
		}
	}

	return &XMLExportResult{
		PageURL:     pageURL,
		DownloadURL: target,
		FileName:    fileName,
		Bytes:       data,
	}, nil
}
