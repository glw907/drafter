package batch

// Usage is the token counts of one response, as the API reports them. The ledger records it
// beside each settlement so a cost can be recomputed from the line alone.
type Usage struct {
	InputTokens  int64 `json:"input_tokens"`
	OutputTokens int64 `json:"output_tokens"`
	CacheWrite5m int64 `json:"cache_write_5m"`
	CacheWrite1h int64 `json:"cache_write_1h"`
	CacheRead    int64 `json:"cache_read"`
}
