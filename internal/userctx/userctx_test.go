package userctx

import (
	"context"
	"testing"
)

func TestRoundTrip(t *testing.T) {
	ctx := WithUser(context.Background(), "user-123")
	if got := FromContext(ctx); got != "user-123" {
		t.Fatalf("FromContext = %q, want user-123", got)
	}
}

func TestEmpty(t *testing.T) {
	if got := FromContext(context.Background()); got != "" {
		t.Fatalf("FromContext on empty ctx = %q, want empty", got)
	}
}
