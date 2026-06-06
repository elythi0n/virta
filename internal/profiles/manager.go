package profiles

import (
	"context"
	"fmt"
	"sync"

	"github.com/elythi0n/virta/internal/clock"
	"github.com/elythi0n/virta/internal/filter"
	"github.com/elythi0n/virta/internal/platform"
	"github.com/elythi0n/virta/internal/store"
)

// Channels is the join/leave surface the manager drives on activation (the engine satisfies it).
type Channels interface {
	Join(ctx context.Context, ch platform.ChannelRef, mode platform.ConnMode) error
	Leave(ch platform.ChannelRef) error
}

// FilterSetter swaps the active filter ruleset (the filter Stage satisfies it).
type FilterSetter interface {
	SetRuleset(rs *filter.Ruleset)
}

// Emitter submits engine-level events into the pipeline (the runner satisfies it).
type Emitter interface {
	Submit(ev platform.Event)
}

// LoggingSetter applies the profile's logging policy (enables/disables persistence and sets
// retention). The wiring layer fans it to the engine, the logbook sink, and the sweeper.
type LoggingSetter interface {
	SetLogging(enabled bool, retention string)
}

// Manager owns the active profile and performs atomic switches. Safe for concurrent use.
type Manager struct {
	repo     store.ProfileRepo
	channels Channels
	filter   FilterSetter
	logging  LoggingSetter
	emitter  Emitter
	clk      clock.Clock

	// activateMu serializes whole activations so two concurrent switches can't diff against the
	// same stale previous profile and leave the engine joined to a mix of both. It is held
	// across the (I/O-bearing) join/leave work; mu only guards the field reads/writes below, so
	// AddChannel/Health-style callers never block on a switch.
	activateMu sync.Mutex

	mu       sync.Mutex
	active   Doc
	activeID string
	loaded   bool
}

// New builds a profile manager.
func New(repo store.ProfileRepo, channels Channels, filter FilterSetter, logging LoggingSetter, emitter Emitter, clk clock.Clock) *Manager {
	return &Manager{repo: repo, channels: channels, filter: filter, logging: logging, emitter: emitter, clk: clk}
}

// EnsureDefault returns the default profile, creating an empty one on first run. Everything a
// user does without thinking about profiles is saved here.
func (m *Manager) EnsureDefault(ctx context.Context) (store.Profile, error) {
	if p, err := m.repo.Default(ctx); err == nil {
		return p, nil
	}
	doc, err := NewDoc().Marshal()
	if err != nil {
		return store.Profile{}, err
	}
	p, err := m.repo.Create(ctx, "default", doc)
	if err != nil {
		return store.Profile{}, err
	}
	if err := m.repo.SetDefault(ctx, p.ID); err != nil {
		return store.Profile{}, err
	}
	return p, nil
}

// Activate switches to the profile: it joins channels new to this profile, leaves ones the
// previous profile had and this one doesn't, leaves channels common to both untouched (so
// their feed never gaps), swaps the filter ruleset, and announces the change. A join that
// fails (e.g. a blocked resolver) doesn't abort the switch — it surfaces via channel health.
func (m *Manager) Activate(ctx context.Context, id string) error {
	m.activateMu.Lock()
	defer m.activateMu.Unlock()

	p, err := m.repo.Get(ctx, id)
	if err != nil {
		return fmt.Errorf("profiles: activate %s: %w", id, err)
	}
	doc, err := Migrate(p.Doc)
	if err != nil {
		return fmt.Errorf("profiles: decode %s: %w", id, err)
	}

	m.mu.Lock()
	prev := m.active
	prevLoaded := m.loaded
	m.mu.Unlock()

	var prevChannels []ChannelSpec
	if prevLoaded {
		prevChannels = prev.Channels
	}
	// Apply the logging policy first so messages arriving mid-activation are flagged correctly.
	m.logging.SetLogging(doc.Logging.Enabled, doc.Logging.Retention)

	add, remove := diffChannels(prevChannels, doc.Channels)

	for _, ch := range remove {
		_ = m.channels.Leave(ch.Ref())
	}
	for _, ch := range add {
		_ = m.channels.Join(ctx, ch.Ref(), doc.effectiveMode(ch))
	}

	rs, err := filter.Compile(doc.Filters)
	if err != nil {
		return fmt.Errorf("profiles: compile filters for %s: %w", id, err)
	}
	m.filter.SetRuleset(rs)

	m.mu.Lock()
	m.active = doc
	m.activeID = id
	m.loaded = true
	m.mu.Unlock()

	m.emitter.Submit(platform.ProfileChangedEvent{ProfileID: id, Name: p.Name})
	return nil
}

// AddChannel records a channel in the active profile (e.g. a user joining one outside a profile
// switch) and persists. The engine join itself is the caller's responsibility.
func (m *Manager) AddChannel(ctx context.Context, ch platform.ChannelRef, mode platform.ConnMode) error {
	m.mu.Lock()
	if !m.loaded {
		m.mu.Unlock()
		return nil
	}
	key := channelKey(ch.Platform, ch.Slug)
	for _, c := range m.active.Channels {
		if channelKey(c.Platform, c.Slug) == key {
			m.mu.Unlock()
			return nil // already present
		}
	}
	m.active.Channels = append(m.active.Channels, ChannelSpec{Platform: ch.Platform, Slug: ch.Slug, Mode: mode})
	m.mu.Unlock()
	return m.save(ctx)
}

