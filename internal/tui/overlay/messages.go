package overlay

import "github.com/LywwKkA-aD/k4s/internal/tui/views"

// CommandMsg is what was typed into the command bar, sent on Enter.
type CommandMsg struct {
	Input string
}

// TailChosenMsg is the line count entered in the tail prompt for Request.
type TailChosenMsg struct {
	Request views.TailPromptRequestMsg
	Tail    int64
}

// ContainerChosenMsg is the container picked for Request.
type ContainerChosenMsg struct {
	Request   views.ContainerPromptRequestMsg
	Container string
}

// ForwardChosenMsg is the port pair entered for Request. The local port
// could be bound when the prompt closed.
type ForwardChosenMsg struct {
	Request views.ForwardRequestMsg
	Local   uint16
	Remote  uint16
}

// RestoreDecisionMsg answers the startup question about saved forwards.
type RestoreDecisionMsg struct {
	Revive bool
}
