package credential

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	domaincredential "github.com/duchoang/llmpool/internal/domain/credential"
	domainoauth "github.com/duchoang/llmpool/internal/domain/oauth"
)

// mockRepository for testing completion service
type mockCompletionRepo struct {
	saved     []domaincredential.Profile
	saveErr   error
	upserted  []domaincredential.Profile
	upsertErr error
}

func (m *mockCompletionRepo) Save(ctx context.Context, profile domaincredential.Profile) (domaincredential.Profile, error) {
	if m.saveErr != nil {
		return domaincredential.Profile{}, m.saveErr
	}
	m.saved = append(m.saved, profile)
	return profile, nil
}

func (m *mockCompletionRepo) List(ctx context.Context) ([]domaincredential.Profile, error) {
	return nil, nil
}

func (m *mockCompletionRepo) GetByID(ctx context.Context, id string) (*domaincredential.Profile, error) {
	return nil, nil
}

func (m *mockCompletionRepo) Update(ctx context.Context, profile domaincredential.Profile) (domaincredential.Profile, error) {
	return profile, nil
}

func (m *mockCompletionRepo) UpsertByTypeAccount(ctx context.Context, profile domaincredential.Profile) (domaincredential.Profile, error) {
	if m.upsertErr != nil {
		return domaincredential.Profile{}, m.upsertErr
	}
	m.upserted = append(m.upserted, profile)
	return profile, nil
}

func (m *mockCompletionRepo) ListEnabled(ctx context.Context) ([]domaincredential.Profile, error) {
	return nil, nil
}

func (m *mockCompletionRepo) CountEnabled(ctx context.Context) (int64, error) {
	return 0, nil
}

func (m *mockCompletionRepo) RandomSample(ctx context.Context, sampleSize int, seed int64) ([]domaincredential.Profile, error) {
	return nil, nil
}

// mockEncryptor for testing
type mockCompletionEncryptor struct {
	encrypted  map[string]string
	decrypted  map[string]string
	encryptErr error
	decryptErr error
}

func newMockCompletionEncryptor() *mockCompletionEncryptor {
	return &mockCompletionEncryptor{
		encrypted: make(map[string]string),
		decrypted: make(map[string]string),
	}
}

func (m *mockCompletionEncryptor) Encrypt(plain string) (string, string, string, error) {
	if m.encryptErr != nil {
		return "", "", "", m.encryptErr
	}
	cipher := "encrypted:" + plain
	m.encrypted[plain] = cipher
	return cipher, "iv", "tag", nil
}

func (m *mockCompletionEncryptor) Decrypt(cipher, iv, tag string) (string, error) {
	if m.decryptErr != nil {
		return "", m.decryptErr
	}
	for plain, c := range m.encrypted {
		if c == cipher {
			return plain, nil
		}
	}
	return "", errors.New("cipher not found")
}

func (m *mockCompletionEncryptor) ShouldEncrypt() bool {
	return true
}

