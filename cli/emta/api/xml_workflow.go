package api

import (
	"bytes"
	"fmt"
	"mime/multipart"
	"net/url"
	"path"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

type KVPair struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

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

func parseXMLUploadForm(html string) (*XMLUploadAction, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil, err
	}

	var result *XMLUploadAction
	doc.Find("form").EachWithBreak(func(_ int, form *goquery.Selection) bool {
		if enctype, _ := form.Attr("enctype"); !strings.Contains(strings.ToLower(enctype), "multipart/form-data") {
			return true
		}

		fileField := ""
		form.Find(`input[type="file"]`).Each(func(_ int, sel *goquery.Selection) {
			if fileField == "" {
				fileField, _ = sel.Attr("name")
			}
		})
		if fileField == "" {
			return true
		}

		action, _ := form.Attr("action")
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
			if upload.SubmitValue == "" {
				upload.SubmitValue = strings.TrimSpace(sel.Text())
			}
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

	for _, selector := range []string{
		`a[href*="doXmlExport"]`,
		`a[href*="/export/xml/"]`,
		`a[href*="xml"]`,
	} {
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
