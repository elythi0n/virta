// Package platform defines the contract every streaming platform implements: the
// Adapter port and the UnifiedMessage model that all chat normalizes into.
//
// This file is the documentation of the subsystem. An implementation lives in
// a subpackage (platform/twitch, platform/kick, platform/x, …), imports only this package,
// and is wired in internal/app — never imported elsewhere (enforced by depguard).
//
// The golden rule: adding a platform means a new subpackage + fixtures + one
// wire.go line + UI tokens. If it forces a change to the engine, pipeline, store, API, or
// frontends, that is a design bug in *this* contract — fix the boundary, not the caller.
package platform

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

// Platform identifies a streaming platform. Open string type: adding YouTube/TikTok/etc.
// introduces a new const here and a new adapter subpackage, nothing more.
type Platform string

const (
	Twitch Platform = "twitch"
	Kick   Platform = "kick"
	X      Platform = "x"
)

// ConnMode is how a channel is connected. Users choose per platform; Automatic
// picks the most robust available method and upgrades on sign-in.
type ConnMode string

const (
	ModeAutomatic     ConnMode = "automatic"     // recommended default — most robust available
	ModeAnonymous     ConnMode = "anonymous"     // read-only, no account
	ModeAuthenticated ConnMode = "authenticated" // official, signed in (read + send + mod)
	ModeSession       ConnMode = "session"       // user's own browser session (X best-effort)
)

// StabilityTier communicates how reliable a platform's access is, surfaced in the UI so a
// best-effort platform (X today, TikTok later) is honestly labeled.
type StabilityTier string

const (
	TierOfficial   StabilityTier = "official"   // official API, supported
	TierUnofficial StabilityTier = "unofficial" // works, no SLA (Kick Pusher read)
	TierBestEffort StabilityTier = "besteffort" // fragile, expected to break (X scrape)
)

// Capabilities report what an adapter can do *right now*, given its connection mode and
// auth state. The UI greys out unavailable actions from this — no frontend hardcodes
// platform knowledge. Capabilities are dynamic: signing in flips Send/Moderation.
type Capabilities struct {
	ReadAnonymous bool          // can read without an account
	ReadAuthed    bool          // can read with an account (richer events)
	Send          bool          // can send messages
	Moderation    bool          // can ban/timeout/delete/chat-settings
	Replies       bool          // supports reply-to-message
	HeldQueue     bool          // supports an AutoMod/held-message queue
	Stability     StabilityTier // honesty label for the UI
}

// MessageType classifies a normalized message. Deletions are an Event, not a type.
type MessageType string

const (
	TypeChat         MessageType = "chat"
	TypeAction       MessageType = "action" // /me
	TypeReply        MessageType = "reply"
	TypeSub          MessageType = "sub"
	TypeResub        MessageType = "resub"
	TypeGiftSub      MessageType = "giftsub"
	TypeRaid         MessageType = "raid"
	TypeHost         MessageType = "host"
	TypeFollow       MessageType = "follow"
	TypeAnnouncement MessageType = "announcement"
	TypeModeration   MessageType = "moderation" // mod action surfaced into the feed
	TypeSystem       MessageType = "system"     // platform/system notice
)

// SegmentKind tags a piece of message content. Parsing happens once, in the adapter — never
// in a frontend.
type SegmentKind string

const (
	SegText    SegmentKind = "text"
	SegEmote   SegmentKind = "emote"
	SegMention SegmentKind = "mention"
	SegLink    SegmentKind = "link"
	SegCheer   SegmentKind = "cheer"
	SegMasked  SegmentKind = "masked" // profanity replaced with a mask token; original in Reveal
)

// EmoteProvider identifies where an emote comes from. Third-party providers (7TV/BTTV/FFZ)
// are resolved by internal/emotes and merged per channel.
type EmoteProvider string

const (
	EmoteTwitch EmoteProvider = "twitch"
	EmoteKick   EmoteProvider = "kick"
	Emote7TV    EmoteProvider = "7tv"
	EmoteBTTV   EmoteProvider = "bttv"
	EmoteFFZ    EmoteProvider = "ffz"
)

// EmoteRef is a resolved emote: enough for any frontend to render it without lookups.
type EmoteRef struct {
	Provider    EmoteProvider `json:"provider"`
	ID          string        `json:"id"`
	Name        string        `json:"name"`
	URLTemplate string        `json:"url_template"` // CDN template; {size} substituted by the frontend
	Animated    bool          `json:"animated"`
}

