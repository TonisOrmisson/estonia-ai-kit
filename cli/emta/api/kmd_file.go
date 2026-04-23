package api

import (
	"archive/zip"
	"bytes"
	"encoding/csv"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

type KMDGeneratedFile struct {
	Part         string `json:"part,omitempty"`
	Status       string `json:"status,omitempty"`
	RequestedAt  string `json:"requested_at,omitempty"`
	GeneratedAt  string `json:"generated_at,omitempty"`
	FileName     string `json:"file_name,omitempty"`
	FileSize     string `json:"file_size,omitempty"`
	DownloadHref string `json:"download_href,omitempty"`
}

type KMDReportRequestForm struct {
	FormAction      string   `json:"form_action"`
	HiddenFields    []KVPair `json:"hidden_fields,omitempty"`
	Options         []string `json:"options,omitempty"`
	SubmitFieldName string   `json:"submit_field_name,omitempty"`
	SubmitValue     string   `json:"submit_value,omitempty"`
}

type KMDFileMetadata struct {
	CompanyName string
	CompanyCode string
	Year        string
	Month       string
}

var kmdReportTypeAliases = map[string]string{
	"main":          "radio30",
	"inf-a":         "radio31",
	"inf-a-summary": "radio32",
	"inf-b":         "radio33",
	"inf-b-summary": "radio34",
	"all":           "radio35",
}

func kmdCachePath(declarationID, reportType string) (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(configDir, "emta-cli", "kmd-cache", sanitizeStableID(declarationID), reportType+".csv"), nil
}

func saveCachedKMDReport(declarationID, reportType string, data []byte) error {
	path, err := kmdCachePath(declarationID, reportType)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func loadCachedKMDReport(declarationID, reportType string) ([]byte, error) {
	path, err := kmdCachePath(declarationID, reportType)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(path)
}

func deleteCachedKMDReports(declarationID string) error {
	path, err := kmdCachePath(declarationID, "main")
	if err != nil {
		return err
	}
	return os.RemoveAll(filepath.Dir(path))
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
	return deleteCachedKMDReports(declarationID)
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
	result, err := c.ImportKMDFile(declarationID, fileName, fileBytes)
	if err != nil {
		return nil, err
	}
	_ = saveCachedKMDReport(declarationID, "main", fileBytes)
	return result, nil
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
		exported, err := c.DownloadKMDGeneratedFile(page.PageURL, href)
		if err != nil {
			return nil, err
		}
		_ = saveCachedKMDReport(declarationID, reportType, exported.Bytes)
		return exported, nil
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
			exported, err := c.DownloadKMDGeneratedFile(page.PageURL, href)
			if err != nil {
				return nil, err
			}
			_ = saveCachedKMDReport(declarationID, reportType, exported.Bytes)
			return exported, nil
		}
	}
	return nil, fmt.Errorf("no downloadable kmd file generated for %s", reportType)
}

func (c *Client) ReadKMDMainFromFile(declarationID string) (*KMDMainSection, error) {
	exported, err := c.ExportKMDReport(declarationID, "main")
	if err != nil {
		cached, cacheErr := loadCachedKMDReport(declarationID, "main")
		if cacheErr != nil {
			return nil, err
		}
		exported = &XMLExportResult{Bytes: cached}
	}
	section, err := ParseKMDMainCSV(exported.Bytes)
	if err != nil {
		return nil, err
	}
	section.DeclarationID = declarationID
	return section, nil
}

func (c *Client) ReadKMDINFAFromFile(declarationID string) (*KMDINFARows, error) {
	exported, err := c.ExportKMDReport(declarationID, "inf-a")
	if err != nil {
		cached, cacheErr := loadCachedKMDReport(declarationID, "inf-a")
		if cacheErr != nil {
			return nil, err
		}
		exported = &XMLExportResult{Bytes: cached}
	}
	rows, err := ParseKMDINFACSV(exported.Bytes)
	if err != nil {
		return nil, err
	}
	rows.DeclarationID = declarationID
	return rows, nil
}

