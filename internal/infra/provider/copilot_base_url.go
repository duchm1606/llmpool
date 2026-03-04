package provider

import (
	"strings"
)

const defaultCopilotBaseURL = "https://api.githubcopilot.com"

// CopilotBaseURLForAccountType resolves the Copilot API base URL from account type.
// Reference behavior:
// - individual -> https://api.githubcopilot.com
// - <type>     -> https://api.<type>.githubcopilot.com
func CopilotBaseURLForAccountType(accountType string) string {
	normalized := strings.ToLower(strings.TrimSpace(accountType))
	if normalized == "" || normalized == "individual" {
		return defaultCopilotBaseURL
	}

	return "https://api." + normalized + ".githubcopilot.com"
}

// NormalizeEnterpriseURL strips the scheme (http:// or https://) and trailing slash
// from an enterprise URL, returning just the domain.
// Examples:
//   - "https://company.ghe.com" -> "company.ghe.com"
//   - "http://github.acme.corp/" -> "github.acme.corp"
//   - "company.ghe.com" -> "company.ghe.com"
func NormalizeEnterpriseURL(url string) string {
	normalized := strings.TrimSpace(url)
	// Strip scheme
	normalized = strings.TrimPrefix(normalized, "https://")
	normalized = strings.TrimPrefix(normalized, "http://")
	// Strip trailing slash
	normalized = strings.TrimSuffix(normalized, "/")
	return normalized
}

// CopilotBaseURLForEnterprise returns the Copilot API base URL for a GitHub Enterprise Server.
// Reference behavior (opencode copilot.ts):
//   - Enterprise URL provided: https://copilot-api.<normalized_domain>
//
// Example: "https://github.acme.corp" -> "https://copilot-api.github.acme.corp"
func CopilotBaseURLForEnterprise(enterpriseURL string) string {
	normalized := NormalizeEnterpriseURL(enterpriseURL)
	if normalized == "" {
		return defaultCopilotBaseURL
	}
	return "https://copilot-api." + normalized
}

// ResolveCopilotBaseURL resolves the Copilot API base URL using the following precedence:
// 1. If enterpriseURL is set, use https://copilot-api.<normalized_domain>
// 2. Otherwise, use account type-based URL
//
// This matches the opencode reference behavior where enterprise URL takes precedence.
func ResolveCopilotBaseURL(enterpriseURL, accountType string) string {
	enterpriseURL = strings.TrimSpace(enterpriseURL)
	if enterpriseURL != "" {
		return CopilotBaseURLForEnterprise(enterpriseURL)
	}
	return CopilotBaseURLForAccountType(accountType)
}
