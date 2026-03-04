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
func (r *testRepo) UpsertByTypeAccount(_ context.Context, p domaincredential.Profile) (domaincredential.Profile, error) {
	r.saved = append(r.saved, p)
	return p, nil
}

func (r *testRepo) ListEnabled(_ context.Context) ([]domaincredential.Profile, error) {
	return nil, nil
}

func (r *testRepo) CountEnabled(_ context.Context) (int64, error) {
	return 0, nil
}

func (r *testRepo) RandomSample(_ context.Context, sampleSize int, seed int64) ([]domaincredential.Profile, error) {
	return nil, nil
}

func TestImport_SavesOnlyEncryptedProfileBlob(t *testing.T) {
	repo := &testRepo{}
	svc := NewImportService(repo, testEncryptor{})

	rawPayload := []byte(`{"type":"openai","access_token":"a1","refresh_token":"r1","email":"user@example.com","account_id":"acc-1","enabled":true,"expired":"2027-01-01T00:00:00Z","last_refresh":"2027-01-01T00:00:00Z"}`)
	var payload CredentialProfile
	if err := json.Unmarshal(rawPayload, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}

	profile, err := svc.Import(context.Background(), payload)
	if err != nil {
		t.Fatalf("import failed: %v", err)
	}

	if profile.Type != "openai" {
		t.Fatalf("expected provider type openai, got %q", profile.Type)
	}

	if profile.EncryptedProfile == "" {
		t.Fatalf("expected encrypted profile field to be set")
	}

	if profile.EncryptedProfile[:4] != "enc:" {
		t.Fatalf("expected encrypted profile marker prefix")
	}

	if profile.Enabled != true {
		t.Fatalf("expected enabled true from payload")
	}

	if len(repo.saved) != 1 {
		t.Fatalf("expected profile saved once, got %d", len(repo.saved))
	}
}

func TestImport_UpsertsDuplicateTypeAccount(t *testing.T) {
	repo := &testRepo{}
	svc := NewImportService(repo, testEncryptor{})

	rawPayload := []byte(`{"type":"openai","access_token":"a1","refresh_token":"r1","email":"user@example.com","account_id":"acc-1","enabled":true,"expired":"2027-01-01T00:00:00Z","last_refresh":"2027-01-01T00:00:00Z"}`)
	var payload CredentialProfile
	if err := json.Unmarshal(rawPayload, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}

	_, err := svc.Import(context.Background(), payload)
	if err != nil {
		t.Fatalf("first import failed: %v", err)
	}

	payload.AccessToken = "a2"
	_, err = svc.Import(context.Background(), payload)
	if err != nil {
		t.Fatalf("second import failed: %v", err)
	}

	if len(repo.saved) != 2 {
		t.Fatalf("expected upsert path called twice, got %d", len(repo.saved))
	}
	if repo.saved[0].Type != repo.saved[1].Type || repo.saved[0].AccountID != repo.saved[1].AccountID {
		t.Fatalf("expected same dedupe key for upserts")
	}
}
