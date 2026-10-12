package usage

import (
	"testing"

	"github.com/prakersh/codexmultiauth/internal/infra/store"
	"github.com/stretchr/testify/require"
)

func TestAccountMetadataPrefersCanonicalUserIDAcrossTokens(t *testing.T) {
	auth := store.CodexAuth{Tokens: &store.CodexTokens{
		IDToken:     buildJWT(t, map[string]any{"sub": "issuer-subject"}),
		AccessToken: buildJWT(t, map[string]any{"https://api.openai.com/auth": map[string]any{"chatgpt_user_id": "canonical-user"}}),
	}}
	require.Equal(t, "canonical-user", ExtractAccountMetadata(auth).UserID)
	auth.Tokens.AccessToken = buildJWT(t, map[string]any{"https://api.openai.com/auth": map[string]any{"user_id": "canonical-user"}})
	require.Equal(t, "canonical-user", ExtractAccountMetadata(auth).UserID)
	auth.Tokens.AccessToken = "opaque"
	require.Equal(t, "issuer-subject", ExtractAccountMetadata(auth).UserID)
	require.Empty(t, ExtractAccountMetadata(store.CodexAuth{}).UserID)
}
