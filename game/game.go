package game

import (
	"context"
	"errors"
	"fmt"
	"image"
	"log"
	"math/rand"
	"runtime"
	"sync"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/oto/v2"
	"github.com/keegancsmith/nth"
	"github.com/murkland/bbn6/av"
	"github.com/murkland/bbn6/bn6"
	"github.com/murkland/bbn6/config"
	"github.com/murkland/bbn6/mgba"
	"github.com/murkland/bbn6/packets"
	"github.com/murkland/bbn6/trapper"
	"github.com/murkland/ctxwebrtc"
	"github.com/murkland/ringbuf"
	signorclient "github.com/murkland/signor/client"
	"golang.org/x/exp/constraints"
	"golang.org/x/sync/errgroup"
)

type Game struct {
	conf config.Config

	dc             *ctxwebrtc.DataChannel
	randSource     rand.Source
	connectionSide signorclient.ConnectionSide

	mainCore      *mgba.Core
	fastforwarder *fastforwarder

	bn6 *bn6.BN6

	vb *av.VideoBuffer

	fbuf   *image.RGBA
	fbufMu sync.Mutex

	audioPlayer oto.Player

	t *mgba.Thread

	match   *Match
	matchMu sync.Mutex

	debugSpew bool

	delayRingbuf   *ringbuf.RingBuf[time.Duration]
	delayRingbufMu sync.RWMutex
}

func New(conf config.Config, romPath string, dc *ctxwebrtc.DataChannel, randSource rand.Source, connectionSide signorclient.ConnectionSide) (*Game, error) {
	mainCore, err := newCore(romPath)
	if err != nil {
		return nil, err
	}

	mainCore.AutoloadSave()

	bn6 := bn6.Load(mainCore.GameTitle())
	if bn6 == nil {
		return nil, fmt.Errorf("unsupported game: %s", mainCore.GameTitle())
	}

	fastforwarder, err := newFastforwarder(romPath, bn6)
	if err != nil {
		return nil, err
	}

	audioCtx, ready, err := oto.NewContext(mainCore.Options().SampleRate, 2, 2)
	if err != nil {
		return nil, err
	}
	<-ready

	width, height := mainCore.DesiredVideoDimensions()
	vb := av.NewVideoBuffer(width, height)
	ebiten.SetWindowSize(width*3, height*3)

	mainCore.SetVideoBuffer(vb.Pointer(), width)
	t := mgba.NewThread(mainCore)
	if !t.Start() {
		log.Fatalf("failed to start mgba thread")
	}
	mainCore.GBA().Sync().SetFPSTarget(float32(expectedFPS))

	audioPlayer := audioCtx.NewPlayer(av.NewAudioReader(mainCore, mainCore.Options().SampleRate))
	audioPlayer.(oto.BufferSizeSetter).SetBufferSize(mainCore.Options().AudioBuffers * 4)

	g := &Game{
		conf: conf,

		dc:             dc,
		randSource:     randSource,
		connectionSide: connectionSide,

		mainCore:      mainCore,
		fastforwarder: fastforwarder,

		bn6: bn6,

		vb: vb,

		audioPlayer: audioPlayer,

		t: t,

		delayRingbuf: ringbuf.New[time.Duration](10),
	}
	g.InstallTraps(mainCore)

	return g, nil
}

func (g *Game) RunBackgroundTasks(ctx context.Context) error {
	errg, ctx := errgroup.WithContext(ctx)

	errg.Go(func() error {
		return g.handleConn(ctx)
	})

	errg.Go(func() error {
		return g.sendPings(ctx)
	})

	errg.Go(func() error {
		g.serviceFbuf()
		return nil
	})

	return errg.Wait()
}

func (g *Game) serviceFbuf() {
	runtime.LockOSThread()
	for {
		if g.mainCore.GBA().Sync().WaitFrameStart() {
			g.fbufMu.Lock()
			g.fbuf = g.vb.CopyImage()
			g.fbufMu.Unlock()
		} else {
			// TODO: Optimize this.
			time.Sleep(500 * time.Microsecond)
		}
		g.mainCore.GBA().Sync().WaitFrameEnd()
	}
}

func (g *Game) sendPings(ctx context.Context) error {
	for {
		now := time.Now()
		if err := packets.Send(ctx, g.dc, packets.Ping{
			ID: uint64(now.UnixMicro()),
		}, nil); err != nil {
			return err
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(1 * time.Second):
		}
	}
}

type orderableSlice[T constraints.Ordered] []T

func (s orderableSlice[T]) Len() int {
	return len(s)
}

func (s orderableSlice[T]) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

func (s orderableSlice[T]) Less(i, j int) bool {
	return s[i] < s[j]
}

