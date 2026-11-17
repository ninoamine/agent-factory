package configuration

// AssignedCommands lists the enabled routing commands (in display order)
// whose assignment names the model.
func (c Config) AssignedCommands(modelID string) []string {
	if c.Strategies == nil {
		return nil
	}
	var out []string
	for _, cmd := range RoutingCommands {
		a, ok := c.Strategies.Commands[cmd]
		if !ok || !a.Enabled {
			continue
		}
		for _, id := range a.Models {
			if id == modelID {
				out = append(out, cmd)
				break
			}
		}
	}
	return out
}