func TestCompletionService_CompleteOAuth_Success(t *testing.T) {
	ctx := context.Background()
	repo := &mockCompletionRepo{}
	encryptor := newMockCompletionEncryptor()

	service := NewCompletionService(repo, encryptor, nil)
	service.(*completionService).now = func() time.Time {
		return time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	}
	service.(*completionService).now = func() time.Time {
		return time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	}
	// Override time function for deterministic testing
	service.(*completionService).now = func() time.Time {
		return time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	}

	accountID := "test-account-123"
	tokenPayload := domainoauth.TokenPayload{
		AccessToken:  "access-token-abc",
		RefreshToken: "refresh-token-xyz",
		ExpiresAt:    time.Date(2025, 1, 2, 12, 0, 0, 0, time.UTC),
		TokenType:    "Bearer",
	}

	profile, err := service.CompleteOAuth(ctx, accountID, tokenPayload)
	if err != nil {
		t.Fatalf("CompleteOAuth failed: %v", err)
	}

	// Verify profile returned
	if profile.Type != "codex" {
		t.Errorf("expected type codex, got %q", profile.Type)
	}
	if profile.AccountID != accountID {
		t.Errorf("expected accountID %q, got %q", accountID, profile.AccountID)
	}
	if !profile.Enabled {
		t.Error("expected profile to be enabled")
	}
	if profile.Expired != tokenPayload.ExpiresAt {
		t.Errorf("expected expiry %v, got %v", tokenPayload.ExpiresAt, profile.Expired)
	}

	// Verify UpsertByTypeAccount was called (not Save)
	if len(repo.upserted) != 1 {
		t.Errorf("expected 1 upsert call, got %d", len(repo.upserted))
	}

	// Verify profile data was encrypted
	upsertedProfile := repo.upserted[0]
	if upsertedProfile.EncryptedProfile == "" {
		t.Error("expected encrypted profile to be set")
	}

	// Verify encrypted data can be "decrypted" and contains expected fields
	decrypted, err := encryptor.Decrypt(
		upsertedProfile.EncryptedProfile,
		stringValue(upsertedProfile.EncryptedIV),
		stringValue(upsertedProfile.EncryptedTag),
	)
	if err != nil {
		t.Fatalf("failed to decrypt profile: %v", err)
	}

	var credProfile CredentialProfile
	if err := json.Unmarshal([]byte(decrypted), &credProfile); err != nil {
		t.Fatalf("failed to unmarshal decrypted profile: %v", err)
	}

	if credProfile.AccessToken != tokenPayload.AccessToken {
		t.Errorf("expected access token %q, got %q", tokenPayload.AccessToken, credProfile.AccessToken)
	}
	if credProfile.RefreshToken != tokenPayload.RefreshToken {
		t.Errorf("expected refresh token %q, got %q", tokenPayload.RefreshToken, credProfile.RefreshToken)
	}
	if credProfile.Type != "codex" {
		t.Errorf("expected type codex, got %q", credProfile.Type)
	}
	if credProfile.AccountID != accountID {
		t.Errorf("expected accountID %q, got %q", accountID, credProfile.AccountID)
	}
}

func TestCompletionService_CompleteOAuth_RefreshesRegistryOnSuccess(t *testing.T) {
	ctx := context.Background()
	repo := &mockCompletionRepo{}
	encryptor := newMockCompletionEncryptor()

	var refreshCalled bool
	var refreshedType string
	var refreshedAccountID string
	service := NewCompletionService(repo, encryptor, func(_ context.Context, profileType, accountID string) {
		refreshCalled = true
		refreshedType = profileType
		refreshedAccountID = accountID
	})
	service.(*completionService).now = func() time.Time {
		return time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	}

	accountID := "test-account-123"
	tokenPayload := domainoauth.TokenPayload{
		AccessToken:  "access-token-abc",
		RefreshToken: "refresh-token-xyz",
		ExpiresAt:    time.Date(2025, 1, 2, 12, 0, 0, 0, time.UTC),
		TokenType:    "Bearer",
	}

	_, err := service.CompleteOAuth(ctx, accountID, tokenPayload)
	if err != nil {
		t.Fatalf("CompleteOAuth failed: %v", err)
	}

	if !refreshCalled {
		t.Fatal("expected registry refresher to be called")
	}
	if refreshedType != "codex" {
		t.Fatalf("expected refreshed type codex, got %q", refreshedType)
	}
	if refreshedAccountID != accountID {
		t.Fatalf("expected refreshed account id %q, got %q", accountID, refreshedAccountID)
	}
}