func (g *Game) medianDelay() time.Duration {
	g.delayRingbufMu.RLock()
	defer g.delayRingbufMu.RUnlock()

	if g.delayRingbuf.Used() == 0 {
		return 0
	}

	delays := make([]time.Duration, g.delayRingbuf.Used())
	g.delayRingbuf.Peek(delays, 0)

	i := len(delays) / 2
	nth.Element(orderableSlice[time.Duration](delays), i)
	return delays[i]
}

func (g *Game) handleConn(ctx context.Context) error {
	for {
		packet, trailer, err := packets.Recv(ctx, g.dc)
		if err != nil {
			return err
		}

		switch p := packet.(type) {
		case packets.Ping:
			if err := packets.Send(ctx, g.dc, packets.Pong{ID: p.ID}, nil); err != nil {
				return err
			}
		case packets.Pong:
			if err := (func() error {
				g.delayRingbufMu.Lock()
				defer g.delayRingbufMu.Unlock()

				if g.delayRingbuf.Free() == 0 {
					g.delayRingbuf.Advance(1)
				}

				delay := time.Now().Sub(time.UnixMicro(int64(p.ID)))
				g.delayRingbuf.Push([]time.Duration{delay})
				return nil
			})(); err != nil {
				return err
			}
		case packets.Ready:
			(func() {
				g.matchMu.Lock()
				defer g.matchMu.Unlock()

				if g.match == nil {
					g.match = &Match{}
				}

				g.match.remoteReady = p.IsReady
			})()
		case packets.Init:
			(func() {
				g.matchMu.Lock()
				defer g.matchMu.Unlock()

				g.match.battle.pendingRemoteInit = p.Marshaled[:]
			})()
		case packets.Input:
			if err := (func() error {
				g.matchMu.Lock()
				defer g.matchMu.Unlock()

				g.match.battle.iq.AddInput(g.match.battle.RemotePlayerIndex(), Input{int(p.ForTick), p.Joyflags, p.CustomScreenState, trailer})
				return nil
			})(); err != nil {
				return err
			}
		}
	}
}