func (c *Client) ReadKMDINFBFromFile(declarationID string) (*KMDINFBRows, error) {
	exported, err := c.ExportKMDReport(declarationID, "inf-b")
	if err != nil {
		cached, cacheErr := loadCachedKMDReport(declarationID, "inf-b")
		if cacheErr != nil {
			return nil, err
		}
		exported = &XMLExportResult{Bytes: cached}
	}
	rows, err := ParseKMDINFBCSV(exported.Bytes)
	if err != nil {
		return nil, err
	}
	rows.DeclarationID = declarationID
	return rows, nil
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
		if len(row) > 1 {
			item.PartnerCode = strings.TrimSpace(row[1])
		}
		if len(row) > 2 {
			item.PartnerName = strings.TrimSpace(row[2])
		}
		if len(row) > 3 {
			item.InvoiceNumber = strings.TrimSpace(row[3])
		}
		if len(row) > 4 {
			item.InvoiceDate = strings.TrimSpace(row[4])
		}
		if len(row) > 5 {
			item.InvoiceSum = strings.TrimSpace(row[5])
		}
		if len(row) > 6 {
			item.TaxRate = strings.TrimSpace(row[6])
		}
		if len(row) > 8 {
			item.SumForRateInPeriod = strings.TrimSpace(row[8])
		}
		if len(row) > 9 && strings.TrimSpace(row[9]) != "" {
			item.CommentCodes = []string{strings.TrimSpace(row[9])}
		}
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
		if len(row) > 1 {
			item.PartnerCode = strings.TrimSpace(row[1])
		}
		if len(row) > 2 {
			item.PartnerName = strings.TrimSpace(row[2])
		}
		if len(row) > 3 {
			item.InvoiceNumber = strings.TrimSpace(row[3])
		}
		if len(row) > 4 {
			item.InvoiceDate = strings.TrimSpace(row[4])
		}
		if len(row) > 5 {
			item.InvoiceSumVAT = strings.TrimSpace(row[5])
		}
		if len(row) > 7 {
			item.VATInPeriod = strings.TrimSpace(row[7])
		}
		if len(row) > 8 && strings.TrimSpace(row[8]) != "" {
			item.CommentCodes = []string{strings.TrimSpace(row[8])}
		}
		result.Rows = append(result.Rows, item)
	}
	return result, nil
}

func parseKMDFileMetadataFromHTML(html string) (*KMDFileMetadata, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil, err
	}
	meta := &KMDFileMetadata{}
	text := strings.TrimSpace(doc.Find("body").Text())
	if strings.Contains(text, "kood kolm OÜ") {
		// no-op; real parsing below
	}
	if company := strings.TrimSpace(doc.Find("span:contains('16773537')").First().Text()); company != "" {
		parts := strings.Fields(company)
		if len(parts) > 0 {
			meta.CompanyCode = parts[0]
			meta.CompanyName = strings.TrimSpace(strings.TrimPrefix(company, meta.CompanyCode))
		}
	}
	if meta.CompanyCode == "" {
		re := regexp.MustCompile(`(\d{8})\s+([^\n\r<]+)`)
		if m := re.FindStringSubmatch(text); len(m) == 3 {
			meta.CompanyCode = m[1]
			meta.CompanyName = strings.TrimSpace(m[2])
		}
	}
	reYear := regexp.MustCompile(`Aasta:\s*(\d{4})`)
	if m := reYear.FindStringSubmatch(text); len(m) == 2 {
		meta.Year = m[1]
	}
	reMonth := regexp.MustCompile(`Kuu:\s*(\d{1,2})`)
	if m := reMonth.FindStringSubmatch(text); len(m) == 2 {
		meta.Month = m[1]
	}
	if meta.CompanyCode == "" || meta.Year == "" || meta.Month == "" {
		return nil, fmt.Errorf("kmd file metadata not found")
	}
	if meta.CompanyName == "" {
		meta.CompanyName = "Unknown Company"
	}
	return meta, nil
}

