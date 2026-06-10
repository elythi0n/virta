package llm

// TestPrivacy_AllTogglesOff verifies the structural guarantee: when the master kill switch is
// off, Complete returns ErrLLMDisabled and no network call is ever initiated. This is the
// automated privacy contract test: intelligence toggles off must mean zero external calls.
//
// It does NOT make real network requests — it proves that the Meter itself enforces the kill
// switch before reaching any provider, so the provider's network code is never invoked.
// The test is deliberately simple: a single assertion on the return value is sufficient because
// the Meter checks `cfg.Enabled` before calling registry.Complete.

import (
	"context"
	"testing"
)

func TestPrivacy_AllTogglesOff_NoCall(t *testing.T) {
	reg := NewRegistry()
	// Register a provider that panics if Complete is called — proves the Meter stops before it.
	reg.Register(&panicOnCompleteProvider{})
	m := NewMeter(reg, MeterConfig{Enabled: false})

	_, err := m.Complete(context.Background(), FeatureAsk, CompletionRequest{Model: "any"})
	if err != ErrLLMDisabled {
		t.Fatalf("expected ErrLLMDisabled with kill switch off, got %v", err)
	}
}

func TestPrivacy_FeatureDisabled_NoCall(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&panicOnCompleteProvider{})
	m := NewMeter(reg, MeterConfig{
		Enabled:        true,
		FeatureEnabled: map[Feature]bool{FeatureAsk: false},
	})
	_, err := m.Complete(context.Background(), FeatureAsk, CompletionRequest{Model: "any"})
	if err == nil {
		t.Fatal("expected error for disabled feature")
	}
}

// panicOnCompleteProvider panics if Complete is ever called — proves the Meter stops before
// any network request could occur when features are disabled.
type panicOnCompleteProvider struct{}

func (p *panicOnCompleteProvider) ID() string                     { return "panic" }
func (p *panicOnCompleteProvider) DisplayName() string            { return "Panic" }
func (p *panicOnCompleteProvider) Verify(_ context.Context) error { return nil }
func (p *panicOnCompleteProvider) ListModels(_ context.Context) ([]ModelInfo, error) {
	return []ModelInfo{{ID: "any"}}, nil
}
func (p *panicOnCompleteProvider) Complete(_ context.Context, _ CompletionRequest) (Stream, error) {
	panic("privacy violation: Complete was called when AI features should be disabled")
}
