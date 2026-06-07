package obsws

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"sync"
	"sync/atomic"
	"time"

	"github.com/elythi0n/virta/internal/platform"
	"github.com/elythi0n/virta/internal/secrets"
	"github.com/elythi0n/virta/internal/store"
)

// ConnState describes the current connection state of the manager.
type ConnState string

const (
	StateDisconnected ConnState = "disconnected"
	StateConnecting   ConnState = "connecting"
	StateConnected    ConnState = "connected"
	StateError        ConnState = "error"
)

// StatKind names a stats counter that can be pushed to an OBS text source.
type StatKind string

const (
	StatMsgsPerMin     StatKind = "msgs_per_min"
	StatUniqueChatters StatKind = "unique_chatters"
)

// DataMapping binds a StatKind to an OBS input (text source) by name.
type DataMapping struct {
	Stat       StatKind `json:"stat"`
	SourceName string   `json:"source_name"`
}

// ActionKind names an OBS action to take in response to a chat event.
type ActionKind string

const (
	ActionSwitchScene ActionKind = "switch_scene"
)

// EventRule fires an OBS action when a chat event matches.
type EventRule struct {
	Trigger    string     `json:"trigger"`     // "raid", "sub", "sub_gift", "resub"
	ChannelKey string     `json:"channel_key"` // "platform:slug", or "" for any
	Action     ActionKind `json:"action"`
	Target     string     `json:"target"` // scene name for switch_scene
}

// Config is the persisted configuration for the OBS WebSocket integration.
type Config struct {
	Enabled         bool          `json:"enabled"`
	Host            string        `json:"host"`
	Port            int           `json:"port"`
	DataMappings    []DataMapping `json:"data_mappings,omitempty"`
	EventRules      []EventRule   `json:"event_rules,omitempty"`
	UpdateIntervalS int           `json:"update_interval_s"`
}

func defaultConfig() Config {
	return Config{
		Host:            "localhost",
		Port:            4455,
		UpdateIntervalS: 5,
	}
}

// Status is the current connection status returned by the API.
type Status struct {
	State            ConnState `json:"state"`
	OBSVersion       string    `json:"obs_version,omitempty"`
	WebSocketVersion string    `json:"websocket_version,omitempty"`
	Error            string    `json:"error,omitempty"`
}

// SourceInfo describes one OBS input source.
type SourceInfo struct {
	Name string `json:"name"`
	Kind string `json:"kind"`
}

// SceneList is the list of OBS scenes with the active one marked.
type SceneList struct {
	Scenes  []string `json:"scenes"`
	Current string   `json:"current"`
}

const vaultKey = "obsws:password"
const settingsKey = "obsws.config"

// Manager implements pipeline.Sink and manages the OBS WebSocket connection.
type Manager struct {
	vault secrets.Vault
	store store.SettingsRepo
	log   *slog.Logger

	cfg atomic.Pointer[Config]

	mu      sync.Mutex
	conn    *client
	state   ConnState
	lastErr string
	obsVer  string
	wsVer   string

	latestStats sync.Map // channel key (string) -> platform.StatsSnapshot

	stopCh chan struct{}
	wg     sync.WaitGroup
}

// New creates a Manager and loads the stored configuration. Call Start to begin connecting.
func New(vault secrets.Vault, st store.SettingsRepo, log *slog.Logger) *Manager {
	m := &Manager{
		vault:  vault,
		store:  st,
		log:    log,
		stopCh: make(chan struct{}),
		state:  StateDisconnected,
	}
	cfg := defaultConfig()
	if raw, err := st.Get(context.Background(), settingsKey); err == nil {
		if err2 := json.Unmarshal(raw.Data, &cfg); err2 != nil {
			log.Warn("obsws: failed to parse stored config", "err", err2)
		}
	}
	m.cfg.Store(&cfg)
	return m
}

// Start spawns the reconnection loop goroutine.
func (m *Manager) Start() {
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		m.loop()
	}()
}

// Stop closes stopCh and waits for the loop to exit, then disconnects.
func (m *Manager) Stop() {
	select {
	case <-m.stopCh:
	default:
		close(m.stopCh)
	}
	m.wg.Wait()
	m.mu.Lock()
	if m.conn != nil {
		m.conn.close()
		m.conn = nil
	}
	m.state = StateDisconnected
	m.mu.Unlock()
}

// Name satisfies pipeline.Sink.
func (m *Manager) Name() string { return "obsws" }

// Consume handles one pipeline event. Stats are stored for the push loop; message events
// may trigger OBS actions.
func (m *Manager) Consume(_ context.Context, ev platform.Event) error {
	cfg := m.cfg.Load()
	if !cfg.Enabled {
		return nil
	}
	switch e := ev.(type) {
	case platform.StatsEvent:
		m.latestStats.Store(e.Channel.Key(), e.Stats)
	case platform.MessageEvent:
		if len(cfg.EventRules) == 0 {
			return nil
		}
		trigger := messageTypeTrigger(e.Message.Type)
		if trigger == "" {
			return nil
		}
		key := e.Message.Channel.Key()
		for _, r := range cfg.EventRules {
			if r.Trigger != trigger {
				continue
			}
			if r.ChannelKey != "" && r.ChannelKey != key {
				continue
			}
			m.fireRule(r)
		}
	}
	return nil
}