func buildKMDINFACSVFromScratch(meta *KMDFileMetadata, rowsState *KMDINFARows) ([]byte, error) {
	out := [][]string{
		{meta.CompanyName, meta.CompanyCode, fmt.Sprintf("%s / %02s", meta.Year, meta.Month)},
		{"KMD osa", "Tehingupartneri kood", "Tehingupartneri nimi", "Arve number", "Arve kuupäev", "Arve summa km-ta", "Maksumäär", "Maksustatav väärtus arvel", "KMD-l dekl-d käive", "Erisuse kood"},
	}
	for _, row := range rowsState.Rows {
		comment := ""
		if len(row.CommentCodes) > 0 {
			comment = row.CommentCodes[0]
		}
		out = append(out, []string{"A", row.PartnerCode, row.PartnerName, row.InvoiceNumber, row.InvoiceDate, row.InvoiceSum, row.TaxRate, "", row.SumForRateInPeriod, comment})
	}
	return writeKMDCSV(out)
}

func buildKMDINFBCSVFromScratch(meta *KMDFileMetadata, rowsState *KMDINFBRows) ([]byte, error) {
	out := [][]string{
		{meta.CompanyName, meta.CompanyCode, fmt.Sprintf("%s / %02s", meta.Year, meta.Month)},
		{"KMD osa", "Tehingupartneri kood", "Tehingupartneri nimi", "Arve number", "Arve kuupäev", "Arve summa km-ga", "Km summa arvel", "KMD-l dekl-d sisendkm", "Erisuse kood"},
	}
	for _, row := range rowsState.Rows {
		comment := ""
		if len(row.CommentCodes) > 0 {
			comment = row.CommentCodes[0]
		}
		out = append(out, []string{"B", row.PartnerCode, row.PartnerName, row.InvoiceNumber, row.InvoiceDate, row.InvoiceSumVAT, "", row.VATInPeriod, comment})
	}
	return writeKMDCSV(out)
}

func buildKMDMainCSVFromScratch(meta *KMDFileMetadata, section *KMDMainSection) ([]byte, error) {
	out := [][]string{
		{meta.CompanyName, meta.CompanyCode, fmt.Sprintf("%s / %02s", meta.Year, meta.Month)},
		{"KMD osa", "24% määraga maksustatav käive", "22% määraga maksustatav käive", "20% määraga maksustatav käive", "9% määraga maksustatav käive", "5% määraga maksustatav käive", "13% määraga maksustatav käive", "0% määraga maksustatav käive, sh", "kauba ja teenuse ühendusesisene käive kokku, sh", "kauba ühendusesisene käive", "kauba eksport, sh", "käibemaksutagastusega müük reisijale", "Käibemaks kokku", "Impordilt tasumisele kuuluv käibemaks", "Sisendkäibemaksu summa, mis on lubatud maha arvata, sh", "tollis impordilt tasutud käibemaks", "põhivara soetamisel tasutud käibemaks", "100% autode sisendkäibemaks", "100% autode arv", "50% autode sisendkäibemaks", "50% autode arv", "Kauba ja teenuse ühendusesisene soetamine kokku, sh", "kauba ühendusesisene soetamine", "Muu kauba soetamine ja teenuse saamine, sh", "kinnisasja, metallijäätmete, väärismetalli ja metalltoodete soetamine", "Maksuvaba käive", "Kinnisasja, metallijäätmete, väärismetalli, metalltoodete ja paigaldatava kauba käive", "Täpsustused (+)", "Täpsustused (-)", "Tasumisele kuuluv käibemaks", "Enammakstud käibemaks"},
		{
			"KMD põhivorm",
			section.Fields.TransactionsWithRate24,
			section.Fields.TransactionsWithRate22,
			section.Fields.TransactionsWithRate20,
			section.Fields.TransactionsWithRate9,
			section.Fields.TransactionsWithRate5,
			section.Fields.TransactionsWithRate13,
			section.Fields.TransactionsZeroVAT,
			section.Fields.EUSupplyInclGoods,
			section.Fields.EUSupplyGoods,
			section.Fields.ExportZeroVAT,
			section.Fields.SalePassengersReturn,
			section.Computed.VATTotal,
			section.Computed.VATFromImport,
			section.Fields.InputVATTotal,
			section.Fields.ImportVAT,
			section.Fields.FixedAssetsVAT,
			section.Fields.CarsVAT,
			section.Fields.NumberOfCars,
			section.Fields.CarsPartialVAT,
			section.Fields.NumberOfCarsPartial,
			section.Fields.EUAcquisitionsTotal,
			section.Fields.EUAcquisitionsGoods,
			section.Fields.OtherGoodsTotal,
			section.Fields.ImmovablesAndMetal,
			section.Fields.ExemptSupply,
			section.Fields.SpecialArrangements,
			section.Fields.AdjustmentsPlus,
			section.Fields.AdjustmentsMinus,
			section.Computed.VATPayable,
			section.Computed.OverpaidVAT,
		},
	}
	return writeKMDCSV(out)
}