func TestCompletionService_CompleteOAuth_ReauthUpdatesExisting(t *testing.T) {
	ctx := context.Background()
	repo := &mockCompletionRepo{}
	encryptor := newMockCompletionEncryptor()

	service := NewCompletionService(repo, encryptor, nil)
	service.(*completionService).now = func() time.Time {
		return time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	}
	service.(*completionService).now = func() time.Time {
		return time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	}
	service.(*completionService).now = func() time.Time {
		return time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	}

	accountID := "test-account-123"

	// First OAuth completion
	tokenPayload1 := domainoauth.TokenPayload{
		AccessToken:  "access-token-v1",
		RefreshToken: "refresh-token-v1",
		ExpiresAt:    time.Date(2025, 1, 2, 12, 0, 0, 0, time.UTC),
	}

	_, err := service.CompleteOAuth(ctx, accountID, tokenPayload1)
	if err != nil {
		t.Fatalf("first CompleteOAuth failed: %v", err)
	}

	// Reauth (second OAuth completion for same account)
	tokenPayload2 := domainoauth.TokenPayload{
		AccessToken:  "access-token-v2",
		RefreshToken: "refresh-token-v2",
		ExpiresAt:    time.Date(2025, 1, 3, 12, 0, 0, 0, time.UTC),
	}

	_, err = service.CompleteOAuth(ctx, accountID, tokenPayload2)
	if err != nil {
		t.Fatalf("second CompleteOAuth (reauth) failed: %v", err)
	}

	// Verify UpsertByTypeAccount was called twice (not Save)
	if len(repo.upserted) != 2 {
		t.Errorf("expected 2 upsert calls for reauth, got %d", len(repo.upserted))
	}

	// Verify Save was never called (ensuring we use upsert semantics)
	if len(repo.saved) != 0 {
		t.Errorf("expected 0 save calls (should use upsert), got %d", len(repo.saved))
	}
}

func TestCompletionService_CompleteOAuth_EncryptionFailure(t *testing.T) {
	ctx := context.Background()
	repo := &mockCompletionRepo{}
	encryptor := newMockCompletionEncryptor()
	encryptor.encryptErr = errors.New("encryption failed")

	service := NewCompletionService(repo, encryptor, nil)
	service.(*completionService).now = func() time.Time {
		return time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	}

	accountID := "test-account-123"
	tokenPayload := domainoauth.TokenPayload{
		AccessToken:  "access-token-abc",
		RefreshToken: "refresh-token-xyz",
		ExpiresAt:    time.Date(2025, 1, 1, 13, 0, 0, 0, time.UTC),
	}

	_, err := service.CompleteOAuth(ctx, accountID, tokenPayload)
	if err == nil {
		t.Fatal("expected error for encryption failure, got nil")
	}

	if !errors.Is(err, encryptor.encryptErr) {
		t.Errorf("expected encryption error, got: %v", err)
	}
}

func TestCompletionService_CompleteOAuth_UpsertFailure(t *testing.T) {
	ctx := context.Background()
	repo := &mockCompletionRepo{}
	repo.upsertErr = errors.New("database error")
	encryptor := newMockCompletionEncryptor()

	service := NewCompletionService(repo, encryptor, nil)
	service.(*completionService).now = func() time.Time {
		return time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	}

	accountID := "test-account-123"
	tokenPayload := domainoauth.TokenPayload{
		AccessToken:  "access-token-abc",
		RefreshToken: "refresh-token-xyz",
		ExpiresAt:    time.Date(2025, 1, 2, 12, 0, 0, 0, time.UTC),
	}

	_, err := service.CompleteOAuth(ctx, accountID, tokenPayload)
	if err == nil {
		t.Fatal("expected error for upsert failure, got nil")
	}

	if !errors.Is(err, repo.upsertErr) {
		t.Errorf("expected upsert error, got: %v", err)
	}
}