func (g *Game) InstallTraps(core *mgba.Core) error {
	tp := trapper.New(core)

	tp.Add(g.bn6.Offsets.ROM.A_battle_init__call__battle_copyInputData, func() {
		g.matchMu.Lock()
		defer g.matchMu.Unlock()

		if g.match == nil {
			return
		}

		core.GBA().SetRegister(0, 0)
		core.GBA().SetRegister(15, core.GBA().Register(15)+4)
		core.GBA().ThumbWritePC()

		if g.match.battle.pendingRemoteInit == nil {
			return
		}

		g.bn6.SetPlayerMarshaledBattleState(core, g.match.battle.RemotePlayerIndex(), g.match.battle.pendingRemoteInit)
		if err := g.match.battle.inputlog.WriteInit(g.match.battle.RemotePlayerIndex(), g.match.battle.pendingRemoteInit); err != nil {
			panic(err)
		}
		g.match.battle.pendingRemoteInit = nil
	})

	tp.Add(g.bn6.Offsets.ROM.A_battle_init_marshal__ret, func() {
		g.matchMu.Lock()
		defer g.matchMu.Unlock()

		if g.match == nil {
			return
		}

		ctx := context.Background()

		marshaled := g.bn6.LocalMarshaledBattleState(core)

		var pkt packets.Init
		copy(pkt.Marshaled[:], marshaled)
		if err := packets.Send(ctx, g.dc, pkt, nil); err != nil {
			panic(err)
		}
		log.Printf("init sent")

		g.bn6.SetPlayerMarshaledBattleState(core, g.match.battle.LocalPlayerIndex(), marshaled)
		if err := g.match.battle.inputlog.WriteInit(g.match.battle.LocalPlayerIndex(), marshaled); err != nil {
			panic(err)
		}
	})

	tp.Add(g.bn6.Offsets.ROM.A_battle_turn_marshal__ret, func() {
		g.matchMu.Lock()
		defer g.matchMu.Unlock()

		if g.match == nil {
			return
		}

		g.match.battle.localPendingTurn = g.bn6.LocalMarshaledBattleState(core)
		g.match.battle.localPendingTurnWaitTicksLeft = 64
	})

	tp.Add(g.bn6.Offsets.ROM.A_battle_update__call__battle_copyInputData, func() {
		g.matchMu.Lock()
		defer g.matchMu.Unlock()

		if g.match == nil {
			return
		}

		core.GBA().SetRegister(0, 0)
		core.GBA().SetRegister(15, core.GBA().Register(15)+4)
		core.GBA().ThumbWritePC()

		ctx := context.Background()
		if g.match.battle.tick == -1 {
			g.match.battle.tick = 0
			g.match.battle.committedState = core.SaveState()
			return
		}

		g.match.battle.tick++

		joyflags := g.bn6.LocalJoyflags(core)
		customScreenState := g.bn6.LocalCustomScreenState(core)

		var turn []byte
		if g.match.battle.localPendingTurnWaitTicksLeft > 0 {
			g.match.battle.localPendingTurnWaitTicksLeft--
			if g.match.battle.localPendingTurnWaitTicksLeft == 0 {
				turn = g.match.battle.localPendingTurn
				g.match.battle.localPendingTurn = nil
			}
		}

		var pkt packets.Input
		pkt.ForTick = uint32(g.match.battle.tick)
		pkt.Joyflags = joyflags
		pkt.CustomScreenState = customScreenState
		if err := packets.Send(ctx, g.dc, pkt, turn); err != nil {
			panic(err)
		}

		g.match.battle.iq.AddInput(g.match.battle.LocalPlayerIndex(), Input{int(g.match.battle.tick), joyflags, customScreenState, turn})
		inputPairs := g.match.battle.iq.Consume()
		if len(inputPairs) > 0 {
			g.match.battle.lastCommittedRemoteInput = inputPairs[len(inputPairs)-1][1-g.match.battle.LocalPlayerIndex()]
		}

		left := g.match.battle.iq.Peek(g.match.battle.LocalPlayerIndex())
		committedState, dirtyState, err := g.fastforwarder.fastforward(g.match.battle.committedState, g.match.battle.inputlog, g.match.battle.LocalPlayerIndex(), inputPairs, g.match.battle.lastCommittedRemoteInput, left)
		if err != nil {
			panic(err)
		}
		g.match.battle.committedState = committedState
		g.mainCore.LoadState(dirtyState)
	})

	tp.Add(g.bn6.Offsets.ROM.A_battle_updating__ret__go_to_custom_screen, func() {
		g.matchMu.Lock()
		defer g.matchMu.Unlock()

		if g.match == nil {
			return
		}

		tick := g.match.battle.tick
		log.Printf("turn ended on %d, rng state = %08x", tick, g.bn6.RNG2State(core))
	})

	tp.Add(g.bn6.Offsets.ROM.A_battle_start__ret, func() {
		g.matchMu.Lock()
		defer g.matchMu.Unlock()

		if g.match == nil {
			return
		}

		if g.match.battle != nil {
			panic("battle already started?")
		}

		g.match.battleNumber++
		log.Printf("battle %d started, is p2 = %t", g.match.battleNumber, g.match.wasLoser)

		battle, err := NewBattle(g.match.wasLoser)
		if err != nil {
			panic(err)
		}
		g.match.battle = battle
	})

	tp.Add(g.bn6.Offsets.ROM.A_battle_end__entry, func() {
		g.matchMu.Lock()
		defer g.matchMu.Unlock()

		if g.match == nil {
			return
		}

		log.Printf("battle ended")
		if err := g.match.battle.Close(); err != nil {
			panic(err)
		}
		g.match.battle = nil

		// TODO: set g.match.wasLoser, somehow.

		g.mainCore.GBA().Sync().SetFPSTarget(float32(expectedFPS))
	})

	tp.Add(g.bn6.Offsets.ROM.A_battle_isP2__tst, func() {
		g.matchMu.Lock()
		defer g.matchMu.Unlock()

		if g.match == nil {
			return
		}

		core.GBA().SetRegister(0, uint32(g.match.battle.LocalPlayerIndex()))
	})

	tp.Add(g.bn6.Offsets.ROM.A_link_isP2__ret, func() {
		g.matchMu.Lock()
		defer g.matchMu.Unlock()

		if g.match == nil {
			return
		}

		core.GBA().SetRegister(0, uint32(g.match.battle.LocalPlayerIndex()))
	})

	tp.Add(g.bn6.Offsets.ROM.A_commMenu_handleLinkCableInput__entry, func() {
		log.Printf("unhandled call to commMenu_handleLinkCableInput at 0x%08x: uh oh!", core.GBA().Register(15)-4)
	})

	tp.Add(g.bn6.Offsets.ROM.A_commMenu_waitForFriend__call__commMenu_handleLinkCableInput, func() {
		g.matchMu.Lock()
		defer g.matchMu.Unlock()

		if g.match == nil {
			g.match = &Match{}
		}

		ctx := context.Background()

		if !g.match.localReady {
			var pkt packets.Ready
			pkt.IsReady = true
			if err := packets.Send(ctx, g.dc, pkt, nil); err != nil {
				panic(err)
			}
			g.match.localReady = true
		}

		if g.match.remoteReady {
			rng := rand.New(g.randSource)
			g.match.wasLoser = (rng.Int31n(2) == 1) == (g.connectionSide == signorclient.ConnectionSideOfferer)
			log.Printf("match started, wasLoser = %t", g.match.wasLoser)
			g.bn6.StartBattleFromCommMenu(core)
		}

		core.GBA().SetRegister(15, core.GBA().Register(15)+4)
		core.GBA().ThumbWritePC()
	})

	tp.Add(g.bn6.Offsets.ROM.A_commMenu_waitForFriend__ret__cancel, func() {
		g.matchMu.Lock()
		defer g.matchMu.Unlock()

		ctx := context.Background()

		if g.match.localReady {
			var pkt packets.Ready
			pkt.IsReady = false
			if err := packets.Send(ctx, g.dc, pkt, nil); err != nil {
				panic(err)
			}
			g.match.localReady = false
		}

		log.Printf("match canceled")
		g.match = nil

		core.GBA().SetRegister(15, core.GBA().Register(15)+4)
		core.GBA().ThumbWritePC()
	})

	tp.Add(g.bn6.Offsets.ROM.A_commMenu_endBattle__entry, func() {
		g.matchMu.Lock()
		defer g.matchMu.Unlock()

		log.Printf("match ended")
		g.match = nil
	})

	tp.Add(g.bn6.Offsets.ROM.A_commMenu_inBattle__call__commMenu_handleLinkCableInput, func() {
		core.GBA().SetRegister(15, core.GBA().Register(15)+4)
		core.GBA().ThumbWritePC()
	})

	core.InstallBeefTrap(tp.BeefHandler)

	return nil
}

