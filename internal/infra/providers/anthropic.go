package providers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"

	"github.com/ninoamine/agent-factory/internal/app/execution"
	"github.com/ninoamine/agent-factory/internal/app/metadata"
)

// anthropicAdapter talks to the Anthropic Messages API (PROV-13, 14, 22, 33).
type anthropicAdapter struct{ *base }

var (
	_ execution.Adapter   = (*anthropicAdapter)(nil)
	_ execution.Streamer  = (*anthropicAdapter)(nil)
	_ execution.Canceller = (*anthropicAdapter)(nil)
	_ MetadataAdapter     = (*anthropicAdapter)(nil)
)

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type anthropicRequest struct {
	Model     string        `json:"model"`
	MaxTokens int           `json:"max_tokens"`
	Stream    bool          `json:"stream"`
	Messages  []chatMessage `json:"messages"`
}

func (a *anthropicAdapter) headers(r *http.Request) {
	if a.apiKey != "" {
		r.Header.Set("x-api-key", a.apiKey)
	}
	r.Header.Set("anthropic-version", "2023-06-01")
}

// Execute implements execution.Adapter by streaming and collecting the text.
func (a *anthropicAdapter) Execute(ctx context.Context, modelID, prompt string) (execution.Response, error) {
	return a.Stream(ctx, modelID, prompt, func(string) {})
}

type anthropicEvent struct {
	Type    string `json:"type"`
	Message *struct {
		Usage struct {
			InputTokens *int `json:"input_tokens"`
		} `json:"usage"`
	} `json:"message"`
	Delta *struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"delta"`
	Usage *struct {
		OutputTokens *int `json:"output_tokens"`
	} `json:"usage"`
	Error json.RawMessage `json:"error"`
}

// Stream implements execution.Streamer (PROV-12, PROV-22).
func (a *anthropicAdapter) Stream(ctx context.Context, modelID, prompt string, onChunk func(string)) (execution.Response, error) {
	m := a.info(modelID)
	max := defaultMaxTokens // PROV-14
	if m.maxTokens != nil {
		max = *m.maxTokens
	}
	body := anthropicRequest{Model: m.name, MaxTokens: max, Stream: true,
		Messages: []chatMessage{{Role: "user", Content: prompt}}} // PROV-17
	req, err := a.do(ctx, http.MethodPost, "/v1/messages", body, a.headers)
	if err != nil {
		return execution.Response{}, err
	}
	defer req.finish()
	if !ok2xx(req.resp.StatusCode) {
		return execution.Response{}, a.statusErr(req, m)
	}

	var text strings.Builder
	var in, out *int
	sc := lines(req.body)
	for sc.Scan() {
		line := sc.Text()
		data, isData := strings.CutPrefix(line, "data:")
		if !isData {
			continue // event names, comments, blank lines
		}
		var ev anthropicEvent
		if err := json.Unmarshal([]byte(strings.TrimSpace(data)), &ev); err != nil {
			return execution.Response{Text: text.String()}, a.streamFailed("invalid stream data")
		}
		switch ev.Type {
		case "message_start":
			if ev.Message != nil {
				in = ev.Message.Usage.InputTokens
			}
		case "content_block_delta":
			if ev.Delta != nil && ev.Delta.Type == "text_delta" && ev.Delta.Text != "" {
				text.WriteString(ev.Delta.Text)
				onChunk(ev.Delta.Text)
			}
		case "message_delta":
			if ev.Usage != nil && ev.Usage.OutputTokens != nil {
				out = ev.Usage.OutputTokens
			}
		case "message_stop":
			return execution.Response{Text: text.String(), Usage: makeUsage(in, out, nil)}, nil
		case "error":
			return execution.Response{Text: text.String()}, a.streamFailed(providerMessage(data2err(ev.Error)))
		}
	}
	return execution.Response{Text: text.String()}, a.endErr(req, sc.Err())
}

// data2err wraps an error value so providerMessage can read it.
func data2err(e json.RawMessage) []byte {
	return append(append([]byte(`{"error":`), e...), '}')
}

// FetchMetadata implements MetadataAdapter (PROV-33, PROV-37): it checks that
// the model exists. The endpoint has no capabilities, speed or pricing, so
// nothing is invented (META-4).
func (a *anthropicAdapter) FetchMetadata(ctx context.Context, modelID string) (metadata.APIMetadata, error) {
	m := a.info(modelID)
	req, err := a.do(ctx, http.MethodGet, "/v1/models/"+url.PathEscape(m.name), nil, a.headers)
	if err != nil {
		return metadata.APIMetadata{}, err
	}
	defer req.finish()
	if !ok2xx(req.resp.StatusCode) {
		return metadata.APIMetadata{}, a.statusErr(req, m)
	}
	return metadata.APIMetadata{}, nil
}
