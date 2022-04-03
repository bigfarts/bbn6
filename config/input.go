package config

import (
	"fmt"

	"github.com/hajimehoshi/ebiten/v2"
)

var keyCodeToKeyName map[ebiten.Key]string
var keyNameToKeyCode map[string]ebiten.Key

func init() {
	keyCodeToKeyName = map[ebiten.Key]string{
		ebiten.KeyA:              "a",
		ebiten.KeyB:              "b",
		ebiten.KeyC:              "c",
		ebiten.KeyD:              "d",
		ebiten.KeyE:              "e",
		ebiten.KeyF:              "f",
		ebiten.KeyG:              "g",
		ebiten.KeyH:              "h",
		ebiten.KeyI:              "i",
		ebiten.KeyJ:              "j",
		ebiten.KeyK:              "k",
		ebiten.KeyL:              "l",
		ebiten.KeyM:              "m",
		ebiten.KeyN:              "n",
		ebiten.KeyO:              "o",
		ebiten.KeyP:              "p",
		ebiten.KeyQ:              "q",
		ebiten.KeyR:              "r",
		ebiten.KeyS:              "s",
		ebiten.KeyT:              "t",
		ebiten.KeyU:              "u",
		ebiten.KeyV:              "v",
		ebiten.KeyW:              "w",
		ebiten.KeyX:              "x",
		ebiten.KeyY:              "y",
		ebiten.KeyZ:              "z",
		ebiten.KeyAlt:            "alt",
		ebiten.KeyAltLeft:        "altleft",
		ebiten.KeyAltRight:       "altright",
		ebiten.KeyArrowDown:      "arrowdown",
		ebiten.KeyArrowLeft:      "arrowleft",
		ebiten.KeyArrowRight:     "arrowright",
		ebiten.KeyArrowUp:        "arrowup",
		ebiten.KeyBackquote:      "backquote",
		ebiten.KeyBackslash:      "backslash",
		ebiten.KeyBackspace:      "backspace",
		ebiten.KeyBracketLeft:    "bracketleft",
		ebiten.KeyBracketRight:   "bracketright",
		ebiten.KeyCapsLock:       "capslock",
		ebiten.KeyComma:          "comma",
		ebiten.KeyContextMenu:    "contextmenu",
		ebiten.KeyControl:        "control",
		ebiten.KeyControlLeft:    "controlleft",
		ebiten.KeyControlRight:   "controlright",
		ebiten.KeyDelete:         "delete",
		ebiten.KeyDigit0:         "digit0",
		ebiten.KeyDigit1:         "digit1",
		ebiten.KeyDigit2:         "digit2",
		ebiten.KeyDigit3:         "digit3",
		ebiten.KeyDigit4:         "digit4",
		ebiten.KeyDigit5:         "digit5",
		ebiten.KeyDigit6:         "digit6",
		ebiten.KeyDigit7:         "digit7",
		ebiten.KeyDigit8:         "digit8",
		ebiten.KeyDigit9:         "digit9",
		ebiten.KeyEnd:            "end",
		ebiten.KeyEnter:          "enter",
		ebiten.KeyEqual:          "equal",
		ebiten.KeyEscape:         "escape",
		ebiten.KeyF1:             "f1",
		ebiten.KeyF2:             "f2",
		ebiten.KeyF3:             "f3",
		ebiten.KeyF4:             "f4",
		ebiten.KeyF5:             "f5",
		ebiten.KeyF6:             "f6",
		ebiten.KeyF7:             "f7",
		ebiten.KeyF8:             "f8",
		ebiten.KeyF9:             "f9",
		ebiten.KeyF10:            "f10",
		ebiten.KeyF11:            "f11",
		ebiten.KeyF12:            "f12",
		ebiten.KeyHome:           "home",
		ebiten.KeyInsert:         "insert",
		ebiten.KeyMeta:           "meta",
		ebiten.KeyMetaLeft:       "metaleft",
		ebiten.KeyMetaRight:      "metaright",
		ebiten.KeyMinus:          "minus",
		ebiten.KeyNumLock:        "numlock",
		ebiten.KeyNumpad0:        "numpad0",
		ebiten.KeyNumpad1:        "numpad1",
		ebiten.KeyNumpad2:        "numpad2",
		ebiten.KeyNumpad3:        "numpad3",
		ebiten.KeyNumpad4:        "numpad4",
		ebiten.KeyNumpad5:        "numpad5",
		ebiten.KeyNumpad6:        "numpad6",
		ebiten.KeyNumpad7:        "numpad7",
		ebiten.KeyNumpad8:        "numpad8",
		ebiten.KeyNumpad9:        "numpad9",
		ebiten.KeyNumpadAdd:      "numpadadd",
		ebiten.KeyNumpadDecimal:  "numpaddecimal",
		ebiten.KeyNumpadDivide:   "numpaddivide",
		ebiten.KeyNumpadEnter:    "numpadenter",
		ebiten.KeyNumpadEqual:    "numpadequal",
		ebiten.KeyNumpadMultiply: "numpadmultiply",
		ebiten.KeyNumpadSubtract: "numpadsubtract",
		ebiten.KeyPageDown:       "pagedown",
		ebiten.KeyPageUp:         "pageup",
		ebiten.KeyPause:          "pause",
		ebiten.KeyPeriod:         "period",
		ebiten.KeyPrintScreen:    "printscreen",
		ebiten.KeyQuote:          "quote",
		ebiten.KeyScrollLock:     "scrolllock",
		ebiten.KeySemicolon:      "semicolon",
		ebiten.KeyShift:          "shift",
		ebiten.KeyShiftLeft:      "shiftleft",
		ebiten.KeyShiftRight:     "shiftright",
		ebiten.KeySlash:          "slash",
		ebiten.KeySpace:          "space",
		ebiten.KeyTab:            "tab",
	}

	keyNameToKeyCode = make(map[string]ebiten.Key, len(keyCodeToKeyName))
	for k, v := range keyCodeToKeyName {
		keyNameToKeyCode[v] = k
	}
}

type Key ebiten.Key

func (k *Key) UnmarshalText(text []byte) error {
	key, ok := keyNameToKeyCode[string(text)]
	if !ok {
		return fmt.Errorf("unknown key: %s", string(text))
	}
	*k = Key(key)
	return nil
}

func (k Key) MarshalText() ([]byte, error) {
	name, ok := keyCodeToKeyName[ebiten.Key(k)]
	if !ok {
		return nil, fmt.Errorf("unknown key: %v", k)
	}
	return []byte(name), nil
}
