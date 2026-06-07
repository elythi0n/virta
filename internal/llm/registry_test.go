package llm

import (
	"context"
	"testing"
)

// fakeProvider is a stub for testing the registry.
type fakeProvider struct {
	id     string
	name   string
	models []ModelInfo
}

func (f *fakeProvider) ID() string                                        { return f.id }
func (f *fakeProvider) DisplayName() string                               { return f.name }
func (f *fakeProvider) Verify(_ context.Context) error                    { return nil }
func (f *fakeProvider) ListModels(_ context.Context) ([]ModelInfo, error) { return f.models, nil }
func (f *fakeProvider) Complete(_ context.Context, _ CompletionRequest) (Stream, error) {
	return nil, ErrProviderUnavailable
}

func TestRegistry_RegisterAndDefault(t *testing.T) {
	r := NewRegistry()
	p := &fakeProvider{id: "anthropic", name: "Anthropic", models: []ModelInfo{
		{ID: "claude-opus-4-8", SupportsTools: true},
	}}
	r.Register(p)

	if got := r.SelectedModel(); got != "claude-opus-4-8" {
		t.Errorf("default model = %q, want claude-opus-4-8", got)
	}
}

func TestRegistry_SetModel(t *testing.T) {
	r := NewRegistry()
	r.Register(&fakeProvider{id: "openai", name: "OpenAI"})
	r.SetModel("gpt-4o")
	if got := r.SelectedModel(); got != "gpt-4o" {
		t.Errorf("selected = %q, want gpt-4o", got)
	}
}

func TestRegistry_AllModels(t *testing.T) {
	r := NewRegistry()
	r.Register(&fakeProvider{id: "anthropic", name: "Anthropic", models: []ModelInfo{
		{ID: "claude-opus-4-8", Family: "Opus"},
	}})
	r.Register(&fakeProvider{id: "ollama", name: "Ollama", models: []ModelInfo{
		{ID: "llama3.3:70b"},
	}})
	groups, err := r.AllModels(context.Background())
	if err != nil {
		t.Fatalf("AllModels: %v", err)
	}
	if len(groups) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(groups))
	}
	if groups[0].ProviderID != "anthropic" {
		t.Errorf("first group should be anthropic, got %q", groups[0].ProviderID)
	}
}

func TestRegistry_Deregister(t *testing.T) {
	r := NewRegistry()
	r.Register(&fakeProvider{id: "anthropic"})
	r.Deregister("anthropic")

	_, err := r.Complete(context.Background(), CompletionRequest{Model: "claude-opus-4-8"})
	if err == nil {
		t.Error("expected error after deregistration")
	}
}
