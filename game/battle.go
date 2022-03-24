package game

import (
	"sync"

	"github.com/murkland/bbn6/mgba"
)

type Battle struct {
	mu sync.Mutex

	tick int32
	isP2 bool

	iq *InputQueue

	localPendingTurn []byte

	committedState *mgba.State
}

func (s *Battle) LocalPlayerIndex() int {
	if s.isP2 {
		return 1
	}
	return 0
}

func (s *Battle) RemotePlayerIndex() int {
	return 1 - s.LocalPlayerIndex()
}

func NewBattle(isP2 bool) *Battle {
	return &Battle{
		tick: -1,
		isP2: isP2,

		iq: NewInputQueue(60),
	}
}
