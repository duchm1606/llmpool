package credential

import (
	"encoding/json"
	"testing"
	"time"
)

func TestCredentialPayload_UnmarshalJSON(t *testing.T) {
	input := []byte(`{
		"type":"openai",
		"access_token":"acc",
		"refresh_token":"ref",
		"expired":"2027-01-01T00:00:00Z",
		"id_token":"idtok",
		"enabled":true,
		"email":"user@example.com",
		"account_id":"abc"
	}`)

	var payload CredentialProfile
	if err := json.Unmarshal(input, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}

	if payload.Type != "openai" {
		t.Fatalf("expected type openai, got %q", payload.Type)
	}

	if payload.AccessToken != "acc" {
		t.Fatalf("expected access token acc, got %q", payload.AccessToken)
	}

	if payload.RefreshToken != "ref" {
		t.Fatalf("expected refresh token ref, got %q", payload.RefreshToken)
	}

	if payload.IDToken != "idtok" {
		t.Fatalf("expected id token idtok, got %q", payload.IDToken)
	}

	expectedExpired, err := time.Parse(time.RFC3339, "2027-01-01T00:00:00Z")
	if err != nil {
		t.Fatalf("parse expected expiry: %v", err)
	}
	if !payload.Expired.Equal(expectedExpired) {
		t.Fatalf("expected expired field set to parsed RFC3339 time")
	}

	if *payload.Enabled != true {
		t.Fatalf("expected enabled true")
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
