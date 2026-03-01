package credential

import (
	"encoding/json"
	"testing"
)

func TestCredentialPayload_UnmarshalJSON(t *testing.T) {
	input := []byte(`{
		"provider":"openai",
		"access_token":"acc",
		"refresh_token":"ref",
		"expires_at":"2027-01-01T00:00:00Z",
		"email":"user@example.com",
		"account_id":"abc"
	}`)

	var payload CredentialPayload
	if err := json.Unmarshal(input, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}

	if payload.Provider != "openai" {
		t.Fatalf("expected provider openai, got %q", payload.Provider)
	}

	if payload.AccessToken != "acc" {
		t.Fatalf("expected access token acc, got %q", payload.AccessToken)
	}

	if payload.RefreshToken != "ref" {
		t.Fatalf("expected refresh token ref, got %q", payload.RefreshToken)
	}

	if payload.Email != "user@example.com" {
		t.Fatalf("expected email user@example.com, got %q", payload.Email)
	}

	if payload.AccountID != "abc" {
		t.Fatalf("expected account id abc, got %q", payload.AccountID)
	}

	raw := payload.ToRawMap()
	if len(raw) == 0 {
		t.Fatalf("expected raw payload map")
	}

	if raw["access_token"] != "acc" {
		t.Fatalf("expected marshaled access_token acc")
	}
}
