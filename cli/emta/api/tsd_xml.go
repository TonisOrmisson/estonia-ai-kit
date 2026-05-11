package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

func parseTSDXMLExportAction(html string) (string, error) {
	return parseXMLDownloadLink(html)
}

func (c *Client) getTSDDeclarationPage(declarationID string) (string, string, error) {
	if err := c.ensureSession(); err != nil {
		return "", "", err
	}

	candidates := []string{
		baseURL + "/tsd2/client/declaration/" + declarationID + "/summary/modify/",
		baseURL + "/tsd2/client/declaration/" + declarationID + "/summary/show/",
	}

	var lastErr error
	for _, pageURL := range candidates {
		body, err := c.doGet(pageURL)
		if err == nil {
			return pageURL, string(body), nil
		}
		lastErr = err
	}

	return "", "", lastErr
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

func parseTSDXMLImportForm(html string) (*XMLUploadAction, error) {
	return parseXMLUploadForm(html)
}

func parseFeedbackMessages(html string) []string {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil
	}

	var messages []string
	doc.Find(".feedbackPanel li, .feedbackPanelERROR, .feedbackPanelINFO, [data-name=\"text\"]").Each(func(_ int, sel *goquery.Selection) {
		text := strings.TrimSpace(sel.Text())
		if text != "" {
			messages = append(messages, text)
		}
	})
	return messages
}

type tsdImportResponse struct {
	OK       bool   `json:"ok"`
	Data     string `json:"data"`
	Detailed bool   `json:"detailed"`
}

func findNewTSDDeclarationID(before, after []TSDDeclaration, year, month int) string {
	seen := make(map[string]bool, len(before))
	for _, item := range before {
		seen[item.DeclarationID] = true
	}
	for _, item := range after {
		if item.DeclarationID == "" || seen[item.DeclarationID] {
			continue
		}
		itemYear, _ := strconv.Atoi(item.Year)
		itemMonth, _ := strconv.Atoi(item.Month)
		if itemYear == year && itemMonth == month {
			return item.DeclarationID
		}
	}
	for _, item := range after {
		itemYear, _ := strconv.Atoi(item.Year)
		itemMonth, _ := strconv.Atoi(item.Month)
		if itemYear == year && itemMonth == month {
			return item.DeclarationID
		}
	}
	return ""
}

func findDraftTSDDeclarationID(items []TSDDeclaration, year, month int) string {
	for _, item := range items {
		itemYear, _ := strconv.Atoi(item.Year)
		itemMonth, _ := strconv.Atoi(item.Month)
		if itemYear == year && itemMonth == month && !strings.EqualFold(item.Status, "Esitatud") {
			return item.DeclarationID
		}
	}
	return ""
}

func (c *Client) CreateTSDDraftFromXML(year, month int, fileName string, xmlBytes []byte, withSums bool) (*XMLImportResult, error) {
	if err := c.ensureSession(); err != nil {
		return nil, err
	}

	beforeList, err := c.GetTSDList(strconv.Itoa(year))
	if err != nil {
		return nil, err
	}

	pageURL := baseURL + "/tsd2/client/declarations/" + strconv.Itoa(year)
	body, err := c.doGet(pageURL)
	if err != nil {
		return nil, err
	}
	if _, err := parseTSDXMLImportForm(string(body)); err != nil {
		return nil, err
	}

	query := url.Values{}
	query.Set("doXmlImport", "")
	query.Set("vorm", "TSD")
	query.Set("year", strconv.Itoa(year))
	query.Set("month", strconv.Itoa(month))
	if withSums {
		query.Set("withSums", "")
	}

	importURL := pageURL + "?" + query.Encode()
	req, err := http.NewRequest("POST", importURL, nil)
	if err != nil {
		return nil, err
	}

	form := &XMLUploadAction{FileFieldName: "datafile"}
	multipartBody, contentType, err := buildMultipartBody(form, filepath.Base(fileName), xmlBytes)
	if err != nil {
		return nil, err
	}
	req.Body = io.NopCloser(multipartBody)
	req.ContentLength = int64(multipartBody.Len())
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Accept", "application/json, text/javascript, */*; q=0.01")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	req.Header.Set("Origin", baseURL)
	req.Header.Set("Referer", pageURL)
	c.setAuthHeaders(req)

	resp, err := c.session.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("xml import failed (%d): %s", resp.StatusCode, string(raw))
	}

	var importResp tsdImportResponse
	if err := json.Unmarshal(raw, &importResp); err != nil {
		return nil, fmt.Errorf("parsing xml import response: %w", err)
	}
	if !importResp.OK {
		return &XMLImportResult{
			PageURL:   pageURL,
			ActionURL: importURL,
			Messages:  []string{importResp.Data},
		}, fmt.Errorf("xml import rejected: %s", importResp.Data)
	}

	afterList, err := c.GetTSDList(strconv.Itoa(year))
	if err != nil {
		return nil, err
	}

	declarationID := findNewTSDDeclarationID(beforeList.Declarations, afterList.Declarations, year, month)
	if declarationID == "" {
		declarationID = findDraftTSDDeclarationID(afterList.Declarations, year, month)
	}
	messages := []string{}
	if strings.TrimSpace(importResp.Data) != "" {
		messages = append(messages, importResp.Data)
	}

	return &XMLImportResult{
		DeclarationID: declarationID,
		PageURL:       pageURL,
		ActionURL:     importURL,
		Messages:      messages,
	}, nil
}

func parseTSDSubmitAction(html string) (*XMLUploadAction, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil, err
	}

	form := doc.Find("#SummaryForm").First()
	if form.Length() == 0 {
		return nil, fmt.Errorf("tsd summary form not found")
	}

	action, _ := form.Attr("action")
	result := &XMLUploadAction{
		FormAction:      action,
		SubmitFieldName: "doConfirmation",
	}

	form.Find(`input[type="hidden"]`).Each(func(_ int, sel *goquery.Selection) {
		name, _ := sel.Attr("name")
		value, _ := sel.Attr("value")
		if name != "" {
			result.HiddenFields = append(result.HiddenFields, KVPair{Name: name, Value: value})
		}
	})

	if button := form.Find(`#summaryConfirmDeclaration`).First(); button.Length() > 0 {
		if name, ok := button.Attr("name"); ok && name != "" {
			result.SubmitFieldName = name
		}
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
	values.Set(action.SubmitFieldName, "")

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

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("tsd submit failed (%d): %s", resp.StatusCode, string(raw))
	}

	return &XMLImportResult{
		DeclarationID: declarationID,
		PageURL:       pageURL,
		ActionURL:     resolveAppURL(pageURL, action.FormAction),
		Messages:      parseFeedbackMessages(string(raw)),
	}, nil
}