func applyKMDMainPatch(section *KMDMainSection, patch KMDMainPatch) {
	if patch.NoSales != nil {
		section.Flags.NoSales = *patch.NoSales
	}
	if patch.NoPurchases != nil {
		section.Flags.NoPurchases = *patch.NoPurchases
	}
	if patch.Line1 != nil {
		section.Fields.TransactionsWithRate24 = *patch.Line1
	}
	if patch.Line11 != nil {
		section.Fields.TransactionsWithRate20 = *patch.Line11
	}
	if patch.Line12 != nil {
		section.Fields.TransactionsWithRate22 = *patch.Line12
	}
	if patch.Line2 != nil {
		section.Fields.TransactionsWithRate9 = *patch.Line2
	}
	if patch.Line21 != nil {
		section.Fields.TransactionsWithRate5 = *patch.Line21
	}
	if patch.Line22 != nil {
		section.Fields.TransactionsWithRate13 = *patch.Line22
	}
	if patch.Line3 != nil {
		section.Fields.TransactionsZeroVAT = *patch.Line3
	}
	if patch.Line31 != nil {
		section.Fields.EUSupplyInclGoods = *patch.Line31
	}
	if patch.Line311 != nil {
		section.Fields.EUSupplyGoods = *patch.Line311
	}
	if patch.Line32 != nil {
		section.Fields.ExportZeroVAT = *patch.Line32
	}
	if patch.Line321 != nil {
		section.Fields.SalePassengersReturn = *patch.Line321
	}
	if patch.Line5 != nil {
		section.Fields.InputVATTotal = *patch.Line5
	}
	if patch.Line51 != nil {
		section.Fields.ImportVAT = *patch.Line51
	}
	if patch.Line52 != nil {
		section.Fields.FixedAssetsVAT = *patch.Line52
	}
	if patch.Line53 != nil {
		section.Fields.CarsVAT = *patch.Line53
	}
	if patch.Line53Cars != nil {
		section.Fields.NumberOfCars = *patch.Line53Cars
	}
	if patch.Line54 != nil {
		section.Fields.CarsPartialVAT = *patch.Line54
	}
	if patch.Line54Cars != nil {
		section.Fields.NumberOfCarsPartial = *patch.Line54Cars
	}
	if patch.Line6 != nil {
		section.Fields.EUAcquisitionsTotal = *patch.Line6
	}
	if patch.Line61 != nil {
		section.Fields.EUAcquisitionsGoods = *patch.Line61
	}
	if patch.Line7 != nil {
		section.Fields.OtherGoodsTotal = *patch.Line7
	}
	if patch.Line71 != nil {
		section.Fields.ImmovablesAndMetal = *patch.Line71
	}
	if patch.Line8 != nil {
		section.Fields.ExemptSupply = *patch.Line8
	}
	if patch.Line9 != nil {
		section.Fields.SpecialArrangements = *patch.Line9
	}
	if patch.Line10 != nil {
		section.Fields.AdjustmentsPlus = *patch.Line10
	}
	if patch.Line11Adj != nil {
		section.Fields.AdjustmentsMinus = *patch.Line11Adj
	}
}

