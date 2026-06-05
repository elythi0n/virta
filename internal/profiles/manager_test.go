package profiles

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/elythi0n/virta/internal/clock"
	"github.com/elythi0n/virta/internal/filter"
	"github.com/elythi0n/virta/internal/platform"
	"github.com/elythi0n/virta/internal/store"
)

type fakeChannels struct {
	mu     sync.Mutex
	joined []string
	left   []string
}

func (f *fakeChannels) Join(_ context.Context, ch platform.ChannelRef, _ platform.ConnMode) error {
	f.mu.Lock()
	f.joined = append(f.joined, channelKey(ch.Platform, ch.Slug))
	f.mu.Unlock()
	return nil
}

func (f *fakeChannels) Leave(ch platform.ChannelRef) error {
	f.mu.Lock()
	f.left = append(f.left, channelKey(ch.Platform, ch.Slug))
	f.mu.Unlock()
	return nil
}

type fakeFilter struct{ rs *filter.Ruleset }

func (f *fakeFilter) SetRuleset(rs *filter.Ruleset) { f.rs = rs }

type fakeEmitter struct{ evs []platform.Event }

func (e *fakeEmitter) Submit(ev platform.Event) { e.evs = append(e.evs, ev) }

type fakeLogging struct {
	enabled   bool
	retention string
	calls     int
}

func (l *fakeLogging) SetLogging(enabled bool, retention string) {
	l.enabled = enabled
	l.retention = retention
	l.calls++
}

func newManager(t *testing.T) (*Manager, *store.Memory, *fakeChannels, *fakeFilter, *fakeEmitter) {
	t.Helper()
	mem := store.NewMemory(clock.NewFake(time.Unix(1000, 0)))
	ch := &fakeChannels{}
	ff := &fakeFilter{}
	em := &fakeEmitter{}
	m := New(mem.Profiles(), ch, ff, &fakeLogging{}, em, clock.NewFake(time.Unix(1000, 0)))
	return m, mem, ch, ff, em
}

