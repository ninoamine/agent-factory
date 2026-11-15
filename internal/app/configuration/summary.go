package configuration

import (
	"fmt"
	"sort"
	"strings"
)

// Summary renders the configuration summary (CFG-7). It reads only names,
// never ProviderEntry.APIKey (SEC-1).
func (c Config) Summary() string {
	var b strings.Builder
	fmt.Fprintf(&b, "Configuration directory: %s\n", c.Dir)
	if c.Models == nil {
		b.WriteString("Providers: none loaded\nModels: none loaded\n")
	} else {
		names := make([]string, 0, len(c.Models.Providers))
		for n := range c.Models.Providers {
			names = append(names, n)
		}
		sort.Strings(names)
		fmt.Fprintf(&b, "Providers (%d): %s\n", len(names), strings.Join(names, ", "))
		ids := make([]string, 0, len(c.Models.Models))
		for _, m := range c.Models.Models {
			ids = append(ids, m.ID)
		}
		fmt.Fprintf(&b, "Models (%d): %s\n", len(ids), strings.Join(ids, ", "))
	}
	if c.Strategies == nil {
		b.WriteString("Strategy: none loaded\nEnabled commands: none\n")
		return b.String()
	}
	fmt.Fprintf(&b, "Strategy: %s\n", c.Strategies.Strategy)
	var enabled []string
	for _, name := range RoutingCommands {
		a, ok := c.Strategies.Commands[name]
		if !ok || !a.Enabled {
			continue
		}
		entry := fmt.Sprintf("%s -> %s", name, strings.Join(a.Models, ", "))
		if reason, bad := c.Unusable[name]; bad {
			entry += " (unusable: " + reason + ")"
		}
		enabled = append(enabled, entry)
	}
	if len(enabled) == 0 {
		b.WriteString("Enabled commands: none\n")
	} else {
		b.WriteString("Enabled commands:\n")
		for _, e := range enabled {
			b.WriteString("  " + e + "\n")
		}
	}
	return b.String()
}
