// Package usage provides pricing utilities for model usage.
package usage

// ModelPricing defines the pricing for input and output tokens.
// Prices are in microdollars per token (1 microdollar = $0.000001).
// For example: $15/1M tokens = 15 microdollars per token.
type ModelPricing struct {
	InputPriceMicros  int64 // Price per input token in microdollars
	OutputPriceMicros int64 // Price per output token in microdollars
}

// PricingConfig holds pricing for all supported models.
type PricingConfig struct {
	Models   map[string]ModelPricing
	Fallback ModelPricing
}

// DefaultPricingConfig returns the default pricing configuration.
// Currently supports only Claude Opus 4.5 and Opus 4.6.
// Pricing based on Anthropic API pricing (as of 2024):
// - claude-opus-4-5: $15/1M input, $75/1M output
// - claude-opus-4-6: $15/1M input, $75/1M output (assumed same as 4.5)
// Fallback: $0 for unknown models until explicitly mapped.
func DefaultPricingConfig() PricingConfig {
	return PricingConfig{
		Models: map[string]ModelPricing{
			// Claude Opus 4.5: $15/1M input, $75/1M output
			"claude-opus-4-5":           {InputPriceMicros: 15, OutputPriceMicros: 75},
			"claude-opus-4.5":           {InputPriceMicros: 15, OutputPriceMicros: 75},
			"claude-4.5-opus":           {InputPriceMicros: 15, OutputPriceMicros: 75},
			"claude-4-5-opus":           {InputPriceMicros: 15, OutputPriceMicros: 75},
			"anthropic/claude-opus-4.5": {InputPriceMicros: 15, OutputPriceMicros: 75},

			// Claude Opus 4.6: $15/1M input, $75/1M output (assumed)
			"claude-opus-4-6":           {InputPriceMicros: 15, OutputPriceMicros: 75},
			"claude-opus-4.6":           {InputPriceMicros: 15, OutputPriceMicros: 75},
			"claude-4.6-opus":           {InputPriceMicros: 15, OutputPriceMicros: 75},
			"claude-4-6-opus":           {InputPriceMicros: 15, OutputPriceMicros: 75},
			"anthropic/claude-opus-4.6": {InputPriceMicros: 15, OutputPriceMicros: 75},
		},
		// Fallback pricing for unknown models (intentionally zero)
		Fallback: ModelPricing{
			InputPriceMicros:  0,
			OutputPriceMicros: 0,
		},
	}
}

// GetPricing returns the pricing for a model.
// Falls back to default pricing if model is not found.
func (c *PricingConfig) GetPricing(model string) ModelPricing {
	if pricing, ok := c.Models[model]; ok {
		return pricing
	}
	return c.Fallback
}

// CalculatePrice calculates the price for token usage.
// Returns the total price in microdollars.
func (c *PricingConfig) CalculatePrice(model string, promptTokens, completionTokens int) (inputMicros, outputMicros, totalMicros int64) {
	pricing := c.GetPricing(model)
	inputMicros = int64(promptTokens) * pricing.InputPriceMicros
	outputMicros = int64(completionTokens) * pricing.OutputPriceMicros
	totalMicros = inputMicros + outputMicros
	return
}
