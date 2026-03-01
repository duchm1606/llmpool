package credential

import (
	"encoding/json"
)

type CredentialPayload struct {
	Provider     string `json:"provider"`
	AccessToken  string `json:"access_token" binding:"required"`
	RefreshToken string `json:"refresh_token"`
	ExpiresAt    string `json:"expires_at"`
	Email        string `json:"email"`
	AccountID    string `json:"account_id"`
}

func (p CredentialPayload) ToRawMap() map[string]any {
	b, err := json.Marshal(p)
	if err != nil {
		return map[string]any{}
	}

	out := map[string]any{}
	if err := json.Unmarshal(b, &out); err != nil {
		return map[string]any{}
	}

	return out
}
