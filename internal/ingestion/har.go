package ingestion

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	ir "github.com/yourorg/recast/recast-ir"
)

type harFile struct {
	Log harLog `json:"log"`
}

type harLog struct {
	Version string     `json:"version"`
	Browser harBrowser `json:"browser"`
	Pages   []harPage  `json:"pages"`
	Entries []harEntry `json:"entries"`
}

type harBrowser struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type harPage struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

type harEntry struct {
	PageRef         string      `json:"pageref"`
	StartedDateTime string      `json:"startedDateTime"`
	Time            float64     `json:"time"`
	Request         harRequest  `json:"request"`
	Response        harResponse `json:"response"`
}

type harRequest struct {
	Method   string       `json:"method"`
	URL      string       `json:"url"`
	Headers  []harHeader  `json:"headers"`
	PostData *harPostData `json:"postData,omitempty"`
}

type harHeader struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type harPostData struct {
	MimeType string     `json:"mimeType"`
	Params   []harParam `json:"params,omitempty"`
	Text     string     `json:"text,omitempty"`
}

type harParam struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type harResponse struct {
	Status int    `json:"status"`
	URL    string `json:"url,omitempty"`
}

type HARIngester struct{}

func (h *HARIngester) FormatName() string {
	return "HAR (HTTP Archive)"
}

func (h *HARIngester) CanHandle(path string, data []byte) bool {
	return bytes.Contains(data, []byte(`"log"`)) &&
		bytes.Contains(data, []byte(`"entries"`)) &&
		bytes.Contains(data, []byte(`"request"`))
}

func (h *HARIngester) Parse(data []byte) (*ir.Trace, error) {
	var har harFile
	if err := json.Unmarshal(data, &har); err != nil {
		return nil, fmt.Errorf("HAR: JSON parse error: %w", err)
	}

	if len(har.Log.Entries) == 0 {
		return nil, fmt.Errorf("HAR: file contains no entries")
	}

	navEntries := filterNavigationEntries(har.Log.Entries)
	if len(navEntries) == 0 {
		return nil, fmt.Errorf("HAR: contains navigation data only — no interactions recorded")
	}

	name := "har_workflow"
	if len(har.Log.Pages) > 0 && har.Log.Pages[0].Title != "" {
		name = sanitizeName(har.Log.Pages[0].Title)
	}

	builder := ir.NewTrace(name).WithSourceFormat("har")

	for i, entry := range navEntries {
		step := convertHAREntry(entry, i+1)
		if step != nil {
			builder.AddStep(*step)
		}
	}

	return builder.BuildUnchecked(), nil
}

func filterNavigationEntries(entries []harEntry) []harEntry {
	var result []harEntry
	for _, entry := range entries {
		req := entry.Request
		if req.Method == "GET" {
			url := strings.ToLower(req.URL)
			if isStaticAsset(url) {
				continue
			}
			result = append(result, entry)
		}
		if req.Method == "POST" && req.PostData != nil {
			result = append(result, entry)
		}
	}

	deduped := make([]harEntry, 0, len(result))
	for i, e := range result {
		if i == 0 || e.Request.URL != result[i-1].Request.URL {
			deduped = append(deduped, e)
		}
	}
	return deduped
}

func isStaticAsset(url string) bool {
	staticExts := []string{".js", ".css", ".png", ".jpg", ".jpeg", ".gif", ".svg",
		".ico", ".woff", ".woff2", ".ttf", ".eot", ".map", ".json"}
	for _, ext := range staticExts {
		if strings.HasSuffix(url, ext) || strings.Contains(url, ext+"?") {
			return true
		}
	}
	skipDomains := []string{"google-analytics.com", "googletagmanager.com", "cdn.", "static."}
	for _, d := range skipDomains {
		if strings.Contains(url, d) {
			return true
		}
	}
	return false
}

func convertHAREntry(entry harEntry, idx int) *ir.Step {
	id := fmt.Sprintf("step_%03d", idx)
	req := entry.Request

	switch req.Method {
	case "GET":
		return &ir.Step{
			ID:    id,
			Type:  ir.StepNavigate,
			Value: req.URL,
			Wait:  ir.WaitSpec{Type: ir.WaitNetworkIdle, Timeout: 30000},
		}

	case "POST":
		if req.PostData == nil {
			return nil
		}
		comment := fmt.Sprintf("// HAR: POST %s — form submission (params: %d)", req.URL, len(req.PostData.Params))
		return &ir.Step{
			ID:      id,
			Type:    ir.StepNavigate,
			Value:   req.URL,
			Comment: comment,
			Wait:    ir.WaitSpec{Type: ir.WaitNetworkIdle, Timeout: 30000},
		}
	}

	return nil
}

func sanitizeName(s string) string {
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, " ", "_")
	var result []rune
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' {
			result = append(result, r)
		}
	}
	name := string(result)
	if len(name) > 50 {
		name = name[:50]
	}
	if name == "" {
		name = "workflow"
	}
	return name
}