func (g *Game) Finish() {
	g.t.End()
	g.t.Join()
}

const expectedFPS = 60

func (g *Game) Update() error {
	if g.t.HasCrashed() {
		return errors.New("mgba thread crashed")
	}

	g.audioPlayer.Play()

	if err := (func() error {
		g.matchMu.Lock()
		defer g.matchMu.Unlock()

		if g.match != nil && g.match.battle != nil {
			expected := int(g.medianDelay()*time.Duration(expectedFPS)/2/time.Second + 1)
			if expected < 1 {
				expected = 1
			}

			lag := g.match.battle.iq.Lag(g.match.battle.RemotePlayerIndex())
			if lag >= expected*2 {
				log.Printf("input is 2x acceptable delay and had to be dropped! %d >= %d", lag, expected*2)
				return nil
			}

			tps := expectedFPS - lag + expected

			// TODO: Not thread safe.
			g.mainCore.GBA().Sync().SetFPSTarget(float32(tps))
		}

		return nil
	})(); err != nil {
		return err
	}

	var keys mgba.Keys
	for _, key := range inpututil.AppendPressedKeys(nil) {
		if key == g.conf.Keymapping.A {
			keys |= mgba.KeysA
		}
		if key == g.conf.Keymapping.B {
			keys |= mgba.KeysB
		}
		if key == g.conf.Keymapping.L {
			keys |= mgba.KeysL
		}
		if key == g.conf.Keymapping.R {
			keys |= mgba.KeysR
		}
		if key == g.conf.Keymapping.Left {
			keys |= mgba.KeysLeft
		}
		if key == g.conf.Keymapping.Right {
			keys |= mgba.KeysRight
		}
		if key == g.conf.Keymapping.Up {
			keys |= mgba.KeysUp
		}
		if key == g.conf.Keymapping.Down {
			keys |= mgba.KeysDown
		}
		if key == g.conf.Keymapping.Start {
			keys |= mgba.KeysStart
		}
		if key == g.conf.Keymapping.Select {
			keys |= mgba.KeysSelect
		}
	}
	g.mainCore.SetKeys(keys)

	if g.conf.Keymapping.DebugSpew != -1 && inpututil.IsKeyJustPressed(g.conf.Keymapping.DebugSpew) {
		g.debugSpew = !g.debugSpew
	}

	return nil
}

func (g *Game) Layout(outsideWidth, outsideHeight int) (screenWidth, screenHeight int) {
	return g.mainCore.DesiredVideoDimensions()
}

func (g *Game) Draw(screen *ebiten.Image) {
	g.fbufMu.Lock()
	defer g.fbufMu.Unlock()

	if g.fbuf == nil {
		return
	}

	opts := &ebiten.DrawImageOptions{}
	screen.DrawImage(ebiten.NewImageFromImage(g.fbuf), opts)

	if g.debugSpew {
		g.spewDebug(screen)
	}
}