// Badge is one author badge (broadcaster, mod, subscriber, …). Artwork is resolved
// separately (M2); ID/Set/Version identify it.
type Badge struct {
	Set     string `json:"set"`     // e.g. "subscriber", "moderator"
	Version string `json:"version"` // e.g. "12" (months)
	Title   string `json:"title,omitempty"`
}

// Author is the message sender, normalized across platforms.
type Author struct {
	ID          string  `json:"id"`           // platform user id
	Login       string  `json:"login"`        // lowercase handle
	DisplayName string  `json:"display_name"` // as the platform presents it
	Color       string  `json:"color"`        // hex; "" if unset (frontend contrast-clamps)
	Badges      []Badge `json:"badges,omitempty"`
}

// Segment is one ordered piece of message content. Exactly one of the optional fields is
// populated according to Kind; Text always carries the literal/displayed text.
type Segment struct {
	Kind  SegmentKind `json:"kind"`
	Text  string      `json:"text"`            // literal text, emote name, mention text, URL, or the mask token
	Emote *EmoteRef   `json:"emote,omitempty"` // when Kind == SegEmote
	// CheerBits is set when Kind == SegCheer.
	CheerBits int `json:"cheer_bits,omitempty"`
	// Reveal holds the original text behind a SegMasked segment, for the local feed's
	// click-to-reveal. Text is the mask token (so PlainText — and thus TTS/webhooks/logging —
	// stays masked); outbound integrations must not forward Reveal.
	Reveal string `json:"reveal,omitempty"`
}

// MessageRef points at another message — used for replies. The platform message
// id lets the engine resolve the parent via its per-channel id→ULID map.
type MessageRef struct {
	PlatformMessageID string `json:"platform_message_id"`
	AuthorLogin       string `json:"author_login"`
	TextSnippet       string `json:"text_snippet"` // short preview of the parent
}

// UnifiedMessage is the contract of the whole product. Every adapter emits this; every
// frontend renders it. Change it carefully and version it on the wire.
type UnifiedMessage struct {
	ID                string          `json:"id"`                  // engine-assigned ULID (sortable, unique across platforms; set by the engine, not the adapter)
	PlatformMessageID string          `json:"platform_message_id"` // the platform's own id (for dedup, deletion mapping, replies)
	Platform          Platform        `json:"platform"`
	Channel           ChannelRef      `json:"channel"`
	Type              MessageType     `json:"type"`
	Author            Author          `json:"author"`
	Segments          []Segment       `json:"segments"`
	ReplyTo           *MessageRef     `json:"reply_to,omitempty"`
	SentAt            time.Time       `json:"sent_at"`     // platform timestamp (displayed)
	ReceivedAt        time.Time       `json:"received_at"` // local arrival (feed ordering key)
	Ephemeral         bool            `json:"-"`           // true → never persisted; the one flag enforcing the logging-off guarantee
	Raw               json.RawMessage `json:"-"`           // original payload, retained bounded for debugging
	Annotations       *Annotations    `json:"annotations,omitempty"`
}

// Annotations are filter/view results attached by pipeline stages. They are display hints —
// a hidden or highlighted message is still logged and counted in stats; only frontends act on
// them — so the feed stays consistent across every client.
type Annotations struct {
	Hidden    bool   `json:"hidden,omitempty"`     // matched a hide rule; frontends don't render it
	Highlight string `json:"highlight,omitempty"`  // id of the rule that highlighted it ("" = not highlighted)
	Masked    bool   `json:"masked,omitempty"`     // profanity was masked in Segments
	FirstTime bool   `json:"first_time,omitempty"` // chatter's first message (platform tag, or first seen this session)
}

// Annotate returns the message's annotations, allocating them on first use, so stages can set
// fields without nil checks.
func (m *UnifiedMessage) Annotate() *Annotations {
	if m.Annotations == nil {
		m.Annotations = &Annotations{}
	}
	return m.Annotations
}

// PlainText returns the message body with emotes rendered as their names — the form used
// for logging, search indexing, and TTS. Frontends render Segments directly instead.
// Segments are concatenated verbatim: each carries its own surrounding whitespace, so the
// result reconstructs the original text exactly (e.g. "hello " + "Kappa" + " there").
func (m *UnifiedMessage) PlainText() string {
	var b []byte
	for _, s := range m.Segments {
		b = append(b, s.Text...)
	}
	return string(b)
}

