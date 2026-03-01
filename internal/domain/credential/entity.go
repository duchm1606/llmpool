package credential

import "time"

type Secret struct {
	AccessToken  string
	RefreshToken string
	ExpiresAt    time.Time
	Raw          map[string]any
}

type Profile struct {
	ID              string
	Provider        string
	Label           string
	SourcePath      string
	Email           string
	AccountID       string
	HasRefreshToken bool
	Status          string
	LastRefreshAt   *time.Time
	RefreshError    string
	CreatedAt       time.Time
	UpdatedAt       time.Time
	Secret          Secret
}
