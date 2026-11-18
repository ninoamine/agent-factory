package providers

import "github.com/ninoamine/agent-factory/internal/app/execution"

// makeUsage builds usage from what the provider returned. The total is the
// provider's own, else input + output when both are known, else unavailable
// (PROV-25). A missing count is never invented (EXEC-5).
func makeUsage(in, out, total *int) execution.Usage {
	u := execution.Usage{Input: in, Output: out, Total: total}
	if u.Total == nil && in != nil && out != nil {
		t := *in + *out
		u.Total = &t
	}
	return u
}
