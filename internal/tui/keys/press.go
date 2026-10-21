package keys

import tea "charm.land/bubbletea/v2"

// lockMods are lock-key states the Windows console and the kitty protocol
// report alongside a key. They never change what the key means.
const lockMods = tea.ModCapsLock | tea.ModNumLock | tea.ModScrollLock

// IsEnter reports an Enter press whatever the modifiers. Prompts have always
// taken alt+enter as Enter, and terminals without the kitty keyboard
// protocol send shift+enter and ctrl+enter as a bare carriage return.
func IsEnter(msg tea.KeyPressMsg) bool {
	return msg.Code == tea.KeyEnter || msg.Code == tea.KeyKpEnter
}

// IsEsc reports an Esc press whatever the modifiers, alt+esc included.
func IsEsc(msg tea.KeyPressMsg) bool {
	return msg.Code == tea.KeyEscape
}

// IsArrow reports the arrow key code pressed alone or with Alt. Shift and
// ctrl arrows are different keys and do not count.
func IsArrow(msg tea.KeyPressMsg, code rune) bool {
	return msg.Code == code && msg.Mod&^(lockMods|tea.ModAlt) == 0
}

// IsForceQuit reports ctrl+c, also when Alt or Shift is held as well.
func IsForceQuit(msg tea.KeyPressMsg) bool {
	return msg.Code == 'c' && msg.Mod.Contains(tea.ModCtrl)
}

// Normalize turns shift- or ctrl-modified Enter and Esc into the bare keys.
// Terminals with the kitty keyboard protocol report those combinations as
// distinct keys, while v1 terminals sent a plain carriage return or escape,
// so every binding written for "enter" or "esc" keeps working. alt+enter
// stays distinct, as it always was. Any other message passes through.
func Normalize(msg tea.Msg) tea.Msg {
	k, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return msg
	}
	if k.Code != tea.KeyEnter && k.Code != tea.KeyKpEnter && k.Code != tea.KeyEscape {
		return msg
	}
	if k.Mod&^(lockMods|tea.ModShift|tea.ModCtrl) != 0 {
		return msg
	}
	code := k.Code
	if code == tea.KeyKpEnter {
		code = tea.KeyEnter
	}
	return tea.KeyPressMsg{Code: code}
}