// messageTypeTrigger maps platform message types to EventRule trigger strings.
func messageTypeTrigger(t platform.MessageType) string {
	switch t {
	case platform.TypeRaid:
		return "raid"
	case platform.TypeSub:
		return "sub"
	case platform.TypeGiftSub:
		return "sub_gift"
	case platform.TypeResub:
		return "resub"
	default:
		return ""
	}
}

// Close satisfies pipeline.Sink by stopping the manager.
func (m *Manager) Close() error {
	m.Stop()
	return nil
}

// Status returns the current connection status.
func (m *Manager) Status() Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	return Status{
		State:            m.state,
		OBSVersion:       m.obsVer,
		WebSocketVersion: m.wsVer,
		Error:            m.lastErr,
	}
}

// GetConfig returns the current config and whether a password is stored in the vault.
func (m *Manager) GetConfig(ctx context.Context) (Config, bool, error) {
	cfg := *m.cfg.Load()
	_, err := m.vault.Get(ctx, vaultKey)
	hasPassword := err == nil
	return cfg, hasPassword, nil
}

// SetConfig persists a new config and triggers a reconnect.
func (m *Manager) SetConfig(ctx context.Context, cfg Config, password string) error {
	switch password {
	case "CLEAR":
		_ = m.vault.Delete(ctx, vaultKey)
	case "":
		// no change to stored password
	default:
		if err := m.vault.Set(ctx, vaultKey, password); err != nil {
			return fmt.Errorf("obsws: store password: %w", err)
		}
	}

	data, err := json.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("obsws: marshal config: %w", err)
	}
	if err := m.store.Put(ctx, store.Setting{Scope: settingsKey, Data: data}); err != nil {
		return fmt.Errorf("obsws: persist config: %w", err)
	}
	m.cfg.Store(&cfg)

	// Close the current connection so the loop reconnects with the new config.
	m.mu.Lock()
	if m.conn != nil {
		m.conn.close()
	}
	m.mu.Unlock()
	return nil
}

// GetSources proxies GetInputList; returns an error if not connected.
func (m *Manager) GetSources(ctx context.Context) ([]SourceInfo, error) {
	c := m.activeConn()
	if c == nil {
		return nil, errors.New("not connected to OBS")
	}
	rd, err := c.request(ctx, "GetInputList", nil)
	if err != nil {
		return nil, err
	}
	if !rd.Status.Result {
		return nil, fmt.Errorf("OBS error %d: %s", rd.Status.Code, rd.Status.Comment)
	}
	var resp getInputListResponse
	if err := json.Unmarshal(rd.Payload, &resp); err != nil {
		return nil, err
	}
	out := make([]SourceInfo, 0, len(resp.Inputs))
	for _, item := range resp.Inputs {
		out = append(out, SourceInfo{Name: item.InputName, Kind: item.InputKind})
	}
	return out, nil
}

// GetScenes proxies GetSceneList; returns an error if not connected.
func (m *Manager) GetScenes(ctx context.Context) (SceneList, error) {
	c := m.activeConn()
	if c == nil {
		return SceneList{}, errors.New("not connected to OBS")
	}
	rd, err := c.request(ctx, "GetSceneList", nil)
	if err != nil {
		return SceneList{}, err
	}
	if !rd.Status.Result {
		return SceneList{}, fmt.Errorf("OBS error %d: %s", rd.Status.Code, rd.Status.Comment)
	}
	var resp getSceneListResponse
	if err := json.Unmarshal(rd.Payload, &resp); err != nil {
		return SceneList{}, err
	}
	names := make([]string, 0, len(resp.Scenes))
	for _, s := range resp.Scenes {
		names = append(names, s.SceneName)
	}
	return SceneList{Scenes: names, Current: resp.CurrentProgramSceneName}, nil
}

// TestSource sends a SetInputSettings request to set the "text" field of a text source.
func (m *Manager) TestSource(ctx context.Context, sourceName, value string) error {
	c := m.activeConn()
	if c == nil {
		return errors.New("not connected to OBS")
	}
	return m.setTextSource(ctx, c, sourceName, value)
}

// Detect tries to connect to OBS on localhost:4455 with no password and a 3-second timeout.
func Detect(ctx context.Context) (bool, error) {
	tctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	c, _, err := dial(tctx, "localhost", "4455", "")
	if err != nil {
		return false, nil
	}
	c.close()
	return true, nil
}

// activeConn returns the current connected client, or nil.
func (m *Manager) activeConn() *client {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.state == StateConnected {
		return m.conn
	}
	return nil
}

