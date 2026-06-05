package twitch

import "strings"

// ircMessage is a parsed IRCv3 line: optional tags, an optional prefix (source), the command,
// the middle parameters, and the trailing parameter (the ":"-prefixed remainder) kept
// separate so a command with no message — e.g. "USERNOTICE #chan" — doesn't mistake its
// channel for a message body.
type ircMessage struct {
	tags    map[string]string
	prefix  string
	command string
	params  []string // middle parameters only (e.g. the channel)
	tr      string   // trailing parameter text ("" if none)
}

// nick returns the sender's login from the prefix ("nick!user@host" → "nick"), or "".
func (m ircMessage) nick() string {
	if m.prefix == "" {
		return ""
	}
	if i := strings.IndexByte(m.prefix, '!'); i >= 0 {
		return m.prefix[:i]
	}
	return m.prefix
}

// trailing returns the trailing parameter — the message body for PRIVMSG/USERNOTICE — or ""
// when the line had none.
func (m ircMessage) trailing() string { return m.tr }

// parseLine parses one IRCv3 line (without the trailing CRLF). It returns false only for an
// empty line. Grammar: [ '@' tags SPACE ] [ ':' prefix SPACE ] command *( SPACE param )
// where a param beginning with ':' is the trailing param and runs to end of line.
func parseLine(line string) (ircMessage, bool) {
	line = strings.TrimRight(line, "\r\n")
	if line == "" {
		return ircMessage{}, false
	}
	var msg ircMessage

	// Tags.
	if line[0] == '@' {
		end := strings.IndexByte(line, ' ')
		if end < 0 {
			return ircMessage{}, false
		}
		msg.tags = parseTags(line[1:end])
		line = line[end+1:]
	}

	// Prefix (source).
	if len(line) > 0 && line[0] == ':' {
		end := strings.IndexByte(line, ' ')
		if end < 0 {
			return ircMessage{}, false
		}
		msg.prefix = line[1:end]
		line = line[end+1:]
	}

	// Command.
	if sp := strings.IndexByte(line, ' '); sp >= 0 {
		msg.command = line[:sp]
		line = line[sp+1:]
	} else {
		msg.command = line
		return msg, msg.command != ""
	}

	// Parameters: middle params, then an optional ":"-prefixed trailing param.
	for len(line) > 0 {
		if line[0] == ':' {
			msg.tr = line[1:]
			break
		}
		sp := strings.IndexByte(line, ' ')
		if sp < 0 {
			msg.params = append(msg.params, line)
			break
		}
		if sp > 0 {
			msg.params = append(msg.params, line[:sp])
		}
		line = line[sp+1:]
	}
	return msg, true
}

// parseTags parses the "key=value;key2=value2" tag string into a map, decoding the IRCv3
// value escapes. A key with no '=' maps to the empty string.
func parseTags(s string) map[string]string {
	tags := make(map[string]string)
	for _, pair := range strings.Split(s, ";") {
		if pair == "" {
			continue
		}
		if eq := strings.IndexByte(pair, '='); eq >= 0 {
			tags[pair[:eq]] = unescapeTagValue(pair[eq+1:])
		} else {
			tags[pair] = ""
		}
	}
	return tags
}

// unescapeTagValue decodes the IRCv3 tag-value escapes: \: → ';', \s → space, \\ → '\',
// \r → CR, \n → LF. A trailing lone backslash is dropped.
func unescapeTagValue(v string) string {
	if !strings.ContainsRune(v, '\\') {
		return v
	}
	var b strings.Builder
	b.Grow(len(v))
	for i := 0; i < len(v); i++ {
		if v[i] != '\\' {
			b.WriteByte(v[i])
			continue
		}
		if i+1 >= len(v) {
			break // trailing backslash dropped
		}
		i++
		switch v[i] {
		case ':':
			b.WriteByte(';')
		case 's':
			b.WriteByte(' ')
		case '\\':
			b.WriteByte('\\')
		case 'r':
			b.WriteByte('\r')
		case 'n':
			b.WriteByte('\n')
		default:
			b.WriteByte(v[i])
		}
	}
	return b.String()
}
