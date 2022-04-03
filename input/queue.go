package input

import (
	"context"
	"sync"

	"github.com/murkland/ringbuf"
)

type Queue struct {
	mu   sync.Mutex
	cond *sync.Cond

	qs         [2]*ringbuf.RingBuf[Input]
	consumable [][2]Input
	delay      int
}

func NewQueue(n int, delay int) *Queue {
	iq := &Queue{
		qs: [2]*ringbuf.RingBuf[Input]{
			ringbuf.New[Input](n),
			ringbuf.New[Input](n),
		},
		delay: delay,
	}
	iq.cond = sync.NewCond(&iq.mu)
	return iq
}

func (q *Queue) AddInput(ctx context.Context, playerIndex int, input Input) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var ctxErr error
	go func() {
		<-ctx.Done()
		ctxErr = ctx.Err()
		q.cond.Broadcast()
	}()

	for q.qs[playerIndex].Free() == 0 && ctxErr == nil {
		q.cond.Wait()
	}
	if ctxErr != nil {
		return ctxErr
	}

	q.qs[playerIndex].Push([]Input{input})
	q.consumable = append(q.consumable, q.advanceManyLocked()...)
	q.cond.Broadcast()
	return nil
}

func (q *Queue) Peek(playerIndex int) []Input {
	q.mu.Lock()
	defer q.mu.Unlock()

	n := q.qs[playerIndex].Used() - q.delay
	if n < 0 {
		return nil
	}
	inputs := make([]Input, n)
	q.qs[playerIndex].Peek(inputs, 0)
	return inputs
}

func (q *Queue) QueueLength(playerIndex int) int {
	q.mu.Lock()
	defer q.mu.Unlock()

	return q.qs[playerIndex].Used()
}

func (q *Queue) advanceManyLocked() [][2]Input {
	n := q.qs[0].Used() - q.delay
	if q.qs[1].Used()-q.delay < n {
		n = q.qs[1].Used() - q.delay
	}

	if n < 0 {
		return nil
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

func (q *Queue) Consume() [][2]Input {
	q.mu.Lock()
	defer q.mu.Unlock()

	consumable := q.consumable
	q.consumable = nil
	return consumable
}