// loop is the reconnect loop. It runs until stopCh is closed.
func (m *Manager) loop() {
	backoff := 2 * time.Second
	const maxBackoff = 30 * time.Second

	for {
		select {
		case <-m.stopCh:
			return
		default:
		}

		cfg := m.cfg.Load()
		if !cfg.Enabled {
			select {
			case <-m.stopCh:
				return
			case <-time.After(2 * time.Second):
			}
			continue
		}

		m.mu.Lock()
		m.state = StateConnecting
		m.lastErr = ""
		m.mu.Unlock()

		password := ""
		if pw, err := m.vault.Get(context.Background(), vaultKey); err == nil {
			password = pw
		}

		port := fmt.Sprintf("%d", cfg.Port)
		dialCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		c, hd, err := dial(dialCtx, cfg.Host, port, password)
		cancel()
		if err != nil {
			m.mu.Lock()
			m.state = StateError
			m.lastErr = err.Error()
			m.mu.Unlock()
			m.log.Debug("obsws: connection failed", "err", err)
			select {
			case <-m.stopCh:
				return
			case <-time.After(backoff):
			}
			backoff = time.Duration(math.Min(float64(backoff*2), float64(maxBackoff)))
			continue
		}
		backoff = 2 * time.Second

		m.mu.Lock()
		m.conn = c
		m.state = StateConnected
		m.lastErr = ""
		if hd != nil {
			m.obsVer = ""
			m.wsVer = hd.OBSWebSocketVersion
		}
		m.mu.Unlock()

		// Fetch OBS version for status.
		go func() {
			vctx, vcancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer vcancel()
			if rd, verr := c.request(vctx, "GetVersion", nil); verr == nil && rd.Status.Result {
				var vr getVersionResponse
				if jerr := json.Unmarshal(rd.Payload, &vr); jerr == nil {
					m.mu.Lock()
					m.obsVer = vr.OBSVersion
					m.wsVer = vr.OBSWebSocketVersion
					m.mu.Unlock()
				}
			}
		}()

		m.log.Info("obsws: connected", "host", cfg.Host, "port", cfg.Port)

		readDone := make(chan struct{})
		loopCtx, loopCancel := context.WithCancel(context.Background())

		m.wg.Add(1)
		go func() {
			defer m.wg.Done()
			defer close(readDone)
			c.readLoop(loopCtx)
		}()

		if cfg.UpdateIntervalS > 0 && len(cfg.DataMappings) > 0 {
			m.wg.Add(1)
			go func() {
				defer m.wg.Done()
				m.statLoop(loopCtx, c, cfg)
			}()
		}

		select {
		case <-m.stopCh:
			loopCancel()
			c.close()
			<-readDone
			return
		case <-readDone:
			loopCancel()
		}

		m.mu.Lock()
		m.conn = nil
		m.state = StateDisconnected
		m.mu.Unlock()
		m.log.Debug("obsws: disconnected; will reconnect")
	}
}

// statLoop ticks every UpdateIntervalS and pushes stats to OBS text sources.
func (m *Manager) statLoop(ctx context.Context, c *client, cfg *Config) {
	interval := time.Duration(cfg.UpdateIntervalS) * time.Second
	if interval <= 0 {
		return
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			m.pushStats(ctx, c, cfg)
		}
	}
}

// pushStats computes aggregated stat values and sends them to the mapped OBS text sources.
func (m *Manager) pushStats(ctx context.Context, c *client, cfg *Config) {
	// Aggregate across all channels.
	var totalMPS float64
	var totalChatters int
	m.latestStats.Range(func(_, v any) bool {
		snap, ok := v.(platform.StatsSnapshot)
		if ok {
			totalMPS += snap.MessagesPerSec
			totalChatters += snap.UniqueChatters
		}
		return true
	})

	for _, dm := range cfg.DataMappings {
		var text string
		switch dm.Stat {
		case StatMsgsPerMin:
			text = fmt.Sprintf("%d", int(totalMPS*60))
		case StatUniqueChatters:
			text = fmt.Sprintf("%d", totalChatters)
		default:
			continue
		}
		if err := m.setTextSource(ctx, c, dm.SourceName, text); err != nil {
			m.log.Debug("obsws: setTextSource failed", "source", dm.SourceName, "err", err)
		}
	}
}

// setTextSource sends a SetInputSettings request to set the text of a GDI+ or freetype2 text source.
func (m *Manager) setTextSource(ctx context.Context, c *client, sourceName, text string) error {
	payload := map[string]any{
		"inputName": sourceName,
		"inputSettings": map[string]any{
			"text": text,
		},
	}
	rd, err := c.request(ctx, "SetInputSettings", payload)
	if err != nil {
		return err
	}
	if !rd.Status.Result {
		return fmt.Errorf("OBS error %d: %s", rd.Status.Code, rd.Status.Comment)
	}
	return nil
}

// fireRule executes the action prescribed by an EventRule.
func (m *Manager) fireRule(r EventRule) {
	c := m.activeConn()
	if c == nil {
		return
	}
	switch r.Action {
	case ActionSwitchScene:
		payload := map[string]any{"sceneName": r.Target}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := c.request(ctx, "SetCurrentProgramScene", payload); err != nil {
			m.log.Debug("obsws: SetCurrentProgramScene failed", "scene", r.Target, "err", err)
		}
	}
}
