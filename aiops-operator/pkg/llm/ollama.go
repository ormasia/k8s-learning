package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type Result struct {
	// 直接放 LLM 输出（严格 JSON），由控制器写到 status.proposedPatch
	Actions      []map[string]any `json:"actions"`
	Risks        []string         `json:"risks,omitempty"`
	RollbackPlan []string         `json:"rollbackPlan,omitempty"`
}

type Client struct {
	BaseURL string
	Model   string
	HTTP    *http.Client
	Schema  map[string]any
}

func New(base, model string, schema map[string]any) *Client {
	return &Client{
		BaseURL: base,
		Model:   model,
		HTTP:    &http.Client{Timeout: 500 * time.Second},
		Schema:  schema,
	}
}

func (c *Client) Propose(ctx context.Context, sys string, evidenceJSON []byte) (*Result, error) {
	payload := map[string]any{
		"model": c.Model,
		"messages": []map[string]string{
			{"role": "system", "content": sys},
			{"role": "user", "content": string(evidenceJSON)},
		},
		"format": c.Schema, // structured outputs
		"stream": false,
	}
	b, _ := json.Marshal(payload)
	req, _ := http.NewRequestWithContext(ctx, "POST", c.BaseURL+"/api/chat", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var out struct {
		Message struct{ Content string } `json:"message"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	var res Result
	if err := json.Unmarshal([]byte(out.Message.Content), &res); err != nil {
		return nil, fmt.Errorf("llm returned non-JSON or schema-mismatch: %w", err)
	}
	return &res, nil
}

func DefaultSchema() map[string]any {
	// 可按需加正则限制，比如禁止 :latest
	var s map[string]any
	_ = json.Unmarshal([]byte(`{
	  "type":"object",
	  "properties":{
	    "actions":{"type":"array","items":{
	      "type":"object",
	      "properties":{
	        "kind":{"type":"string","enum":["Patch"]},
	        "strategy":{"type":"string","enum":["ServerSideApply"]},
	        "objectRef":{"type":"object","properties":{
	          "apiVersion":{"type":"string"},
	          "kind":{"type":"string"},
	          "namespace":{"type":"string"},
	          "name":{"type":"string"}
	        },"required":["apiVersion","kind","namespace","name"]},
	        "patch":{"type":"object"}
	      },
	      "required":["kind","strategy","objectRef","patch"]
	    }},
	    "risks":{"type":"array","items":{"type":"string"}},
	    "rollbackPlan":{"type":"array","items":{"type":"string"}}
	  },
	  "required":["actions"]
	}`), &s)
	return s
}
