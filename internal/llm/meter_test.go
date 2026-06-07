package llm

import (
	"context"
	"io"
	"testing"
	"time"
)

// noopStream returns one text event then EOF.
type noopStream struct{ n int }

func (s *noopStream) Next() (Event, error) {
	s.n++
	if s.n == 1 {
		return Event{Kind: EventText, Text: "hello"}, nil
	}
	return Event{Kind: EventDone, Usage: &Usage{InputTokens: 100, OutputTokens: 50}}, io.EOF
}
func (s *noopStream) Close() error { return nil }

// noopProvider drives a simple stream for tests.
type noopProvider struct {
	id   string
	name string
}

func (p *noopProvider) ID() string                     { return p.id }
func (p *noopProvider) DisplayName() string            { return p.name }
func (p *noopProvider) Verify(_ context.Context) error { return nil }
func (p *noopProvider) ListModels(_ context.Context) ([]ModelInfo, error) {
	return []ModelInfo{{ID: "test-model", SupportsTools: true}}, nil
}
func (p *noopProvider) Complete(_ context.Context, _ CompletionRequest) (Stream, error) {
	return &noopStream{}, nil
}

func TestMeter_MasterKillSwitch(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&noopProvider{id: "test", name: "Test"})
	m := NewMeter(reg, MeterConfig{Enabled: false})
	_, err := m.Complete(context.Background(), FeatureAsk, CompletionRequest{Model: "test-model"})
	if err != ErrLLMDisabled {
		t.Errorf("expected ErrLLMDisabled, got %v", err)
	}
}

func TestMeter_FeatureToggle(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&noopProvider{id: "test"})
	m := NewMeter(reg, MeterConfig{
		Enabled:        true,
		FeatureEnabled: map[Feature]bool{FeatureTranslation: false},
	})
	_, err := m.Complete(context.Background(), FeatureTranslation, CompletionRequest{Model: "test-model"})
	if err == nil {
		t.Error("expected error for disabled feature")
	}
	// Ask should still work.
	s, err := m.Complete(context.Background(), FeatureAsk, CompletionRequest{Model: "test-model"})
	if err != nil {
		t.Fatalf("unexpected error for enabled feature: %v", err)
	}
	_ = s.Close()
}

func TestMeter_BudgetEnforcement(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&noopProvider{id: "test"})
	m := NewMeter(reg, MeterConfig{
		Enabled: true,
		Limits:  []BudgetLimit{{Period: PeriodDaily, USD: 0.001}},
	})
	// Artificially record spending that exceeds the limit.
	m.mu.Lock()
	m.records = append(m.records, UsageRecord{At: time.Now(), CostUSD: 0.01})
	m.mu.Unlock()
	_, err := m.Complete(context.Background(), FeatureAsk, CompletionRequest{Model: "test-model"})
	var budgetErr *ErrBudgetExceeded
	if !isErrBudget(err, &budgetErr) {
		t.Errorf("expected ErrBudgetExceeded, got %v", err)
	}
}

func isErrBudget(err error, target **ErrBudgetExceeded) bool {
	if err == nil {
		return false
	}
	if be, ok := err.(*ErrBudgetExceeded); ok {
		*target = be
		return true
	}
	return false
}

func TestMeter_UsageRecording(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&noopProvider{id: "test"})
	m := NewMeter(reg, MeterConfig{Enabled: true})
	s, err := m.Complete(context.Background(), FeatureAsk, CompletionRequest{Model: "test-model"})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	for {
		ev, err := s.Next()
		if err == io.EOF {
			break
		}
		_ = ev
	}
	_ = s.Close()
	// The stream fires EventDone with usage; record should appear.
	recs := m.UsageSince(time.Now().Add(-time.Minute))
	if len(recs) != 1 {
		t.Fatalf("expected 1 usage record, got %d", len(recs))
	}
	if recs[0].Feature != FeatureAsk {
		t.Errorf("feature = %q, want ask", recs[0].Feature)
	}
}
