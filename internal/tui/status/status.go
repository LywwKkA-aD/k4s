// Package status is the footer's status row: at most one message, with a
// level and a time to live.
package status

import (
	"strings"
	"time"
	"unicode"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/LywwKkA-aD/k4s/internal/kubectl"
)

// Level ranks a status message; the footer colours it by level.
type Level int

const (
	// Info is neutral progress, such as "resolving logs for deployment/api".
	Info Level = iota
	// OK reports an action that worked.
	OK
	// Warn reports something the user should look at.
	Warn
	// Error reports a failed action. It stays up until a key is pressed,
	// however long that takes, but never for less than TTL.
	Error
	// Alert is bad news nobody asked for, such as a pod a poll found
	// failing: shown as an error, but gone at TTL like Warn, since no
	// key of the user's is waiting on it.
	Alert
)

// TTL is how long a message stays up.
const TTL = 5 * time.Second

// Message is one status row.
type Message struct {
	Level Level
	Text  string
}

// ExpireMsg is delivered TTL after a message was set; hand it to
// Line.Expire.
type ExpireMsg struct{ seq uint64 }

// Line holds the current status message. The zero value shows nothing.
// Methods return a new Line and leave the receiver as it was.
type Line struct {
	msg Message
	// cmd is the command msg is about (see SetCommand); zero for most.
	cmd   kubectl.Cmd
	shown bool
	since time.Time
	// seq tells the expiry of this message from that of one it replaced.
	seq uint64
	// keys counts the key presses since msg was set, up to 2: enough to
	// tell the press that dismissed an error from the ones after it.
	keys int
}

// Set shows text at level from now on, replacing any message, and returns
// the Cmd that delivers its ExpireMsg. The text is folded onto one line,
// stripped of escape sequences, and $HOME in it is shortened to ~.
func (l Line) Set(level Level, text string, now time.Time) (Line, tea.Cmd) {
	seq := l.seq + 1
	next := Line{
		msg:   Message{Level: level, Text: clean(text)},
		shown: true,
		since: now,
		seq:   seq,
	}
	return next, tea.Tick(TTL, func(time.Time) tea.Msg { return ExpireMsg{seq: seq} })
}

// Clear drops the message.
func (l Line) Clear() Line {
	return Line{seq: l.seq}
}

// KeyPressed records a key press: an error already past its TTL goes, one
// still inside it goes when the TTL runs out.
func (l Line) KeyPressed() Line {
	l.keys = min(l.keys+1, 2)
	return l
}

// Current returns the message showing at now, if any.
func (l Line) Current(now time.Time) (Message, bool) {
	if !l.showing(now) {
		return Message{}, false
	}
	return l.msg, true
}

func (l Line) showing(now time.Time) bool {
	if !l.shown {
		return false
	}
	return now.Sub(l.since) < TTL || (l.msg.Level == Error && l.keys == 0)
}

// Expire drops the message if it has stopped showing at now. The expiry of
// a message that has since been replaced or cleared changes nothing.
func (l Line) Expire(msg ExpireMsg, now time.Time) Line {
	if msg.seq != l.seq {
		return l
	}
	if _, showing := l.Current(now); showing {
		return l
	}
	return l.Clear()
}

// clean makes text safe for one footer row.
func clean(text string) string {
	text = redact(ansi.Strip(text), userHome())
	text = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, text)
	return strings.Join(strings.Fields(text), " ")
}