// ChannelRef identifies a channel on a platform.
type ChannelRef struct {
	Platform    Platform `json:"platform"`
	ID          string   `json:"id"`   // platform channel/room id (may be resolved lazily, e.g. Kick chatroom id)
	Slug        string   `json:"slug"` // login / kick slug / x handle — what the user typed
	DisplayName string   `json:"display_name,omitempty"`
}

// ---- Health & reason codes machine codes, never prose ----

// HealthState is the coarse adapter/channel state.
type HealthState string

const (
	HealthOK       HealthState = "ok"
	HealthDegraded HealthState = "degraded"
	HealthDown     HealthState = "down"
)

// ReasonCode is a machine-readable cause. Frontends map it to user copy; the raw code +
// Detail appear only in diagnostics. Open type — adapters may emit codes beyond
// these constants.
type ReasonCode string

const (
	ReasonNone            ReasonCode = ""
	ReasonReconnecting    ReasonCode = "reconnecting"
	ReasonAuthRequired    ReasonCode = "auth_required"
	ReasonAuthExpired     ReasonCode = "auth_expired"
	ReasonRateLimited     ReasonCode = "rate_limited"
	ReasonResolverBlocked ReasonCode = "resolver_blocked" // Kick chatroom-id lookup blocked
	ReasonSelectorDrift   ReasonCode = "selector_drift"   // X scrape selectors changed
	ReasonNoBrowser       ReasonCode = "no_browser"       // X: no Chromium-family browser found
	ReasonChannelNotFound ReasonCode = "channel_not_found"
	ReasonUpstreamDown    ReasonCode = "upstream_down"
)

// HealthStatus is an adapter's or channel's current state. Detail is technical (for the
// diagnostics panel), never shown as primary UI copy.
type HealthStatus struct {
	State  HealthState `json:"state"`
	Reason ReasonCode  `json:"reason,omitempty"`
	Detail string      `json:"detail,omitempty"`
}

// ---- Sending & moderation ----

// SendOpts carries optional send parameters.
type SendOpts struct {
	Action        bool   // send as a /me action
	ReplyParentID string // platform message id to reply to; "" for a normal message
}

// ModActionType enumerates moderation operations. The single typed action path that mod
// buttons, slash commands, and the held-message queue all funnel
// through — so behavior and capability/rate checks live in one place.
type ModActionType string

const (
	ModBan           ModActionType = "ban"
	ModUnban         ModActionType = "unban"
	ModTimeout       ModActionType = "timeout"
	ModUntimeout     ModActionType = "untimeout"
	ModDeleteMessage ModActionType = "delete_message"
	ModClear         ModActionType = "clear"
	ModSetSlow       ModActionType = "set_slow"
	ModSetFollowers  ModActionType = "set_followers_only"
	ModSetEmoteOnly  ModActionType = "set_emote_only"
	ModSetUniqueChat ModActionType = "set_unique_chat"
	ModApproveHeld   ModActionType = "approve_held"
	ModDenyHeld      ModActionType = "deny_held"
)

// ModAction is one moderation request. Fields are interpreted per Type; unused fields are
// zero. Targeting is by platform ids/handles the adapter understands.
type ModAction struct {
	Type            ModActionType
	Channel         ChannelRef
	TargetUserID    string        // ban/timeout/unban
	TargetMessageID string        // delete_message / approve_held / deny_held (platform id)
	Duration        time.Duration // timeout / slow interval
	Enabled         bool          // for the set_* toggles
	Reason          string        // optional, where the platform supports it
}

// ---- Events ----

// Event is anything an adapter emits on its Events() channel: a normalized message, a
// deletion, or a health transition. Sealed interface — the pipeline type-switches on it
// and only MessageEvent runs through stages. Add a case here, not a new channel.
type Event interface{ isEvent() }

// MessageEvent carries a normalized chat message (the high-volume path).
type MessageEvent struct{ Message UnifiedMessage }

// MessageDeletedEvent reports a message removal (CLEARMSG, deletion). Adapters set Channel
// and PlatformMessageID; the engine resolves MessageID to the ULID it assigned the original
// message (empty if that message was never seen or has aged out of the bounded map) so
// frontends can strike the exact row they rendered.
type MessageDeletedEvent struct {
	Channel           ChannelRef
	PlatformMessageID string // platform's own id for the removed message
	MessageID         string // engine ULID of the original message, resolved by the engine ("" if unknown)
}

