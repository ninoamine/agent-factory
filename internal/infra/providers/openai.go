package providers

import (
	"context"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/ninoamine/agent-factory/internal/app/configuration"
	"github.com/ninoamine/agent-factory/internal/app/execution"
	"github.com/ninoamine/agent-factory/internal/app/metadata"
)

// openaiAdapter talks to any server with an OpenAI-style /chat/completions
// and /models API (PROV-15, 23, 34).
type openaiAdapter struct {
	*base

	listMu   sync.Mutex
	listDone bool
	list     map[string]*configuration.Pricing // by API model name; nil value: no pricing
	listErr  error
}

var (
	_ execution.Adapter   = (*openaiAdapter)(nil)
	_ execution.Streamer  = (*openaiAdapter)(nil)
	_ execution.Canceller = (*openaiAdapter)(nil)
	_ MetadataAdapter     = (*openaiAdapter)(nil)
)

type openaiRequest struct {
	Model         string         `json:"model"`
	Stream        bool           `json:"stream"`
	StreamOptions map[string]any `json:"stream_options"`
	Messages      []chatMessage  `json:"messages"`
	MaxTokens     *int           `json:"max_tokens,omitempty"`
}

func (a *openaiAdapter) headers(r *http.Request) {
	if a.apiKey != "" {
		r.Header.Set("Authorization", "Bearer "+a.apiKey)
	}
}

// Execute implements execution.Adapter by streaming and collecting the text.
func (a *openaiAdapter) Execute(ctx context.Context, modelID, prompt string) (execution.Response, error) {
	return a.Stream(ctx, modelID, prompt, func(string) {})
}

type openaiChunk struct {
	Choices []struct {
		Delta struct {
			Content *string `json:"content"`
		} `json:"delta"`
	} `json:"choices"`
	Usage *struct {
		PromptTokens     *int `json:"prompt_tokens"`
		CompletionTokens *int `json:"completion_tokens"`
		TotalTokens      *int `json:"total_tokens"`
	} `json:"usage"`
	Error json.RawMessage `json:"error"`
}

// Stream implements execution.Streamer (PROV-12, PROV-23).
func (a *openaiAdapter) Stream(ctx context.Context, modelID, prompt string, onChunk func(string)) (execution.Response, error) {
	m := a.info(modelID)
	body := openaiRequest{Model: m.name, Stream: true,
		StreamOptions: map[string]any{"include_usage": true},          // PROV-15; never re-sent without it
		Messages:      []chatMessage{{Role: "user", Content: prompt}}, // PROV-17
		MaxTokens:     m.maxTokens}
	req, err := a.do(ctx, http.MethodPost, "/chat/completions", body, a.headers)
	if err != nil {
		return execution.Response{}, err
	}
	defer req.finish()
	if !ok2xx(req.resp.StatusCode) {
		return execution.Response{}, a.statusErr(req, m)
	}

	var text strings.Builder
	var in, out, total *int
	sc := lines(req.body)
	for sc.Scan() {
		data, isData := strings.CutPrefix(sc.Text(), "data:")
		if !isData {
			continue
		}
		data = strings.TrimSpace(data)
		if data == "[DONE]" {
			return execution.Response{Text: text.String(), Usage: makeUsage(in, out, total)}, nil
		}
		var c openaiChunk
		if err := json.Unmarshal([]byte(data), &c); err != nil {
			return execution.Response{Text: text.String()}, a.streamFailed("invalid stream data")
		}
		if len(c.Error) > 0 && string(c.Error) != "null" {
			return execution.Response{Text: text.String()}, a.streamFailed(providerMessage([]byte(data)))
		}
		if len(c.Choices) > 0 && c.Choices[0].Delta.Content != nil && *c.Choices[0].Delta.Content != "" {
			text.WriteString(*c.Choices[0].Delta.Content)
			onChunk(*c.Choices[0].Delta.Content)
		}
		if c.Usage != nil {
			in, out, total = c.Usage.PromptTokens, c.Usage.CompletionTokens, c.Usage.TotalTokens
		}
	}
	return execution.Response{Text: text.String()}, a.endErr(req, sc.Err())
}

const maxModelList = 16 << 20 // a /models list can be large (OpenRouter)

type openaiModelList struct {
	Data []struct {
		ID      string `json:"id"`
		Pricing *struct {
			Prompt     json.RawMessage `json:"prompt"`
			Completion json.RawMessage `json:"completion"`
		} `json:"pricing"`
	} `json:"data"`
}

// FetchMetadata implements MetadataAdapter (PROV-34, PROV-35, PROV-37). The
// list is fetched once per adapter, that is once per load or refresh.
func (a *openaiAdapter) FetchMetadata(ctx context.Context, modelID string) (metadata.APIMetadata, error) {
	m := a.info(modelID)
	list, err := a.models(ctx, m)
	if err != nil {
		return metadata.APIMetadata{}, err
	}
	pricing, found := list[m.name]
	if !found {
		return metadata.APIMetadata{}, fail(execution.KindModelUnavailable, "provider %q has no model %q", a.provider, m.name)
	}
	return metadata.APIMetadata{Pricing: pricing}, nil
}

func (a *openaiAdapter) models(ctx context.Context, m modelInfo) (map[string]*configuration.Pricing, error) {
	a.listMu.Lock()
	defer a.listMu.Unlock()
	if a.listDone {
		return a.list, a.listErr
	}
	list, err := a.fetchList(ctx, m)
	if ctx.Err() == nil { // a cancelled fetch is not cached
		a.listDone, a.list, a.listErr = true, list, err
	}
	return list, err
}

func (a *openaiAdapter) fetchList(ctx context.Context, m modelInfo) (map[string]*configuration.Pricing, error) {
	req, err := a.do(ctx, http.MethodGet, "/models", nil, a.headers)
	if err != nil {
		return nil, err
	}
	defer req.finish()
	if !ok2xx(req.resp.StatusCode) {
		return nil, a.statusErr(req, m)
	}
	raw, err := io.ReadAll(io.LimitReader(req.body, maxModelList))
	if err != nil {
		return nil, a.streamErr(req, err)
	}
	var l openaiModelList
	if err := json.Unmarshal(raw, &l); err != nil {
		return nil, fail(execution.KindProviderFailure, "%q returned an invalid model list", a.provider)
	}
	out := make(map[string]*configuration.Pricing, len(l.Data))
	for _, d := range l.Data {
		var p *configuration.Pricing
		if d.Pricing != nil {
			p = perMillion(d.Pricing.Prompt, d.Pricing.Completion)
		}
		out[d.ID] = p
	}
	return out, nil
}

var decimalRe = regexp.MustCompile(`^(\d+(\.\d*)?|\.\d+)([eE][+-]?\d+)?$`)

// perMillion converts per-token decimal strings to USD per million tokens
// (PROV-34). Both must be non-negative decimal strings, else pricing stays
// unknown (META-4).
func perMillion(prompt, completion json.RawMessage) *configuration.Pricing {
	in, ok1 := parsePrice(prompt)
	out, ok2 := parsePrice(completion)
	if !ok1 || !ok2 {
		return nil
	}
	return &configuration.Pricing{InputPer1M: &in, OutputPer1M: &out}
}

func parsePrice(raw json.RawMessage) (float64, bool) {
	var s string
	if json.Unmarshal(raw, &s) != nil || !decimalRe.MatchString(s) {
		return 0, false
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsInf(v, 0) || math.IsNaN(v) || v < 0 {
		return 0, false
	}
	return math.Round(v*1e6*1e6) / 1e6, true // rounded: 0.000003 -> 3, not 2.9999999999999996
}
