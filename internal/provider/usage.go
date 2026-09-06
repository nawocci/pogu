package provider

import (
	"bytes"
	"encoding/json"
	"time"

	"github.com/nawocci/pogu/internal/telemetry"
)

func decodeFirstJSON(v any, body []byte) bool {
	start := bytes.IndexByte(body, '{')
	if start < 0 {
		return false
	}
	return json.NewDecoder(bytes.NewReader(body[start:])).Decode(v) == nil
}

func ExtractUnaryUsage(protocol string, body []byte) telemetry.TokenUsage {
	var payload struct {
		Usage *struct {
			PromptTokens     *int64 `json:"prompt_tokens"`
			CompletionTokens *int64 `json:"completion_tokens"`
			TotalTokens      *int64 `json:"total_tokens"`
			InputTokens      *int64 `json:"input_tokens"`
			OutputTokens     *int64 `json:"output_tokens"`
		} `json:"usage"`
	}
	if !decodeFirstJSON(&payload, body) || payload.Usage == nil {
		return telemetry.TokenUsage{}
	}
	return usageFromFields(protocol, payload.Usage.PromptTokens, payload.Usage.CompletionTokens, payload.Usage.TotalTokens, payload.Usage.InputTokens, payload.Usage.OutputTokens)
}

func ExtractUpstreamModel(body []byte) (string, bool) {
	var payload struct {
		Model *string `json:"model"`
	}
	if !decodeFirstJSON(&payload, body) || payload.Model == nil || *payload.Model == "" {
		return "", false
	}
	return *payload.Model, true
}

type SSEUsageCollector struct {
	protocol string
	obtained bool
	input    *int64
	output   *int64
	total    *int64
	first    time.Time
	hasFirst bool
	model    string
	hasModel bool
	OnFirst  func()
	notified bool
}

func NewSSEUsageCollector(protocol string) *SSEUsageCollector {
	return &SSEUsageCollector{protocol: protocol}
}

func (c *SSEUsageCollector) Observe(data []byte) {
	if !c.hasFirst {
		c.hasFirst = true
		c.first = time.Now()
	}
	var payload struct {
		Model *string `json:"model"`
		Usage *struct {
			PromptTokens     *int64 `json:"prompt_tokens"`
			CompletionTokens *int64 `json:"completion_tokens"`
			TotalTokens      *int64 `json:"total_tokens"`
			InputTokens      *int64 `json:"input_tokens"`
			OutputTokens     *int64 `json:"output_tokens"`
		} `json:"usage"`
		Message *struct {
			Model *string `json:"model"`
			Usage *struct {
				InputTokens  *int64 `json:"input_tokens"`
				OutputTokens *int64 `json:"output_tokens"`
			} `json:"usage"`
		} `json:"message"`
		Delta *struct {
			Usage *struct {
				OutputTokens *int64 `json:"output_tokens"`
			} `json:"usage"`
		} `json:"delta"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return
	}
	if !c.hasModel {
		if payload.Model != nil && *payload.Model != "" {
			c.model, c.hasModel = *payload.Model, true
		} else if payload.Message != nil && payload.Message.Model != nil && *payload.Message.Model != "" {
			c.model, c.hasModel = *payload.Message.Model, true
		}
	}
	if payload.Usage != nil {
		u := usageFromFields(c.protocol, payload.Usage.PromptTokens, payload.Usage.CompletionTokens, payload.Usage.TotalTokens, payload.Usage.InputTokens, payload.Usage.OutputTokens)
		hasOutput := u.Output != nil
		c.merge(u, hasOutput)
	}
	if payload.Message != nil && payload.Message.Usage != nil {
		c.merge(telemetry.TokenUsage{Input: payload.Message.Usage.InputTokens, Output: payload.Message.Usage.OutputTokens}, payload.Message.Usage.OutputTokens != nil)
	}
	if payload.Delta != nil && payload.Delta.Usage != nil && payload.Delta.Usage.OutputTokens != nil {
		c.output = payload.Delta.Usage.OutputTokens
		c.obtained = true
	}
	if !c.notified {
		c.notified = true
		if c.OnFirst != nil {
			c.OnFirst()
		}
	}
}

func (c *SSEUsageCollector) merge(u telemetry.TokenUsage, hasOutput bool) {
	if u.Input != nil {
		c.input = u.Input
		c.obtained = true
	}
	if hasOutput && u.Output != nil {
		c.output = u.Output
		c.obtained = true
	}
	if u.Total != nil {
		c.total = u.Total
		c.obtained = true
	}
}

func (c *SSEUsageCollector) Usage() telemetry.TokenUsage {
	if !c.obtained {
		return telemetry.TokenUsage{}
	}
	u := telemetry.TokenUsage{Input: c.input, Output: c.output, Total: c.total}
	if u.Total == nil && u.Input != nil && u.Output != nil {
		total := *u.Input + *u.Output
		u.Total = &total
	}
	return u
}

func (c *SSEUsageCollector) FirstByte() (time.Time, bool) {
	if c == nil || !c.hasFirst {
		return time.Time{}, false
	}
	return c.first, true
}

func (c *SSEUsageCollector) FirstModel() (string, bool) {
	if c == nil || !c.hasModel {
		return "", false
	}
	return c.model, true
}

func usageFromFields(protocol string, prompt, completion, total, input, output *int64) telemetry.TokenUsage {
	u := telemetry.TokenUsage{}
	if protocol == "anthropic" {
		u.Input, u.Output = input, output
	} else {
		u.Input, u.Output, u.Total = prompt, completion, total
	}
	if u.Total == nil && u.Input != nil && u.Output != nil {
		t := *u.Input + *u.Output
		u.Total = &t
	}
	return u
}
