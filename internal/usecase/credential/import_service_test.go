package credential

import (
	"context"
	"encoding/json"
	"testing"

	domaincredential "github.com/duchoang/llmpool/internal/domain/credential"
)

type testEncryptor struct{}

func (t testEncryptor) Encrypt(plain string) (string, error)  { return "enc:" + plain, nil }
func (t testEncryptor) Decrypt(cipher string) (string, error) { return cipher, nil }

type testRepo struct{ saved []domaincredential.Profile }

func (r *testRepo) Save(_ context.Context, p domaincredential.Profile) (domaincredential.Profile, error) {
	r.saved = append(r.saved, p)
	return p, nil
}
func (r *testRepo) List(_ context.Context) ([]domaincredential.Profile, error) { return nil, nil }
func (r *testRepo) Update(_ context.Context, p domaincredential.Profile) (domaincredential.Profile, error) {
	return p, nil
}

func TestImport_DetectsProviderAndEncryptsTokens(t *testing.T) {
	repo := &testRepo{}
	svc := NewImportService(repo, testEncryptor{})

	rawPayload := []byte(`{"access_token":"a1","refresh_token":"r1","email":"user@example.com","account_id":"acc-1"}`)
	var payload CredentialPayload
	if err := json.Unmarshal(rawPayload, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}

	profile, err := svc.Import(context.Background(), ImportInput{
		Label:   "codex-user@example.com-plus.json",
		Payload: payload,
	})
	if err != nil {
		t.Fatalf("import failed: %v", err)
	}

	if profile.Provider != "openai" {
		t.Fatalf("expected provider openai, got %q", profile.Provider)
	}

	if profile.Secret.AccessToken != "enc:a1" {
		t.Fatalf("expected encrypted access token, got %q", profile.Secret.AccessToken)
	}

	if profile.Secret.RefreshToken != "enc:r1" {
		t.Fatalf("expected encrypted refresh token, got %q", profile.Secret.RefreshToken)
	}

	if len(repo.saved) != 1 {
		t.Fatalf("expected profile saved once, got %d", len(repo.saved))
	}
}
