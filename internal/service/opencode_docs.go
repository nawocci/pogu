package service

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const OpenCodeDocsURL = "https://raw.githubusercontent.com/anomalyco/opencode/refs/heads/dev/packages/web/src/content/docs/zen.mdx"

var openCodeDocsHTTPClient = &http.Client{Timeout: 20 * time.Second}

const openCodeDocsMaxBytes = 1 << 20

func (s *Service) openCodeDocsURL() string {
	if s.OpenCodeDocsURL != "" {
		return s.OpenCodeDocsURL
	}
	return OpenCodeDocsURL
}

// DocsModels is the free-model signal parsed out of the Zen docs page:
// every model ID the pricing table marks Free, mapped to its endpoint URL
// from the endpoints table.
type DocsModels struct {
	Endpoints map[string]string
}

func (d DocsModels) FreeIDs() map[string]bool {
	out := make(map[string]bool, len(d.Endpoints))
	for id := range d.Endpoints {
		out[id] = true
	}
	return out
}

func fetchOpenCodeDocs(ctx context.Context, docsURL string) (DocsModels, error) {
	var out DocsModels
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, docsURL, nil)
	if err != nil {
		return out, err
	}
	resp, err := openCodeDocsHTTPClient.Do(req)
	if err != nil {
		return out, fmt.Errorf("fetch OpenCode docs: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return out, fmt.Errorf("fetch OpenCode docs: status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, openCodeDocsMaxBytes))
	if err != nil {
		return out, fmt.Errorf("read OpenCode docs: %w", err)
	}
	return parseOpenCodeDocs(body)
}

func parseOpenCodeDocs(body []byte) (DocsModels, error) {
	out := DocsModels{Endpoints: map[string]string{}}
	text := string(body)
	endpointHeader, endpointRows := findMarkdownTable(text, "## Endpoints")
	nameCol := headerIndex(endpointHeader, "Model")
	idCol := headerIndex(endpointHeader, "Model ID")
	endpointCol := headerIndex(endpointHeader, "Endpoint")
	if nameCol < 0 || idCol < 0 || endpointCol < 0 {
		return out, fmt.Errorf("decode OpenCode docs: endpoints table not found")
	}
	names := map[string]string{}
	for _, row := range endpointRows {
		name, id, endpoint := cellAt(row, nameCol), cellAt(row, idCol), cellAt(row, endpointCol)
		if name == "" || id == "" {
			continue
		}
		names[name] = id
		if endpoint != "" {
			out.Endpoints[id] = strings.Trim(endpoint, "`")
		} else {
			out.Endpoints[id] = ""
		}
	}
	pricingHeader, pricingRows := findMarkdownTable(text, "## Pricing")
	pricingName := headerIndex(pricingHeader, "Model")
	pricingInput := headerIndex(pricingHeader, "Input")
	pricingOutput := headerIndex(pricingHeader, "Output")
	if pricingName < 0 || pricingInput < 0 || pricingOutput < 0 {
		return out, fmt.Errorf("decode OpenCode docs: pricing table not found")
	}
	free := map[string]bool{}
	for _, row := range pricingRows {
		name := cellAt(row, pricingName)
		if name == "" || !strings.EqualFold(cellAt(row, pricingInput), "Free") || !strings.EqualFold(cellAt(row, pricingOutput), "Free") {
			continue
		}
		id, ok := names[name]
		if !ok || id == "" {
			continue
		}
		free[id] = true
	}
	if len(free) == 0 {
		return out, fmt.Errorf("decode OpenCode docs: no free models parsed")
	}
	filtered := map[string]string{}
	for id := range free {
		filtered[id] = out.Endpoints[id]
	}
	out.Endpoints = filtered
	return out, nil
}

// findMarkdownTable locates the first pipe table following the given ##
// section and returns its header plus data rows.
func findMarkdownTable(text, section string) ([]string, [][]string) {
	idx := strings.Index(text, section)
	if idx < 0 {
		return nil, nil
	}
	rest := text[idx+len(section):]
	if next := strings.Index(rest, "\n## "); next >= 0 {
		rest = rest[:next]
	}
	var header []string
	var rows [][]string
	inTable := false
	for _, line := range strings.Split(rest, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "|") {
			if inTable {
				break
			}
			continue
		}
		cells := splitMarkdownRow(line)
		if isMarkdownSeparator(cells) {
			inTable = true
			continue
		}
		if !inTable {
			header = cells
			continue
		}
		rows = append(rows, cells)
	}
	if len(header) == 0 {
		return nil, nil
	}
	return header, rows
}

func headerIndex(header []string, name string) int {
	for i, h := range header {
		if strings.EqualFold(strings.TrimSpace(h), name) {
			return i
		}
	}
	return -1
}

func cellAt(row []string, idx int) string {
	if idx < 0 || idx >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[idx])
}

func splitMarkdownRow(line string) []string {
	line = strings.TrimSpace(line)
	line = strings.TrimPrefix(line, "|")
	line = strings.TrimSuffix(line, "|")
	parts := strings.Split(line, "|")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return parts
}

func isMarkdownSeparator(cells []string) bool {
	if len(cells) == 0 {
		return false
	}
	for _, c := range cells {
		t := strings.Trim(c, " :-")
		if t != "" {
			return false
		}
		if !strings.Contains(c, "-") {
			return false
		}
	}
	return true
}

// SchemeForDocsEndpoint maps a docs endpoint URL to the model wire scheme.
// Unknown or empty endpoints inherit (""), leaving routing to the default.
func SchemeForDocsEndpoint(endpoint string) string {
	lower := strings.ToLower(strings.TrimSpace(strings.Trim(endpoint, "`")))
	switch {
	case strings.HasSuffix(lower, "/v1/responses"):
		return string(SchemeOpenAIResponses)
	case strings.HasSuffix(lower, "/v1/messages"):
		return string(SchemeAnthropic)
	default:
		return ""
	}
}