func decodeKMDCSVText(data []byte) ([][]string, error) {
	rows, err := parseKMDCSVRows(data)
	if err != nil {
		return nil, err
	}
	if len(rows) < 2 {
		return nil, fmt.Errorf("kmd csv too short")
	}
	return rows, nil
}

func writeKMDCSV(rows [][]string) ([]byte, error) {
	var b strings.Builder
	b.WriteRune('\ufeff')
	w := csv.NewWriter(&b)
	w.Comma = ';'
	w.UseCRLF = true
	if err := w.WriteAll(rows); err != nil {
		return nil, err
	}
	return []byte(b.String()), nil
}

func buildKMDMainCSVFromSection(original []byte, section *KMDMainSection) ([]byte, error) {
	rows, err := decodeKMDCSVText(original)
	if err != nil {
		return nil, err
	}
	dataRow := []string{
		"KMD põhivorm",
		section.Fields.TransactionsWithRate24,
		section.Fields.TransactionsWithRate22,
		section.Fields.TransactionsWithRate20,
		section.Fields.TransactionsWithRate9,
		section.Fields.TransactionsWithRate5,
		section.Fields.TransactionsWithRate13,
		section.Fields.TransactionsZeroVAT,
		section.Fields.EUSupplyInclGoods,
		section.Fields.EUSupplyGoods,
		section.Fields.ExportZeroVAT,
		section.Fields.SalePassengersReturn,
		section.Computed.VATTotal,
		section.Computed.VATFromImport,
		section.Fields.InputVATTotal,
		section.Fields.ImportVAT,
		section.Fields.FixedAssetsVAT,
		section.Fields.CarsVAT,
		section.Fields.NumberOfCars,
		section.Fields.CarsPartialVAT,
		section.Fields.NumberOfCarsPartial,
		section.Fields.EUAcquisitionsTotal,
		section.Fields.EUAcquisitionsGoods,
		section.Fields.OtherGoodsTotal,
		section.Fields.ImmovablesAndMetal,
		section.Fields.ExemptSupply,
		section.Fields.SpecialArrangements,
		section.Fields.AdjustmentsPlus,
		section.Fields.AdjustmentsMinus,
		section.Computed.VATPayable,
		section.Computed.OverpaidVAT,
	}
	rows = [][]string{rows[0], rows[1], dataRow}
	return writeKMDCSV(rows)
}

func buildKMDINFACSV(original []byte, state *KMDINFARows) ([]byte, error) {
	rows, err := decodeKMDCSVText(original)
	if err != nil {
		return nil, err
	}
	out := [][]string{rows[0], rows[1]}
	for _, row := range state.Rows {
		comment := ""
		if len(row.CommentCodes) > 0 {
			comment = row.CommentCodes[0]
		}
		out = append(out, []string{
			"A",
			row.PartnerCode,
			row.PartnerName,
			row.InvoiceNumber,
			row.InvoiceDate,
			row.InvoiceSum,
			row.TaxRate,
			"",
			row.SumForRateInPeriod,
			comment,
		})
	}
	return writeKMDCSV(out)
}

