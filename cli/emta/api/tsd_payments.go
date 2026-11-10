package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"regexp"
	"time"
)

// TSD payments view (apa-tsd), used for appendix 1/2 payments from period 2026-10.
const tsdPaymentsAPI = baseURL + "/apa-tsd-payments-api/client/api/v1"

var tsdHeaderRe = map[string]*regexp.Regexp{
	"regKood": regexp.MustCompile(`<regKood>([^<]*)</regKood>`),
	"year":    regexp.MustCompile(`<c108_Aasta>([^<]*)</c108_Aasta>`),
	"month":   regexp.MustCompile(`<c109_Kuu>([^<]*)</c109_Kuu>`),
}

// CheckTSDDraftOwner fails unless the declaration belongs to the given taxpayer and period.
// The payments CSV carries neither, so this is the only guard against uploading to the wrong draft.
func (c *Client) CheckTSDDraftOwner(declarationID, regCode string, year, month int) error {
	exp, err := c.ExportTSDXML(declarationID)
	if err != nil {
		return fmt.Errorf("reading declaration %s: %w", declarationID, err)
	}
	return checkTSDHeader(exp.Bytes, regCode, year, month)
}

func checkTSDHeader(xmlBytes []byte, regCode string, year, month int) error {
	want := map[string]string{"regKood": regCode, "year": fmt.Sprint(year), "month": fmt.Sprint(month)}
	for k, re := range tsdHeaderRe {
		m := re.FindSubmatch(xmlBytes)
		if m == nil || string(m[1]) != want[k] {
			got := ""
			if m != nil {
				got = string(m[1])
			}
			return fmt.Errorf("declaration %s is %q, input says %q", k, got, want[k])
		}
	}
	return nil
}

// tsdPaymentsToken signs in to auth-gateway through the existing SSO cookies and returns its bearer token.
func (c *Client) tsdPaymentsToken() (string, error) {
	if err := c.ensureSession(); err != nil {
		return "", err
	}
	loginURL := baseURL + "/auth-gateway/v1/client/login?redirect_uri=" + url.QueryEscape(baseURL+"/apa-tsd/client/")
	resp, err := c.session.Client.Get(loginURL)
	if err != nil {
		return "", fmt.Errorf("auth-gateway login: %w", err)
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	resp, err = c.retry429(func() (*http.Request, error) {
		req, err := http.NewRequest("GET", baseURL+"/auth-gateway/v1/client/access-token?", nil)
		if err == nil {
			req.Header.Set("Accept", "application/json")
		}
		return req, err
	})
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("auth-gateway access-token (%d): session expired? run 'emta-cli login'", resp.StatusCode)
	}
	var tok portalAccessTokenResponse
	if err := json.Unmarshal(data, &tok); err != nil || tok.AccessToken == "" {
		return "", fmt.Errorf("auth-gateway access-token: no token in response")
	}
	return tok.AccessToken, nil
}

// retry429 repeats the request while EMTA answers 429 (seen under deadline load), up to ~5 minutes.
func (c *Client) retry429(newReq func() (*http.Request, error)) (*http.Response, error) {
	wait := 5 * time.Second
	for attempt := 0; ; attempt++ {
		req, err := newReq()
		if err != nil {
			return nil, err
		}
		resp, err := c.session.Client.Do(req)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode != http.StatusTooManyRequests || attempt >= 15 {
			return resp, nil
		}
		resp.Body.Close()
		time.Sleep(wait)
		if wait < 30*time.Second {
			wait *= 2
		}
	}
}

func (c *Client) tsdPaymentsDo(token, method, path string, body []byte, contentType string) (json.RawMessage, error) {
	resp, err := c.retry429(func() (*http.Request, error) {
		req, err := http.NewRequest(method, tsdPaymentsAPI+path, bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Accept", "application/json")
		if contentType != "" {
			req.Header.Set("Content-Type", contentType)
		}
		return req, nil
	})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, fmt.Errorf("%s %s failed (%d): %s", method, path, resp.StatusCode, string(data[:min(500, len(data))]))
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return json.RawMessage("null"), nil
	}
	return json.RawMessage(data), nil
}

// UploadTSDPaymentsCSV uploads the transitional appendix 1 CSV into the declaration's payments view.
func (c *Client) UploadTSDPaymentsCSV(declarationID, fileName string, csv []byte) (json.RawMessage, error) {
	token, err := c.tsdPaymentsToken()
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	part, err := w.CreateFormFile("file", fileName)
	if err != nil {
		return nil, err
	}
	part.Write(csv)
	w.Close()
	return c.tsdPaymentsDo(token, "POST", "/files/csv/upload/"+url.PathEscape(declarationID), buf.Bytes(), w.FormDataContentType())
}

// ReadTSDPayments reads the payments list and its summary live from the payments view.
func (c *Client) ReadTSDPayments(declarationID string) (map[string]json.RawMessage, error) {
	token, err := c.tsdPaymentsToken()
	if err != nil {
		return nil, err
	}
	q := url.Values{"declarationIdentifier": {declarationID}, "searchFilter": {""}, "isOnlyInvalidResults": {"false"}, "sort": {"executionDate,desc"}, "size": {"200"}, "page": {"0"}}
	base := "/declarations/" + url.PathEscape(declarationID) + "/payments/faulty"
	list, err := c.tsdPaymentsDo(token, "GET", base+"?"+q.Encode()+"&includeCancelledStatus=false", nil, "")
	if err != nil {
		return nil, err
	}
	summary, err := c.tsdPaymentsDo(token, "GET", base+"/summary?"+q.Encode(), nil, "")
	if err != nil {
		return nil, err
	}
	return map[string]json.RawMessage{"payments": list, "summary": summary}, nil
}
