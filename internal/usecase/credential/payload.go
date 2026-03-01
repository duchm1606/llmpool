package credential

import (
	"encoding/json"
	"time"
)

type CredentialProfile struct {
	AccessToken  string    `json:"access_token" binding:"required"`
	RefreshToken string    `json:"refresh_token"`
	IDToken      string    `json:"id_token"`
	Email        string    `json:"email" binding:"required"`
	AccountID    string    `json:"account_id" binding:"required"`
	Expired      time.Time `json:"expired" binding:"required"`
	Enabled      *bool     `json:"enabled"`
	Type         string    `json:"type" binding:"required"`
	LastRefresh  time.Time `json:"last_refresh" binding:"required"`
}

func (p CredentialProfile) ToRawMap() map[string]any {
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
