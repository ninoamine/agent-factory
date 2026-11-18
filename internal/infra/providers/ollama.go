package providers

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/ninoamine/agent-factory/internal/app/execution"
	"github.com/ninoamine/agent-factory/internal/app/metadata"
)

// ollamaAdapter talks to the Ollama native API (PROV-16, 24, 36).
type ollamaAdapter struct{ *base }

var (
	_ execution.Adapter   = (*ollamaAdapter)(nil)
	_ execution.Streamer  = (*ollamaAdapter)(nil)
	_ execution.Canceller = (*ollamaAdapter)(nil)
	_ MetadataAdapter     = (*ollamaAdapter)(nil)
)

type ollamaRequest struct {
	Model    string         `json:"model"`
	Stream   bool           `json:"stream"`
	Messages []chatMessage  `json:"messages"`
	Options  *ollamaOptions `json:"options,omitempty"`
}

type ollamaOptions struct {
	NumPredict int `json:"num_predict"`
}

func (a *ollamaAdapter) headers(r *http.Request) {
	if a.apiKey != "" {
		r.Header.Set("Authorization", "Bearer "+a.apiKey)
	}
}

// Execute implements execution.Adapter by streaming and collecting the text.
func (a *ollamaAdapter) Execute(ctx context.Context, modelID, prompt string) (execution.Response, error) {
	return a.Stream(ctx, modelID, prompt, func(string) {})
}

type ollamaLine struct {
	Message *struct {
		Content string `json:"content"`
	} `json:"message"`
	Done            bool            `json:"done"`
	PromptEvalCount *int            `json:"prompt_eval_count"`
	EvalCount       *int            `json:"eval_count"`
	Error           json.RawMessage `json:"error"`
}

// Stream implements execution.Streamer (PROV-12, PROV-24).
func (a *ollamaAdapter) Stream(ctx context.Context, modelID, prompt string, onChunk func(string)) (execution.Response, error) {
	m := a.info(modelID)
	body := ollamaRequest{Model: m.name, Stream: true,
		Messages: []chatMessage{{Role: "user", Content: prompt}}} // PROV-17
	if m.maxTokens != nil {
		body.Options = &ollamaOptions{NumPredict: *m.maxTokens}
	}
	req, err := a.do(ctx, http.MethodPost, "/api/chat", body, a.headers)
	if err != nil {
		return execution.Response{}, err
	}
	defer req.finish()
	if !ok2xx(req.resp.StatusCode) {
		return execution.Response{}, a.statusErr(req, m)
	}

	var text strings.Builder
	sc := lines(req.body)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var l ollamaLine
		if err := json.Unmarshal([]byte(line), &l); err != nil {
			return execution.Response{Text: text.String()}, a.streamFailed("invalid stream data")
		}
		if len(l.Error) > 0 && string(l.Error) != "null" {
			return execution.Response{Text: text.String()}, a.streamFailed(providerMessage([]byte(line)))
		}
		if l.Message != nil && l.Message.Content != "" {
			text.WriteString(l.Message.Content)
			onChunk(l.Message.Content)
		}
		if l.Done {
			return execution.Response{Text: text.String(), Usage: makeUsage(l.PromptEvalCount, l.EvalCount, nil)}, nil
		}
	}
	return execution.Response{Text: text.String()}, a.endErr(req, sc.Err())
}

// FetchMetadata implements MetadataAdapter (PROV-36, PROV-37): it checks that
// the model exists locally; nothing else is returned (META-4).
func (a *ollamaAdapter) FetchMetadata(ctx context.Context, modelID string) (metadata.APIMetadata, error) {
	m := a.info(modelID)
	req, err := a.do(ctx, http.MethodPost, "/api/show", map[string]string{"model": m.name}, a.headers)
	if err != nil {
		return metadata.APIMetadata{}, err
	}
	defer req.finish()
	if !ok2xx(req.resp.StatusCode) {
		return metadata.APIMetadata{}, a.statusErr(req, m)
	}
	return metadata.APIMetadata{}, nil
}