func createProfile(t *testing.T, mem *store.Memory, name string, doc Doc) store.Profile {
	t.Helper()
	raw, err := doc.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	p, err := mem.Profiles().Create(context.Background(), name, raw)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func spec(slug string) ChannelSpec {
	return ChannelSpec{Platform: platform.Twitch, Slug: slug, Mode: platform.ModeAnonymous}
}

func TestEnsureDefault(t *testing.T) {
	m, _, _, _, _ := newManager(t)
	p, err := m.EnsureDefault(context.Background())
	if err != nil || p.Name != "default" || !p.IsDefault {
		t.Fatalf("EnsureDefault = %+v, %v", p, err)
	}
	p2, _ := m.EnsureDefault(context.Background())
	if p2.ID != p.ID {
		t.Errorf("second EnsureDefault created a new profile (%s != %s)", p2.ID, p.ID)
	}
}

func TestActivate_JoinsChannelsAndSwapsFilter(t *testing.T) {
	m, mem, ch, ff, em := newManager(t)
	p := createProfile(t, mem, "rig", Doc{
		Version:  CurrentVersion,
		Channels: []ChannelSpec{spec("forsen"), spec("xqc")},
		Filters:  []filter.Rule{{ID: "bots", Action: filter.ActionHide, Match: filter.Match{Authors: []string{"nightbot"}}}},
	})

	if err := m.Activate(context.Background(), p.ID); err != nil {
		t.Fatalf("Activate: %v", err)
	}
	if len(ch.joined) != 2 {
		t.Errorf("joined = %v, want both channels", ch.joined)
	}
	if ff.rs == nil {
		t.Error("filter ruleset not swapped")
	}
	if len(em.evs) != 1 {
		t.Fatalf("emitted %d events, want 1 profile_changed", len(em.evs))
	}
	if pc, ok := em.evs[0].(platform.ProfileChangedEvent); !ok || pc.ProfileID != p.ID || pc.Name != "rig" {
		t.Errorf("event = %+v", em.evs[0])
	}
}

func TestActivate_AtomicSwitchNoGapForKept(t *testing.T) {
	m, mem, ch, _, _ := newManager(t)
	a := createProfile(t, mem, "a", Doc{Version: CurrentVersion, Channels: []ChannelSpec{spec("x"), spec("y")}})
	b := createProfile(t, mem, "b", Doc{Version: CurrentVersion, Channels: []ChannelSpec{spec("y"), spec("z")}})

	if err := m.Activate(context.Background(), a.ID); err != nil {
		t.Fatal(err)
	}
	// Reset the recorder to observe only the switch a→b.
	ch.joined = nil
	ch.left = nil

	if err := m.Activate(context.Background(), b.ID); err != nil {
		t.Fatal(err)
	}
	// y is common to both: must not be left or re-joined (no feed gap).
	if len(ch.joined) != 1 || ch.joined[0] != "twitch:z" {
		t.Errorf("switch joined = %v, want only twitch:z", ch.joined)
	}
	if len(ch.left) != 1 || ch.left[0] != "twitch:x" {
		t.Errorf("switch left = %v, want only twitch:x", ch.left)
	}
}

func TestAddAndRemoveChannel_Persisted(t *testing.T) {
	m, mem, _, _, _ := newManager(t)
	p, _ := m.EnsureDefault(context.Background())
	if err := m.Activate(context.Background(), p.ID); err != nil {
		t.Fatal(err)
	}

	ref := platform.ChannelRef{Platform: platform.Twitch, Slug: "forsen"}
	if err := m.AddChannel(context.Background(), ref, platform.ModeAnonymous); err != nil {
		t.Fatal(err)
	}
	// Re-read from the store: the channel must be persisted in the active profile.
	stored, _ := mem.Profiles().Get(context.Background(), p.ID)
	doc, _ := Migrate(stored.Doc)
	if len(doc.Channels) != 1 || doc.Channels[0].Slug != "forsen" {
		t.Fatalf("after AddChannel, doc channels = %+v", doc.Channels)
	}

	if err := m.RemoveChannel(context.Background(), ref); err != nil {
		t.Fatal(err)
	}
	stored, _ = mem.Profiles().Get(context.Background(), p.ID)
	doc, _ = Migrate(stored.Doc)
	if len(doc.Channels) != 0 {
		t.Errorf("after RemoveChannel, doc channels = %+v", doc.Channels)
	}
}

func TestMigrate(t *testing.T) {
	if d, err := Migrate(nil); err != nil || d.Version != CurrentVersion {
		t.Errorf("empty raw → %+v, %v", d, err)
	}
	if d, err := Migrate(json.RawMessage(`{"channels":[{"platform":"kick","slug":"x"}]}`)); err != nil || d.Version != CurrentVersion || len(d.Channels) != 1 {
		t.Errorf("unversioned doc → %+v, %v", d, err)
	}
	if _, err := Migrate(json.RawMessage(`not json`)); err == nil {
		t.Error("bad json did not error")
	}
}

func TestActivate_AppliesLoggingPolicy(t *testing.T) {
	mem := store.NewMemory(clock.NewFake(time.Unix(1000, 0)))
	lg := &fakeLogging{}
	m := New(mem.Profiles(), &fakeChannels{}, &fakeFilter{}, lg, &fakeEmitter{}, clock.NewFake(time.Unix(1000, 0)))
	p := createProfile(t, mem, "log", Doc{Version: CurrentVersion, Logging: Logging{Enabled: true, Retention: "30d"}})

	if err := m.Activate(context.Background(), p.ID); err != nil {
		t.Fatal(err)
	}
	if !lg.enabled || lg.retention != "30d" {
		t.Errorf("logging policy = {enabled:%v retention:%q}, want {true 30d}", lg.enabled, lg.retention)
	}
}

func TestActivate_UnknownProfileErrors(t *testing.T) {
	m, _, _, _, _ := newManager(t)
	if err := m.Activate(context.Background(), "nope"); err == nil {
		t.Error("activating an unknown profile returned nil error")
	}
}

func TestActiveID_TracksActivation(t *testing.T) {
	m, mem, _, _, _ := newManager(t)
	if m.ActiveID() != "" {
		t.Error("ActiveID should be empty before activation")
	}
	p := createProfile(t, mem, "p", NewDoc())
	_ = m.Activate(context.Background(), p.ID)
	if m.ActiveID() != p.ID {
		t.Errorf("ActiveID = %q, want %q", m.ActiveID(), p.ID)
	}
}

func TestAddRemoveChannel_BeforeActivationNoop(t *testing.T) {
	m, _, _, _, _ := newManager(t)
	ref := platform.ChannelRef{Platform: platform.Twitch, Slug: "x"}
	if err := m.AddChannel(context.Background(), ref, ""); err != nil {
		t.Errorf("AddChannel before activation: %v", err)
	}
	if err := m.RemoveChannel(context.Background(), ref); err != nil {
		t.Errorf("RemoveChannel before activation: %v", err)
	}
}

func TestAddChannel_IdempotentAndRemoveMissing(t *testing.T) {
	m, _, _, _, _ := newManager(t)
	p, _ := m.EnsureDefault(context.Background())
	_ = m.Activate(context.Background(), p.ID)
	ref := platform.ChannelRef{Platform: platform.Twitch, Slug: "forsen"}

	_ = m.AddChannel(context.Background(), ref, platform.ModeAnonymous)
	_ = m.AddChannel(context.Background(), ref, platform.ModeAnonymous) // duplicate: no-op
	// Removing a channel that isn't there is a no-op, not an error.
	if err := m.RemoveChannel(context.Background(), platform.ChannelRef{Platform: platform.Twitch, Slug: "ghost"}); err != nil {
		t.Errorf("RemoveChannel missing: %v", err)
	}
}

func TestChannelSpec_DefaultMode(t *testing.T) {
	// A spec with no mode joins as Automatic; an explicit mode is preserved.
	if got := (ChannelSpec{}).mode(); got != platform.ModeAutomatic {
		t.Errorf("empty mode = %q, want automatic", got)
	}
	if got := spec("x").mode(); got != platform.ModeAnonymous {
		t.Errorf("explicit mode = %q, want anonymous", got)
	}
}
