package credential

import "time"

type Profile struct {
	ID               string
	Type             string
	AccountID        string
	Enabled          bool
	Email            string
	Expired          time.Time
	LastRefreshAt    time.Time
	EncryptedProfile string
}
