package configuration

import (
	"fmt"
	"net"
	"net/url"
	"sort"
	"strings"
)

// Provider types (PROV-1).
const (
	TypeAnthropic = "anthropic"
	TypeOpenAI    = "openai"
	TypeOllama    = "ollama"
)

// Provider timeout bounds in seconds (PROV-7).
const (
	MinTimeoutSeconds = 1
	MaxTimeoutSeconds = 3600
)

var defaultBaseURLs = map[string]string{
	TypeAnthropic: "https://api.anthropic.com",
	TypeOpenAI:    "https://api.openai.com/v1",
	TypeOllama:    "http://localhost:11434",
}

// EffectiveBaseURL returns the configured base URL, or the default of the
// type, without a trailing slash (PROV-2, PROV-3).
func (p ProviderEntry) EffectiveBaseURL() string {
	u := p.BaseURL
	if u == "" {
		u = defaultBaseURLs[p.Type]
	}
	return strings.TrimRight(u, "/")
}

// validate checks the provider entry; path is its field path (PROV-1, 3, 7).
func (p ProviderEntry) validate(path string) []FieldError {
	var errs []FieldError
	if _, ok := defaultBaseURLs[p.Type]; !ok {
		errs = append(errs, FieldError{path + ".type", "must be one of anthropic, openai, ollama"})
	}
	if p.BaseURL != "" {
		if r := checkBaseURL(p.BaseURL); r != "" {
			errs = append(errs, FieldError{path + ".baseUrl", r})
		}
	}
	if t := p.TimeoutSeconds; t != nil && (*t < MinTimeoutSeconds || *t > MaxTimeoutSeconds) {
		errs = append(errs, FieldError{path + ".timeoutSeconds",
			fmt.Sprintf("must be an integer from %d to %d", MinTimeoutSeconds, MaxTimeoutSeconds)})
	}
	return errs
}

func checkBaseURL(s string) string {
	const want = "must be an absolute http or https URL with a host, without query or fragment"
	u, err := url.Parse(s)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.Hostname() == "" ||
		strings.ContainsAny(s, "?#") {
		return want
	}
	return ""
}

// providerWarnings returns load-time warnings, sorted by provider name. They
// never contain an API key (SEC-1).
func (m *ModelsFile) providerWarnings() []string {
	names := make([]string, 0, len(m.Providers))
	for n := range m.Providers {
		names = append(names, n)
	}
	sort.Strings(names)
	var out []string
	for _, n := range names {
		p := m.Providers[n]
		missingKey := p.APIKey == "" &&
			(p.Type == TypeAnthropic || (p.Type == TypeOpenAI && p.BaseURL == "")) // PROV-5
		if missingKey {
			out = append(out, fmt.Sprintf("provider %q (%s) has no apiKey: its requests will be rejected. Set providers.%s.apiKey in models.json.", n, p.Type, n))
		}
		if p.APIKey != "" { // PROV-6
			if u, err := url.Parse(p.EffectiveBaseURL()); err == nil && u.Scheme == "http" && !isLoopback(u.Hostname()) {
				out = append(out, fmt.Sprintf("provider %q sends its apiKey over unencrypted http to %s: use an https baseUrl.", n, u.Hostname()))
			}
		}
	}
	return out
}

func isLoopback(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
