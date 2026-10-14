package assemble

// collectRequired adds every name in a "required" array, at any depth of v, to into.
func collectRequired(v any, into map[string]bool) {
	switch v := v.(type) {
	case map[string]any:
		for k, x := range v {
			if names, ok := x.([]any); ok && k == "required" {
				for _, n := range names {
					if s, ok := n.(string); ok {
						into[s] = true
					}
				}
				continue
			}
			collectRequired(x, into)
		}
	case []any:
		for _, x := range v {
			collectRequired(x, into)
		}
	}
}
