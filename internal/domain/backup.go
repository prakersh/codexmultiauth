package domain

import "time"

type ConflictPolicy string

const (
	ConflictAsk       ConflictPolicy = "ask"
	ConflictOverwrite ConflictPolicy = "overwrite"
	ConflictSkip      ConflictPolicy = "skip"
	ConflictRename    ConflictPolicy = "rename"
)

type BackupManifest struct {
	Version    string    `json:"version"`
	CreatedAt  time.Time `json:"created_at"`
	AccountIDs []string  `json:"account_ids"`
}
