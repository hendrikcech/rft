package rftp

import (
	"log"
	"math/rand"
)

const (
	state_lost = iota
	state_rcvd
)

type LossSimulator struct {
	p     float32
	q     float32
	state int
}

// Return a new loss simulator. p and q between 0 and 1.
// Caller should consider seeding global randomness source.
func NewLossSimulator(p float32, q float32) *LossSimulator {
	if p < 0 || q < 0 || p > 1 || q > 1 {
		log.Panic("The loss simulation parameters must be between 0 and 1")
	}

	return &LossSimulator{
		p:     p,
		q:     q,
		state: state_rcvd,
	}
}

func (l *LossSimulator) shouldDrop() bool {
	x := rand.Float32() // upper bound is exclusive, i.e., never 1; problem?
	switch l.state {
	case state_lost:
		if x >= 1-l.q {
			l.state = state_rcvd
		}
	case state_rcvd:
		if x < l.p {
			l.state = state_lost
		}
	default:
		log.Panicf("Undefined loss state: %d", l.state)
	}

	return l.state == state_lost
}
