package game

import (
	"github.com/murkland/ringbuf"
)

type Input struct {
	Tick              int
	Joyflags          uint16
	CustomScreenState uint8
	Turn              []byte
}

type InputQueue struct {
	qs [2]*ringbuf.RingBuf[Input]
}

func NewInputQueue(n int) *InputQueue {
	return &InputQueue{[2]*ringbuf.RingBuf[Input]{
		ringbuf.New[Input](n),
		ringbuf.New[Input](n),
	}}
}

func (q *InputQueue) AddInput(playerIndex int, input Input) {
	q.qs[playerIndex].Push([]Input{input})
}

func (q *InputQueue) Peek(playerIndex int) []Input {
	n := q.qs[playerIndex].Used()
	inputs := make([]Input, n)
	q.qs[playerIndex].Peek(inputs, 0)
	return inputs
}

func (q *InputQueue) Lag(playerIndex int) int {
	return q.qs[1-playerIndex].Used() - q.qs[playerIndex].Used()
}

func (q *InputQueue) Consume() [][2]Input {
	n := q.qs[0].Used()
	if q.qs[1].Used() < n {
		n = q.qs[1].Used()
	}

	p1Inputs := make([]Input, n)
	q.qs[0].Pop(p1Inputs, 0)

	p2Inputs := make([]Input, n)
	q.qs[1].Pop(p2Inputs, 0)

	inputPairs := make([][2]Input, n)
	for i := 0; i < n; i++ {
		inputPairs[i] = [2]Input{p1Inputs[i], p2Inputs[i]}
	}

	return inputPairs
}
