package secrets_test

import (
	"testing"

	"github.com/elythi0n/virta/internal/secrets"
	"github.com/elythi0n/virta/internal/secrets/secretstest"
)

func TestMemory_Contract(t *testing.T) {
	secretstest.RunContract(t, func(t *testing.T) secrets.Vault {
		return secrets.NewMemory()
	})
}

func TestKeyHelpers(t *testing.T) {
	tests := []struct {
		name string
		got  string
		want string
		ns   string
	}{
		{"platform", secrets.PlatformKey("twitch", "01H"), "platform:twitch:01H", "platform"},
		{"llm", secrets.LLMKey("anthropic"), "llm:anthropic", "llm"},
		{"search", secrets.SearchKey("meilisearch"), "search:meilisearch", "search"},
		{"webhook", secrets.WebhookKey("wh_01H"), "webhook:wh_01H", "webhook"},
		{"api", secrets.APITokenKey("tok_01H"), "api:tok_01H", "api"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("key = %q, want %q", tt.got, tt.want)
			}
			if ns := secrets.SplitKey(tt.got); ns != tt.ns {
				t.Errorf("SplitKey(%q) = %q, want %q", tt.got, ns, tt.ns)
			}
		})
	}
}
