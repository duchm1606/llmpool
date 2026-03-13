package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	domaincredential "github.com/duchoang/llmpool/internal/domain/credential"
	domainquota "github.com/duchoang/llmpool/internal/domain/quota"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type stubUsageCache struct {
	usages []domainquota.CopilotUsage
}

func (s *stubUsageCache) ListCopilotUsages(ctx context.Context) ([]domainquota.CopilotUsage, error) {
	return s.usages, nil
}

type stubUsageCredentialRepo struct {
	profiles []domaincredential.Profile
}

func (s *stubUsageCredentialRepo) List(ctx context.Context) ([]domaincredential.Profile, error) {
	return s.profiles, nil
}

type stubSessionQuotaReader struct {
	values map[string]*domainquota.SessionQuotaUsage
}

func (s *stubSessionQuotaReader) GetUsage(ctx context.Context, providerType, accountID string) (*domainquota.SessionQuotaUsage, error) {
	return s.values[accountID], nil
}

func TestUsageHandler_ListUsages_EnrichesSessionQuota(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	r := gin.New()
	h := NewUsageHandler(
		&stubUsageCache{usages: []domainquota.CopilotUsage{{CredentialID: "cred-1", Login: "octocat", FetchedAt: time.Now()}}},
		&stubUsageCredentialRepo{profiles: []domaincredential.Profile{{ID: "cred-1", Type: "copilot", AccountID: "acct-1"}}},
		&stubSessionQuotaReader{values: map[string]*domainquota.SessionQuotaUsage{
			"acct-1": {
				RequestsPerMinute:    5,
				RequestsThisMinute:   2,
				RemainingThisMinute:  3,
				RequestsPerSession:   50,
				RequestsThisSession:  11,
				RemainingThisSession: 39,
			},
		}},
		zap.NewNop(),
	)
	r.GET("/v1/internal/usage", h.ListUsages)

	req := httptest.NewRequest(http.MethodGet, "/v1/internal/usage", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status code 200, got %d", w.Code)
	}

	var resp UsageResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if len(resp.Usages) != 1 {
		t.Fatalf("expected 1 usage, got %d", len(resp.Usages))
	}
	if resp.Usages[0].SessionQuota == nil {
		t.Fatal("expected session quota to be present")
	}
	if resp.Usages[0].SessionQuota.RequestsThisSession != 11 {
		t.Fatalf("unexpected session usage: %d", resp.Usages[0].SessionQuota.RequestsThisSession)
	}
}
