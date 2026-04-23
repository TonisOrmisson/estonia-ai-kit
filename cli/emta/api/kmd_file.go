package api

import (
	"archive/zip"
	"bytes"
	"encoding/csv"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"time"

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
	if (!ok || href == "") && strings.Contains(page.HTML, "printReportLink") {
		re := `href="([^"]*printReportLink[^"]*)"`
		if m := regexp.MustCompile(re).FindStringSubmatch(page.HTML); len(m) == 2 {
			href = m[1]
			ok = true
		}
	}
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

func (c *Client) ExportKMDReport(declarationID, reportType string) (*XMLExportResult, error) {
	page, err := c.OpenKMDGeneratedFilesPage(declarationID)
	if err != nil {
		return nil, err
	}
	files, err := parseKMDGeneratedFiles(page.HTML)
	if err != nil {
		return nil, err
	}

	matchPart := map[string]string{
		"main":          "KMD põhivorm",
		"inf-a":         "KMD INF A osa",
		"inf-a-summary": "KMD INF A osa koond",
		"inf-b":         "KMD INF B osa",
		"inf-b-summary": "KMD INF B osa koond",
	}[reportType]

	findDownload := func(rows []KMDGeneratedFile) string {
		for _, file := range rows {
			if file.Part == matchPart && file.DownloadHref != "" {
				return file.DownloadHref
			}
		}
		return ""
	}

	if href := findDownload(files); href != "" {
		return c.DownloadKMDGeneratedFile(page.PageURL, href)
	}

	if _, err := c.RequestKMDGeneratedFile(declarationID, reportType); err != nil {
		return nil, err
	}

	for i := 0; i < 5; i++ {
		time.Sleep(1500 * time.Millisecond)
		page, err := c.OpenKMDGeneratedFilesPage(declarationID)
		if err != nil {
			return nil, err
		}
		rows, err := parseKMDGeneratedFiles(page.HTML)
		if err != nil {
			return nil, err
		}
		if href := findDownload(rows); href != "" {
			return c.DownloadKMDGeneratedFile(page.PageURL, href)
		}
	}
	return nil, fmt.Errorf("no downloadable kmd file generated for %s", reportType)
}

func parseKMDCSVRows(data []byte) ([][]string, error) {
	unzipped, err := extractSingleCSVFromZIP(data)
	if err == nil {
		data = unzipped
	}
	text := strings.TrimPrefix(string(data), "\ufeff")
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	reader := csv.NewReader(strings.NewReader(text))
	reader.Comma = ';'
	reader.FieldsPerRecord = -1
	reader.LazyQuotes = true
	return reader.ReadAll()
}

func extractSingleCSVFromZIP(data []byte) ([]byte, error) {
	readerAt := bytes.NewReader(data)
	zr, err := zip.NewReader(readerAt, int64(len(data)))
	if err != nil {
		return nil, err
	}
	for _, file := range zr.File {
		if strings.HasSuffix(strings.ToLower(file.Name), ".csv") {
			rc, err := file.Open()
			if err != nil {
				return nil, err
			}
			defer rc.Close()
			return io.ReadAll(rc)
		}
	}
	return nil, fmt.Errorf("no csv file found in zip")
}

func ParseKMDMainCSV(data []byte) (*KMDMainSection, error) {
	rows, err := parseKMDCSVRows(data)
	if err != nil {
		return nil, err
	}
	if len(rows) < 3 {
		return nil, fmt.Errorf("kmd main csv too short")
	}
	dataRow := rows[2]
	get := func(i int) string {
		if i >= 0 && i < len(dataRow) {
			return strings.TrimSpace(dataRow[i])
		}
		return ""
	}
	return &KMDMainSection{
		Fields: KMDMainFields{
			TransactionsWithRate24: get(1),
			TransactionsWithRate22: get(2),
			TransactionsWithRate20: get(3),
			TransactionsWithRate9:  get(4),
			TransactionsWithRate5:  get(5),
			TransactionsWithRate13: get(6),
			TransactionsZeroVAT:    get(7),
			EUSupplyInclGoods:      get(8),
			EUSupplyGoods:          get(9),
			ExportZeroVAT:          get(10),
			SalePassengersReturn:   get(11),
			InputVATTotal:          get(14),
			ImportVAT:              get(15),
			FixedAssetsVAT:         get(16),
			CarsVAT:                get(17),
			NumberOfCars:           get(18),
			CarsPartialVAT:         get(19),
			NumberOfCarsPartial:    get(20),
			EUAcquisitionsTotal:    get(21),
			EUAcquisitionsGoods:    get(22),
			OtherGoodsTotal:        get(23),
			ImmovablesAndMetal:     get(24),
			ExemptSupply:           get(25),
			SpecialArrangements:    get(26),
			AdjustmentsPlus:        get(27),
			AdjustmentsMinus:       get(28),
		},
		Computed: KMDMainComputed{
			VATTotal:      get(12),
			VATFromImport: get(13),
			VATPayable:    get(29),
			OverpaidVAT:   get(30),
		},
	}, nil
}

func ParseKMDINFACSV(data []byte) (*KMDINFARows, error) {
	rows, err := parseKMDCSVRows(data)
	if err != nil {
		return nil, err
	}
	result := &KMDINFARows{}
	for _, row := range rows[2:] {
		if len(row) == 0 || strings.TrimSpace(row[0]) == "" {
			continue
		}
		item := KMDINFARow{}
		if len(row) > 1 { item.PartnerCode = strings.TrimSpace(row[1]) }
		if len(row) > 2 { item.PartnerName = strings.TrimSpace(row[2]) }
		if len(row) > 3 { item.InvoiceNumber = strings.TrimSpace(row[3]) }
		if len(row) > 4 { item.InvoiceDate = strings.TrimSpace(row[4]) }
		if len(row) > 5 { item.InvoiceSum = strings.TrimSpace(row[5]) }
		if len(row) > 6 { item.TaxRate = strings.TrimSpace(row[6]) }
		if len(row) > 8 { item.SumForRateInPeriod = strings.TrimSpace(row[8]) }
		if len(row) > 9 && strings.TrimSpace(row[9]) != "" { item.CommentCodes = []string{strings.TrimSpace(row[9])} }
		result.Rows = append(result.Rows, item)
	}
	return result, nil
}

func ParseKMDINFBCSV(data []byte) (*KMDINFBRows, error) {
	rows, err := parseKMDCSVRows(data)
	if err != nil {
		return nil, err
	}
	result := &KMDINFBRows{}
	for _, row := range rows[2:] {
		if len(row) == 0 || strings.TrimSpace(row[0]) == "" {
			continue
		}
		item := KMDINFBRow{}
		if len(row) > 1 { item.PartnerCode = strings.TrimSpace(row[1]) }
		if len(row) > 2 { item.PartnerName = strings.TrimSpace(row[2]) }
		if len(row) > 3 { item.InvoiceNumber = strings.TrimSpace(row[3]) }
		if len(row) > 4 { item.InvoiceDate = strings.TrimSpace(row[4]) }
		if len(row) > 5 { item.InvoiceSumVAT = strings.TrimSpace(row[5]) }
		if len(row) > 7 { item.VATInPeriod = strings.TrimSpace(row[7]) }
		if len(row) > 8 && strings.TrimSpace(row[8]) != "" { item.CommentCodes = []string{strings.TrimSpace(row[8])} }
		result.Rows = append(result.Rows, item)
	}
	return result, nil
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