func buildKMDINFBCSV(original []byte, state *KMDINFBRows) ([]byte, error) {
	rows, err := decodeKMDCSVText(original)
	if err != nil {
		return nil, err
	}
	out := [][]string{rows[0], rows[1]}
	for _, row := range state.Rows {
		comment := ""
		if len(row.CommentCodes) > 0 {
			comment = row.CommentCodes[0]
		}
		out = append(out, []string{
			"B",
			row.PartnerCode,
			row.PartnerName,
			row.InvoiceNumber,
			row.InvoiceDate,
			row.InvoiceSumVAT,
			"",
			row.VATInPeriod,
			comment,
		})
	}
	return writeKMDCSV(out)
}

func (c *Client) UpdateKMDMainFromPatch(declarationID string, patch KMDMainPatch) (*KMDMainSection, error) {
	exported, err := c.ExportKMDReport(declarationID, "main")
	if err != nil {
		page, pageErr := c.openKMDDeclaration(declarationID)
		if pageErr != nil {
			return nil, pageErr
		}
		meta, metaErr := parseKMDFileMetadataFromHTML(page.HTML)
		if metaErr != nil {
			return nil, metaErr
		}
		section := &KMDMainSection{}
		applyKMDMainPatch(section, patch)
		updatedBytes, buildErr := buildKMDMainCSVFromScratch(meta, section)
		if buildErr != nil {
			return nil, buildErr
		}
		if _, impErr := c.ImportKMDFile(declarationID, "kmd-main.csv", updatedBytes); impErr != nil {
			return nil, impErr
		}
		_ = saveCachedKMDReport(declarationID, "main", updatedBytes)
		section.DeclarationID = declarationID
		return section, nil
	}
	section, err := ParseKMDMainCSV(exported.Bytes)
	if err != nil {
		return nil, err
	}
	applyKMDMainPatch(section, patch)
	updatedBytes, err := buildKMDMainCSVFromSection(exported.Bytes, section)
	if err != nil {
		return nil, err
	}
	if _, err := c.ImportKMDFile(declarationID, "kmd-main.csv", updatedBytes); err != nil {
		return nil, err
	}
	_ = saveCachedKMDReport(declarationID, "main", updatedBytes)
	section.DeclarationID = declarationID
	return section, nil
}

func (c *Client) UpdateKMDINFAFromPatch(declarationID string, patch KMDINFAPatch) (*KMDINFARows, error) {
	exported, err := c.ExportKMDReport(declarationID, "inf-a")
	if err != nil {
		page, pageErr := c.openKMDDeclaration(declarationID)
		if pageErr != nil {
			return nil, pageErr
		}
		meta, metaErr := parseKMDFileMetadataFromHTML(page.HTML)
		if metaErr != nil {
			return nil, metaErr
		}
		state := &KMDINFARows{Rows: patch.Rows, DeclarationID: declarationID}
		updatedBytes, buildErr := buildKMDINFACSVFromScratch(meta, state)
		if buildErr != nil {
			return nil, buildErr
		}
		if _, impErr := c.ImportKMDFile(declarationID, "kmd-infa.csv", updatedBytes); impErr != nil {
			return nil, impErr
		}
		_ = saveCachedKMDReport(declarationID, "inf-a", updatedBytes)
		return state, nil
	}
	state, err := ParseKMDINFACSV(exported.Bytes)
	if err != nil {
		return nil, err
	}
	merged, _, err := mergeINFARows(state.Rows, patch.Rows)
	if err != nil {
		return nil, err
	}
	state.Rows = merged
	updatedBytes, err := buildKMDINFACSV(exported.Bytes, state)
	if err != nil {
		return nil, err
	}
	if _, err := c.ImportKMDFile(declarationID, "kmd-infa.csv", updatedBytes); err != nil {
		return nil, err
	}
	_ = saveCachedKMDReport(declarationID, "inf-a", updatedBytes)
	state.DeclarationID = declarationID
	return state, nil
}