// ChannelClearEvent reports a full or per-user chat clear (CLEARCHAT).
type ChannelClearEvent struct {
	Channel      ChannelRef
	TargetUserID string // "" = clear entire channel
}

// HealthEvent reports an adapter- or channel-level state change.
type HealthEvent struct {
	Channel *ChannelRef // nil = adapter-wide
	Status  HealthStatus
}

// ChatSettings is a channel's chat-mode configuration (slow mode, followers-only, etc.).
// A platform may send these incrementally (only the changed setting), so consumers that
// need the full picture should merge updates per channel rather than replace.
type ChatSettings struct {
	EmoteOnly            bool `json:"emote_only"`
	SubsOnly             bool `json:"subs_only"`
	UniqueChat           bool `json:"unique_chat"`            // no duplicate messages ("r9k")
	FollowersOnlyMinutes int  `json:"followers_only_minutes"` // -1 = off, 0 = any follower, >0 = minimum follow age
	SlowSeconds          int  `json:"slow_seconds"`           // 0 = off
}

// ChatSettingsEvent reports a channel's chat-mode settings (ROOMSTATE), surfaced so the UI
// can show and toggle slow mode, followers-only, etc.
type ChatSettingsEvent struct {
	Channel  ChannelRef
	Settings ChatSettings
}

// StatsSnapshot is a channel's live activity over a rolling window. Frontends combine
// per-channel snapshots into the cross-platform totals shown in the stats panel.
type StatsSnapshot struct {
	WindowSeconds  int          `json:"window_seconds"`
	MessagesPerSec float64      `json:"messages_per_sec"`
	UniqueChatters int          `json:"unique_chatters"`
	TopEmotes      []EmoteCount `json:"top_emotes,omitempty"`
}

// EmoteCount is one entry in the emote leaderboard.
type EmoteCount struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// StatsEvent reports a channel's rolling stats, emitted periodically by the stats aggregator.
type StatsEvent struct {
	Channel ChannelRef
	Stats   StatsSnapshot
}

// ProfileChangedEvent announces that the active workspace profile switched, so every attached
// frontend re-renders against the new channel set, filters, and layout.
type ProfileChangedEvent struct {
	ProfileID string
	Name      string
}

func (MessageEvent) isEvent()        {}
func (MessageDeletedEvent) isEvent() {}
func (ChannelClearEvent) isEvent()   {}
func (HealthEvent) isEvent()         {}
func (ChatSettingsEvent) isEvent()   {}
func (StatsEvent) isEvent()          {}
func (ProfileChangedEvent) isEvent() {}

// ---- The Adapter port ----

// ErrUnsupported is returned by Send/Moderate when the current Capabilities don't allow the
// operation (e.g. sending while anonymous). Callers check Capabilities first; this is the
// defensive backstop.
var ErrUnsupported = errors.New("platform: operation not supported in current mode")

// Adapter is one streaming platform. Implementations run their own goroutine tree under a
// supervisor (reconnect/backoff/circuit-breaker); the engine consumes Events() and never
// blocks the adapter. All methods must be safe for concurrent use.
type Adapter interface {
	// Platform returns which platform this adapter serves.
	Platform() Platform

	// Capabilities reports what the adapter can do right now (changes with auth/mode).
	Capabilities() Capabilities

	// Join begins reading a channel in the given mode. Idempotent per channel.
	Join(ctx context.Context, ch ChannelRef, mode ConnMode) error

	// Leave stops reading a channel.
	Leave(ch ChannelRef) error

	// Send posts a message. Returns ErrUnsupported if Capabilities().Send is false.
	Send(ctx context.Context, ch ChannelRef, text string, opts SendOpts) error

	// Moderate performs a moderation action. Returns ErrUnsupported if not permitted.
	Moderate(ctx context.Context, action ModAction) error

	// Events is the adapter's output stream. Closed when the adapter is Closed.
	Events() <-chan Event

	// Health is the adapter-wide status (per-channel detail arrives via HealthEvent).
	Health() HealthStatus

	// Close shuts the adapter down and closes Events(). Idempotent.
	Close() error
}
