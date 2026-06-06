package api

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

type fakeFilters struct {
	rules  []FilterRule
	setErr error
}

func (f *fakeFilters) Filters() []FilterRule { return f.rules }
func (f *fakeFilters) SetFilters(_ context.Context, rules []FilterRule) error {
	if f.setErr != nil {
		return f.setErr
	}
	f.rules = rules
	return nil
}

func TestFilters_ListReturnsRules(t *testing.T) {
	s := start(t)
	s.SetFilters(&fakeFilters{rules: []FilterRule{{ID: "1", Action: "hide", Keywords: []string{"spoiler"}}}})
	code, body := authedReq(t, http.MethodGet, "http://"+s.Addr()+"/v1/filters?token="+s.Token(), nil)
	if code != http.StatusOK {
		t.Fatalf("list status = %d, want 200", code)
	}
	var resp struct {
		Filters []FilterRule `json:"filters"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.Filters) != 1 || resp.Filters[0].Action != "hide" {
		t.Errorf("filters = %+v", resp.Filters)
	}
}

func TestFilters_PutStoresRules(t *testing.T) {
	s := start(t)
	ff := &fakeFilters{}
	s.SetFilters(ff)
	body, _ := json.Marshal(map[string]any{"filters": []FilterRule{{ID: "a", Action: "highlight", Authors: []string{"mod"}}}})
	code, _ := authedReq(t, http.MethodPut, "http://"+s.Addr()+"/v1/filters?token="+s.Token(), body)
	if code != http.StatusOK {
		t.Fatalf("put status = %d, want 200", code)
	}
	if len(ff.rules) != 1 || ff.rules[0].ID != "a" {
		t.Errorf("stored = %+v", ff.rules)
	}
}

func TestFilters_InvalidRulesetIs400(t *testing.T) {
	s := start(t)
	s.SetFilters(&fakeFilters{setErr: ErrInvalidRuleset})
	body, _ := json.Marshal(map[string]any{"filters": []FilterRule{{ID: "a", Action: "hide", Regexes: []string{"("}}}})
	code, _ := authedReq(t, http.MethodPut, "http://"+s.Addr()+"/v1/filters?token="+s.Token(), body)
	if code != http.StatusBadRequest {
		t.Fatalf("invalid ruleset status = %d, want 400", code)
	}
}

func TestFilters_UnavailableWithoutController(t *testing.T) {
	s := start(t)
	code, _ := authedReq(t, http.MethodGet, "http://"+s.Addr()+"/v1/filters?token="+s.Token(), nil)
	if code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", code)
	}
}
