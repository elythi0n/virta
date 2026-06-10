// Package command parses composer input into one typed intent: a chat send, a moderation
// action, a help request, or a "not sent" hint for an unknown command. It is the
// single front door that mod buttons, slash commands, and the held-message queue all funnel
// through, so behavior and capability checks live in one place. Parsing is pure; executing the
// result (sending, moderating) is the caller's job.
package command

import (
	"strconv"
	"strings"
	"time"

	"github.com/elythi0n/virta/internal/platform"
)

// Kind is what the input resolved to.
type Kind int

const (
	KindSend Kind = iota // a chat message to send (Action set if it's a /me)
	KindMod              // a moderation action to perform
	KindHelp             // /help
	KindHint             // recognized-but-unavailable or unknown command — NOT sent
)

// Parsed is the result of parsing one composer input.
type Parsed struct {
	Kind     Kind
	Text     string             // KindSend: the message body (/me wrapper stripped)
	IsAction bool               // KindSend: a /me action message
	Action   platform.ModAction // KindMod: the typed moderation action
	Hint     string             // KindHint: user-facing reason in plain language, never sent
}

// defaultTimeout is applied to /timeout when no duration is given.
const defaultTimeout = 10 * time.Minute

// spec describes a recognized slash command: how to build its action and what capability it
// needs. A nil build means it's handled specially (/me, /help).
type spec struct {
	mod   bool // requires Moderation capability
	build func(ch platform.ChannelRef, args []string) (platform.ModAction, string)
}

var commands = map[string]spec{
	"ban":           {mod: true, build: buildBan},
	"unban":         {mod: true, build: buildTarget(platform.ModUnban)},
	"timeout":       {mod: true, build: buildTimeout},
	"untimeout":     {mod: true, build: buildTarget(platform.ModUntimeout)},
	"delete":        {mod: true, build: buildDelete},
	"clear":         {mod: true, build: buildSimple(platform.ModClear)},
	"slow":          {mod: true, build: buildSlow},
	"slowoff":       {mod: true, build: buildToggle(platform.ModSetSlow, false)},
	"followers":     {mod: true, build: buildFollowers},
	"followersoff":  {mod: true, build: buildToggle(platform.ModSetFollowers, false)},
	"emoteonly":     {mod: true, build: buildToggle(platform.ModSetEmoteOnly, true)},
	"emoteonlyoff":  {mod: true, build: buildToggle(platform.ModSetEmoteOnly, false)},
	"uniquechat":    {mod: true, build: buildToggle(platform.ModSetUniqueChat, true)},
	"uniquechatoff": {mod: true, build: buildToggle(platform.ModSetUniqueChat, false)},
}

// Parse turns input into a typed intent for channel ch, honoring caps (capabilities the
// account has there right now). Unknown commands and commands the account can't perform return
// KindHint and are never sent.
func Parse(input string, ch platform.ChannelRef, caps platform.Capabilities) Parsed {
	// Not a command → a plain send. A leading "//" escapes a literal message starting with "/".
	if !strings.HasPrefix(input, "/") {
		return Parsed{Kind: KindSend, Text: input}
	}
	if strings.HasPrefix(input, "//") {
		return Parsed{Kind: KindSend, Text: input[1:]}
	}

	name, rest, _ := strings.Cut(input[1:], " ")
	name = strings.ToLower(name)
	args := splitArgs(rest)

	switch name {
	case "me":
		return Parsed{Kind: KindSend, Text: strings.TrimSpace(rest), IsAction: true}
	case "help":
		return Parsed{Kind: KindHelp}
	}

	sp, ok := commands[name]
	if !ok {
		return Parsed{Kind: KindHint, Hint: "unknown command — see /help"}
	}
	if sp.mod && !caps.Moderation {
		return Parsed{Kind: KindHint, Hint: "/" + name + " needs moderator access on this channel"}
	}
	action, hint := sp.build(ch, args)
	if hint != "" {
		return Parsed{Kind: KindHint, Hint: hint}
	}
	action.Channel = ch
	return Parsed{Kind: KindMod, Action: action}
}

// splitArgs splits on whitespace, dropping empties.
func splitArgs(s string) []string { return strings.Fields(s) }

func buildTarget(t platform.ModActionType) func(platform.ChannelRef, []string) (platform.ModAction, string) {
	return func(_ platform.ChannelRef, args []string) (platform.ModAction, string) {
		if len(args) < 1 {
			return platform.ModAction{}, "usage: a username is required"
		}
		return platform.ModAction{Type: t, TargetUserID: args[0]}, ""
	}
}

func buildBan(_ platform.ChannelRef, args []string) (platform.ModAction, string) {
	if len(args) < 1 {
		return platform.ModAction{}, "usage: /ban <user> [reason]"
	}
	return platform.ModAction{Type: platform.ModBan, TargetUserID: args[0], Reason: strings.Join(args[1:], " ")}, ""
}

func buildTimeout(_ platform.ChannelRef, args []string) (platform.ModAction, string) {
	if len(args) < 1 {
		return platform.ModAction{}, "usage: /timeout <user> [seconds] [reason]"
	}
	a := platform.ModAction{Type: platform.ModTimeout, TargetUserID: args[0], Duration: defaultTimeout}
	if len(args) >= 2 {
		if secs, err := strconv.Atoi(args[1]); err == nil {
			if secs <= 0 {
				return platform.ModAction{}, "usage: /timeout <user> [seconds>0] [reason]"
			}
			a.Duration = time.Duration(secs) * time.Second
			a.Reason = strings.Join(args[2:], " ")
		} else {
			a.Reason = strings.Join(args[1:], " ")
		}
	}
	return a, ""
}

func buildDelete(_ platform.ChannelRef, args []string) (platform.ModAction, string) {
	if len(args) < 1 {
		return platform.ModAction{}, "usage: /delete <message-id>"
	}
	return platform.ModAction{Type: platform.ModDeleteMessage, TargetMessageID: args[0]}, ""
}

func buildSimple(t platform.ModActionType) func(platform.ChannelRef, []string) (platform.ModAction, string) {
	return func(platform.ChannelRef, []string) (platform.ModAction, string) {
		return platform.ModAction{Type: t}, ""
	}
}

func buildToggle(t platform.ModActionType, on bool) func(platform.ChannelRef, []string) (platform.ModAction, string) {
	return func(platform.ChannelRef, []string) (platform.ModAction, string) {
		return platform.ModAction{Type: t, Enabled: on}, ""
	}
}

func buildSlow(_ platform.ChannelRef, args []string) (platform.ModAction, string) {
	secs := 30 // Twitch's default slow interval
	if len(args) >= 1 {
		n, err := strconv.Atoi(args[0])
		if err != nil || n < 0 {
			return platform.ModAction{}, "usage: /slow [seconds>=0]"
		}
		secs = n
	}
	return platform.ModAction{Type: platform.ModSetSlow, Duration: time.Duration(secs) * time.Second, Enabled: secs > 0}, ""
}

func buildFollowers(_ platform.ChannelRef, args []string) (platform.ModAction, string) {
	mins := 0 // any follower
	if len(args) >= 1 {
		n, err := strconv.Atoi(args[0])
		if err != nil || n < 0 {
			return platform.ModAction{}, "usage: /followers [minutes>=0]"
		}
		mins = n
	}
	return platform.ModAction{Type: platform.ModSetFollowers, Duration: time.Duration(mins) * time.Minute, Enabled: true}, ""
}
