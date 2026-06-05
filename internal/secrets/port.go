// Package secrets defines the credential-storage contract. Every credential — platform
// OAuth tokens, LLM API keys, webhook secrets, the local API token — lives in the OS
// keychain via a Vault; the database stores only the reference string.
//
// Implementations live in subpackages: secrets/keychain (OS-native, the default) and
// secrets/agevault (an age-encrypted file, the headless fallback). They are wired in
// internal/app and selected by a startup probe (native OS keychain first; an encrypted-file fallback when none is available). Both run the same
// conformance suite (secrets/secretstest), so the fallback can't behave differently.
package secrets

import (
	"context"
	"errors"
	"strings"
)

// ErrNotFound is returned by Get when no value is stored for the key.
var ErrNotFound = errors.New("secrets: not found")

// Backend names which storage rung is active — surfaced to the UI (in user terms) and
// diagnostics so the user knows where their secrets live.
type Backend string

const (
	BackendKeychain  Backend = "keychain"   // OS-native (Credential Manager / Keychain / Secret Service)
	BackendFileVault Backend = "file_vault" // encrypted local file fallback (weaker; for systems with no keychain)
	BackendMemory    Backend = "memory"     // tests only
)

// Vault stores and retrieves secrets by key. Keys are built with the helpers below so every
// caller uses the same canonical scheme. Implementations must be safe for concurrent use.
type Vault interface {
	// Get returns the secret for key, or ErrNotFound.
	Get(ctx context.Context, key string) (string, error)
	// Set stores value under key (overwriting any existing value).
	Set(ctx context.Context, key, value string) error
	// Delete removes key. Deleting a missing key is not an error (idempotent).
	Delete(ctx context.Context, key string) error
	// Backend reports which storage rung this vault is.
	Backend() Backend
}

// ---- Canonical key scheme ----
//
// Keys are namespaced strings stored under the OS service name "virta". Build them only
// with these helpers so the DB's secret_ref values and the vault stay in lockstep.

// ServiceName is the OS keychain service all virta secrets live under.
const ServiceName = "virta"

// PlatformKey is the keychain key for a platform account's token bundle, e.g.
// "platform:twitch:01H…". accountID is the store Account ULID.
func PlatformKey(platform, accountID string) string {
	return "platform:" + platform + ":" + accountID
}

// LLMKey is the keychain key for an LLM provider's API key, e.g. "llm:anthropic".
func LLMKey(provider string) string { return "llm:" + provider }

// SearchKey is the keychain key for a search/embedding provider key, e.g.
// "search:meilisearch", "embed:voyage".
func SearchKey(provider string) string { return "search:" + provider }

// WebhookKey is the keychain key for a webhook endpoint's signing secret, e.g.
// "webhook:wh_01H…".
func WebhookKey(endpointID string) string { return "webhook:" + endpointID }

// APITokenKey is the keychain key for a scoped local-API token, e.g. "api:tok_01H…".
func APITokenKey(tokenID string) string { return "api:" + tokenID }

// SplitKey returns the namespace (first segment) of a key — useful for diagnostics and
// bulk operations. SplitKey("platform:twitch:1") == "platform".
func SplitKey(key string) string {
	if i := strings.IndexByte(key, ':'); i >= 0 {
		return key[:i]
	}
	return key
}