func (c *Client) DeleteKMDINFAFromFile(declarationID, partnerCode, invoiceNumber string) (*KMDINFARows, error) {
	exported, err := c.ExportKMDReport(declarationID, "inf-a")
	if err != nil {
		cached, cacheErr := loadCachedKMDReport(declarationID, "inf-a")
		if cacheErr != nil {
			return nil, err
		}
		exported = &XMLExportResult{Bytes: cached}
	}
	state, err := ParseKMDINFACSV(exported.Bytes)
	if err != nil {
		return nil, err
	}
	filtered, _, err := deleteINFARow(state.Rows, partnerCode, invoiceNumber)
	if err != nil {
		return nil, err
	}
	state.Rows = filtered
	updatedBytes, err := buildKMDINFACSV(exported.Bytes, state)
	if err != nil {
		return nil, err
	}
	if _, err := c.ImportKMDFile(declarationID, "kmd-infa.csv", updatedBytes); err != nil {
		return nil, err
	}
	_ = saveCachedKMDReport(declarationID, "inf-a", updatedBytes)
	state.DeclarationID = declarationID
	return state, nil
}

func (c *Client) UpdateKMDINFBFromPatch(declarationID string, patch KMDINFBPatch) (*KMDINFBRows, error) {
	exported, err := c.ExportKMDReport(declarationID, "inf-b")
	if err != nil {
		page, pageErr := c.openKMDDeclaration(declarationID)
		if pageErr != nil {
			return nil, pageErr
		}
		meta, metaErr := parseKMDFileMetadataFromHTML(page.HTML)
		if metaErr != nil {
			return nil, metaErr
		}
		state := &KMDINFBRows{Rows: patch.Rows, DeclarationID: declarationID}
		updatedBytes, buildErr := buildKMDINFBCSVFromScratch(meta, state)
		if buildErr != nil {
			return nil, buildErr
		}
		if _, impErr := c.ImportKMDFile(declarationID, "kmd-infb.csv", updatedBytes); impErr != nil {
			return nil, impErr
		}
		_ = saveCachedKMDReport(declarationID, "inf-b", updatedBytes)
		return state, nil
	}
	state, err := ParseKMDINFBCSV(exported.Bytes)
	if err != nil {
		return nil, err
	}
	merged, _, err := mergeINFBRows(state.Rows, patch.Rows)
	if err != nil {
		return nil, err
	}
	state.Rows = merged
	updatedBytes, err := buildKMDINFBCSV(exported.Bytes, state)
	if err != nil {
		return nil, err
	}
	if _, err := c.ImportKMDFile(declarationID, "kmd-infb.csv", updatedBytes); err != nil {
		return nil, err
	}
	_ = saveCachedKMDReport(declarationID, "inf-b", updatedBytes)
	state.DeclarationID = declarationID
	return state, nil
}

func (c *Client) DeleteKMDINFBFromFile(declarationID, partnerCode, invoiceNumber string) (*KMDINFBRows, error) {
	exported, err := c.ExportKMDReport(declarationID, "inf-b")
	if err != nil {
		cached, cacheErr := loadCachedKMDReport(declarationID, "inf-b")
		if cacheErr != nil {
			return nil, err
		}
		exported = &XMLExportResult{Bytes: cached}
	}
	state, err := ParseKMDINFBCSV(exported.Bytes)
	if err != nil {
		return nil, err
	}
	filtered, _, err := deleteINFBRow(state.Rows, partnerCode, invoiceNumber)
	if err != nil {
		return nil, err
	}
	state.Rows = filtered
	updatedBytes, err := buildKMDINFBCSV(exported.Bytes, state)
	if err != nil {
		return nil, err
	}
	if _, err := c.ImportKMDFile(declarationID, "kmd-infb.csv", updatedBytes); err != nil {
		return nil, err
	}
	_ = saveCachedKMDReport(declarationID, "inf-b", updatedBytes)
	state.DeclarationID = declarationID
	return state, nil
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