func TestCompletionService_CompleteOAuth_TimestampsSet(t *testing.T) {
	ctx := context.Background()
	repo := &mockCompletionRepo{}
	encryptor := newMockCompletionEncryptor()

	service := NewCompletionService(repo, encryptor, nil)
	fixedNow := time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC)
	service.(*completionService).now = func() time.Time {
		return fixedNow
	}

	accountID := "test-account-123"
	expiry := time.Date(2025, 1, 16, 10, 30, 0, 0, time.UTC)
	tokenPayload := domainoauth.TokenPayload{
		AccessToken:  "access-token-abc",
		RefreshToken: "refresh-token-xyz",
		ExpiresAt:    expiry,
	}

	profile, err := service.CompleteOAuth(ctx, accountID, tokenPayload)
	if err != nil {
		t.Fatalf("CompleteOAuth failed: %v", err)
	}

	// Verify timestamps
	if profile.Expired != expiry {
		t.Errorf("expected Expired to be %v, got %v", expiry, profile.Expired)
	}
	if profile.LastRefreshAt != fixedNow {
		t.Errorf("expected LastRefreshAt to be %v, got %v", fixedNow, profile.LastRefreshAt)
	}
}

func TestCompletionService_CompleteOAuth_RejectsMissingAccessToken(t *testing.T) {
	ctx := context.Background()
	repo := &mockCompletionRepo{}
	encryptor := newMockCompletionEncryptor()

	service := NewCompletionService(repo, encryptor, nil)
	service.(*completionService).now = func() time.Time {
		return time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	}

	_, err := service.CompleteOAuth(ctx, "test-account-123", domainoauth.TokenPayload{
		AccessToken:  "",
		RefreshToken: "refresh-token-xyz",
		ExpiresAt:    time.Date(2025, 1, 2, 12, 0, 0, 0, time.UTC),
	})
	if err == nil {
		t.Fatal("expected error for missing access token")
	}
	if !strings.Contains(err.Error(), "missing access_token") {
		t.Fatalf("expected missing access_token error, got %v", err)
	}
	if len(repo.upserted) != 0 {
		t.Fatalf("expected no upsert for invalid payload, got %d", len(repo.upserted))
	}
}

func TestCompletionService_CompleteOAuth_RejectsMissingRefreshToken(t *testing.T) {
	ctx := context.Background()
	repo := &mockCompletionRepo{}
	encryptor := newMockCompletionEncryptor()

	service := NewCompletionService(repo, encryptor, nil)
	service.(*completionService).now = func() time.Time {
		return time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	}

	_, err := service.CompleteOAuth(ctx, "test-account-123", domainoauth.TokenPayload{
		AccessToken:  "access-token-abc",
		RefreshToken: "",
		ExpiresAt:    time.Date(2025, 1, 2, 12, 0, 0, 0, time.UTC),
	})
	if err == nil {
		t.Fatal("expected error for missing refresh token")
	}
	if !strings.Contains(err.Error(), "missing refresh_token") {
		t.Fatalf("expected missing refresh_token error, got %v", err)
	}
	if len(repo.upserted) != 0 {
		t.Fatalf("expected no upsert for invalid payload, got %d", len(repo.upserted))
	}
}

func TestCompletionService_CompleteOAuth_RejectsExpiredToken(t *testing.T) {
	ctx := context.Background()
	repo := &mockCompletionRepo{}
	encryptor := newMockCompletionEncryptor()

	service := NewCompletionService(repo, encryptor, nil)
	service.(*completionService).now = func() time.Time {
		return time.Date(2025, 1, 2, 12, 0, 0, 0, time.UTC)
	}

	_, err := service.CompleteOAuth(ctx, "test-account-123", domainoauth.TokenPayload{
		AccessToken:  "access-token-abc",
		RefreshToken: "refresh-token-xyz",
		ExpiresAt:    time.Date(2025, 1, 2, 11, 0, 0, 0, time.UTC),
	})
	if err == nil {
		t.Fatal("expected error for expired token")
	}
	if !strings.Contains(err.Error(), "expires_at is not in the future") {
		t.Fatalf("expected expires_at validation error, got %v", err)
	}
	if len(repo.upserted) != 0 {
		t.Fatalf("expected no upsert for invalid payload, got %d", len(repo.upserted))
	}
}
