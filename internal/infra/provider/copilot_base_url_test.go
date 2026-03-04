package provider

import "testing"

func TestCopilotBaseURLForAccountType(t *testing.T) {
	tests := []struct {
		name        string
		accountType string
		want        string
	}{
		{
			name:        "empty defaults to individual",
			accountType: "",
			want:        "https://api.githubcopilot.com",
		},
		{
			name:        "individual explicit",
			accountType: "individual",
			want:        "https://api.githubcopilot.com",
		},
		{
			name:        "individual mixed case and spaces",
			accountType: "  InDiViDuAl  ",
			want:        "https://api.githubcopilot.com",
		},
		{
			name:        "business account",
			accountType: "business",
			want:        "https://api.business.githubcopilot.com",
		},
		{
			name:        "enterprise account",
			accountType: "enterprise",
			want:        "https://api.enterprise.githubcopilot.com",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := CopilotBaseURLForAccountType(tc.accountType)
			if got != tc.want {
				t.Fatalf("CopilotBaseURLForAccountType(%q) = %q, want %q", tc.accountType, got, tc.want)
			}
		})
	}
}

func TestNormalizeEnterpriseURL(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want string
	}{
		{
			name: "https with trailing slash",
			url:  "https://company.ghe.com/",
			want: "company.ghe.com",
		},
		{
			name: "https without trailing slash",
			url:  "https://company.ghe.com",
			want: "company.ghe.com",
		},
		{
			name: "http scheme",
			url:  "http://github.acme.corp/",
			want: "github.acme.corp",
		},
		{
			name: "no scheme",
			url:  "company.ghe.com",
			want: "company.ghe.com",
		},
		{
			name: "with spaces",
			url:  "  https://company.ghe.com  ",
			want: "company.ghe.com",
		},
		{
			name: "empty string",
			url:  "",
			want: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := NormalizeEnterpriseURL(tc.url)
			if got != tc.want {
				t.Fatalf("NormalizeEnterpriseURL(%q) = %q, want %q", tc.url, got, tc.want)
			}
		})
	}
}

func TestCopilotBaseURLForEnterprise(t *testing.T) {
	tests := []struct {
		name          string
		enterpriseURL string
		want          string
	}{
		{
			name:          "standard enterprise URL",
			enterpriseURL: "https://company.ghe.com",
			want:          "https://copilot-api.company.ghe.com",
		},
		{
			name:          "enterprise URL with trailing slash",
			enterpriseURL: "https://github.acme.corp/",
			want:          "https://copilot-api.github.acme.corp",
		},
		{
			name:          "domain only",
			enterpriseURL: "company.ghe.com",
			want:          "https://copilot-api.company.ghe.com",
		},
		{
			name:          "empty falls back to default",
			enterpriseURL: "",
			want:          "https://api.githubcopilot.com",
		},
		{
			name:          "whitespace only falls back to default",
			enterpriseURL: "   ",
			want:          "https://api.githubcopilot.com",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := CopilotBaseURLForEnterprise(tc.enterpriseURL)
			if got != tc.want {
				t.Fatalf("CopilotBaseURLForEnterprise(%q) = %q, want %q", tc.enterpriseURL, got, tc.want)
			}
		})
	}
}

func TestResolveCopilotBaseURL(t *testing.T) {
	tests := []struct {
		name          string
		enterpriseURL string
		accountType   string
		want          string
	}{
		{
			name:          "enterprise URL takes precedence over account type",
			enterpriseURL: "https://company.ghe.com",
			accountType:   "business",
			want:          "https://copilot-api.company.ghe.com",
		},
		{
			name:          "enterprise URL takes precedence over individual",
			enterpriseURL: "https://company.ghe.com",
			accountType:   "individual",
			want:          "https://copilot-api.company.ghe.com",
		},
		{
			name:          "no enterprise URL uses account type business",
			enterpriseURL: "",
			accountType:   "business",
			want:          "https://api.business.githubcopilot.com",
		},
		{
			name:          "no enterprise URL uses account type enterprise",
			enterpriseURL: "",
			accountType:   "enterprise",
			want:          "https://api.enterprise.githubcopilot.com",
		},
		{
			name:          "no enterprise URL uses account type individual",
			enterpriseURL: "",
			accountType:   "individual",
			want:          "https://api.githubcopilot.com",
		},
		{
			name:          "both empty defaults to individual",
			enterpriseURL: "",
			accountType:   "",
			want:          "https://api.githubcopilot.com",
		},
		{
			name:          "whitespace enterprise URL uses account type",
			enterpriseURL: "   ",
			accountType:   "business",
			want:          "https://api.business.githubcopilot.com",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ResolveCopilotBaseURL(tc.enterpriseURL, tc.accountType)
			if got != tc.want {
				t.Fatalf("ResolveCopilotBaseURL(%q, %q) = %q, want %q", tc.enterpriseURL, tc.accountType, got, tc.want)
			}
		})
	}
}
