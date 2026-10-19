package dto

// ModelProbeResponse is the body of POST /api/v1/models/{modelId}/test.
type ModelProbeResponse struct {
	OK        bool  `json:"ok"`
	LatencyMS int64 `json:"latencyMs"`
}