// RemoveChannel drops a channel from the active profile and persists.
func (m *Manager) RemoveChannel(ctx context.Context, ch platform.ChannelRef) error {
	m.mu.Lock()
	if !m.loaded {
		m.mu.Unlock()
		return nil
	}
	key := channelKey(ch.Platform, ch.Slug)
	kept := m.active.Channels[:0:0]
	changed := false
	for _, c := range m.active.Channels {
		if channelKey(c.Platform, c.Slug) == key {
			changed = true
			continue
		}
		kept = append(kept, c)
	}
	if !changed {
		m.mu.Unlock()
		return nil
	}
	m.active.Channels = kept
	m.mu.Unlock()
	return m.save(ctx)
}

// Filters returns a copy of the active profile's filter rules.
func (m *Manager) Filters() []filter.Rule {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]filter.Rule, len(m.active.Filters))
	copy(out, m.active.Filters)
	return out
}

// SetFilters validates and applies a new rule set: it compiles (rejecting a bad regex), hot-swaps
// the live ruleset, records it on the active profile, and persists. A compile error changes
// nothing.
func (m *Manager) SetFilters(ctx context.Context, rules []filter.Rule) error {
	rs, err := filter.Compile(rules)
	if err != nil {
		return err
	}
	m.mu.Lock()
	if !m.loaded {
		m.mu.Unlock()
		return nil
	}
	m.active.Filters = rules
	m.mu.Unlock()
	m.filter.SetRuleset(rs)
	return m.save(ctx)
}

// Methods returns the active profile's pinned per-platform connection methods (a copy).
func (m *Manager) Methods() map[platform.Platform]platform.ConnMode {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make(map[platform.Platform]platform.ConnMode, len(m.active.Methods))
	for k, v := range m.active.Methods {
		out[k] = v
	}
	return out
}

// MethodFor returns the pinned connection method for a platform, defaulting to Automatic. Used by
// the join path so a user-added channel honors the platform's pinned method.
func (m *Manager) MethodFor(p platform.Platform) platform.ConnMode {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.active.methodFor(p)
}

// SetMethod pins a platform's connection method, reconnecting that platform's joined channels with
// the new mode (a brief drop), and persists. Empty mode clears the pin (back to Automatic).
func (m *Manager) SetMethod(ctx context.Context, p platform.Platform, mode platform.ConnMode) error {
	m.mu.Lock()
	if !m.loaded {
		m.mu.Unlock()
		return nil
	}
	if m.active.Methods == nil {
		m.active.Methods = map[platform.Platform]platform.ConnMode{}
	}
	if mode == "" || mode == platform.ModeAutomatic {
		delete(m.active.Methods, p)
	} else {
		m.active.Methods[p] = mode
	}
	// Snapshot the channels to reconnect and the effective mode, under the lock.
	type rejoin struct {
		ref  platform.ChannelRef
		mode platform.ConnMode
	}
	var todo []rejoin
	for _, c := range m.active.Channels {
		if c.Platform == p {
			todo = append(todo, rejoin{ref: c.Ref(), mode: m.active.effectiveMode(c)})
		}
	}
	m.mu.Unlock()

	for _, r := range todo {
		_ = m.channels.Leave(r.ref)
		_ = m.channels.Join(ctx, r.ref, r.mode)
	}
	return m.save(ctx)
}

// ActiveID returns the id of the active profile ("" if none activated yet).
func (m *Manager) ActiveID() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.activeID
}

// save persists the active doc.
func (m *Manager) save(ctx context.Context) error {
	m.mu.Lock()
	id := m.activeID
	doc := m.active
	m.mu.Unlock()
	if id == "" {
		return nil
	}
	raw, err := doc.Marshal()
	if err != nil {
		return err
	}
	_, err = m.repo.Update(ctx, id, raw)
	return err
}

// diffChannels returns channels to add (in next, not prev) and to remove (in prev, not next).
// Channels in both are omitted from both lists, so an activation never disturbs them.
func diffChannels(prev, next []ChannelSpec) (add, remove []ChannelSpec) {
	prevSet := make(map[string]struct{}, len(prev))
	for _, c := range prev {
		prevSet[channelKey(c.Platform, c.Slug)] = struct{}{}
	}
	nextSet := make(map[string]struct{}, len(next))
	for _, c := range next {
		nextSet[channelKey(c.Platform, c.Slug)] = struct{}{}
	}
	for _, c := range next {
		if _, ok := prevSet[channelKey(c.Platform, c.Slug)]; !ok {
			add = append(add, c)
		}
	}
	for _, c := range prev {
		if _, ok := nextSet[channelKey(c.Platform, c.Slug)]; !ok {
			remove = append(remove, c)
		}
	}
	return add, remove
}
