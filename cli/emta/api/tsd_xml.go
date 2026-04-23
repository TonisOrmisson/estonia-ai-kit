package api

import (
	"fmt"
	"io"
	"net/http"
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
