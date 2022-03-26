package game

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/murkland/bbn6/mgba"
	"github.com/murkland/ringbuf"
)

type Match struct {
	localReady  bool
	remoteReady bool

	wonLastBattle bool

	stalledFrames int

	battleNumber int
	battle       *Battle
}

type Battle struct {
	tick int32
	isP2 bool

	rw *ReplayWriter

	localInputBuffer *ringbuf.RingBuf[uint16]
	iq               *InputQueue

	pendingRemoteInit []byte

	localPendingTurnWaitTicksLeft int
	localPendingTurn              []byte

	lastCommittedRemoteInput Input

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

func NewBattle(isP2 bool, localInputBufferSize int) (*Battle, error) {
	b := &Battle{
		tick: -1,
		isP2: isP2,

		lastCommittedRemoteInput: Input{Joyflags: 0xfc00},

		iq:               NewInputQueue(60),
		localInputBuffer: ringbuf.New[uint16](localInputBufferSize),
	}

	os.MkdirAll("replays", 0o700)
	fn := filepath.Join("replays", fmt.Sprintf("p%d_%s.bbn6replay", b.LocalPlayerIndex()+1, time.Now().Format("20060102030405")))
	log.Printf("writing replay: %s", fn)

	il, err := newReplayWriter(fn)
	if err != nil {
		return nil, err
	}
	b.rw = il
	return b, nil
}

func (s *Battle) Close() error {
	if err := s.rw.Close(); err != nil {
		return err
	}
	return nil
}
