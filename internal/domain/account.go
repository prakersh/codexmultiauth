package domain

import "time"

const StateVersionV1 = "cma-state-v1"

type AuthStoreKind string

const (
	AuthStoreFile    AuthStoreKind = "file"
	AuthStoreKeyring AuthStoreKind = "keyring"
)

type Account struct {
	ID            string         `json:"id"`
	DisplayName   string         `json:"display_name"`
	Aliases       []string       `json:"aliases,omitempty"`
	Fingerprint   string         `json:"fingerprint"`
	AuthStoreKind AuthStoreKind  `json:"auth_store_kind"`
	CreatedAt     time.Time      `json:"created_at"`
	LastUsedAt    *time.Time     `json:"last_used_at,omitempty"`
	Usage         *UsageSummary  `json:"usage,omitempty"`
	Source        map[string]any `json:"source,omitempty"`
}

type State struct {
	Version         string    `json:"version"`
	Accounts        []Account `json:"accounts"`
	ActiveAccountID string    `json:"active_account_id,omitempty"`
}
