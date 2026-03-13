// Package ollama defines domain entities for Ollama-compatible API responses.
// These types mirror the Ollama wire format so that VS Code extensions
// (Continue, etc.) can discover models via /api/tags and /api/version.
package ollama

import "time"

// TagsResponse is the Ollama /api/tags response listing available models.
type TagsResponse struct {
	Models []ModelInfo `json:"models"`
}

// ModelInfo describes a single model in the Ollama tags format.
type ModelInfo struct {
	Name       string       `json:"name"`
	Model      string       `json:"model"`
	ModifiedAt time.Time    `json:"modified_at"`
	Size       int64        `json:"size"`
	Digest     string       `json:"digest"`
	Details    ModelDetails `json:"details"`
}

// ModelDetails contains metadata about a model.
type ModelDetails struct {
	ParentModel       string   `json:"parent_model"`
	Format            string   `json:"format"`
	Family            string   `json:"family"`
	Families          []string `json:"families"`
	ParameterSize     string   `json:"parameter_size"`
	QuantizationLevel string   `json:"quantization_level"`
}

// VersionResponse is the Ollama /api/version response.
type VersionResponse struct {
	Version string `json:"version"`
}

// ShowRequest is the Ollama /api/show request body.
type ShowRequest struct {
	Model   string `json:"model"`
	Verbose bool   `json:"verbose,omitempty"`
}

// ShowResponse is the Ollama /api/show response.
type ShowResponse struct {
	Modelfile  string         `json:"modelfile"`
	Parameters string         `json:"parameters"`
	Template   string         `json:"template"`
	Details    ModelDetails   `json:"details"`
	ModelInfo  map[string]any `json:"model_info"`
	ModifiedAt time.Time      `json:"modified_at"`
}
