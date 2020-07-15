package rftp

import (
	"log"
	"math/rand"
)

type LossSimulator interface {
	shouldDrop() bool
}

type NoopLossSimulator struct{}

func (l *NoopLossSimulator) shouldDrop() bool {
	return false
}

type MarkovLossSimulator struct {
	p         float32
	q         float32
	lossState bool
}

// Return a new loss simulator. p and q between 0 and 1.
// Caller should consider seeding global randomness source.
func NewMarkovLossSimulator(p float32, q float32) LossSimulator {
	if p < 0 || q < 0 || p > 1 || q > 1 {
		log.Panic("The loss simulation parameters must be between 0 and 1")
	}

	return &MarkovLossSimulator{
		p:         p,
		q:         q,
		lossState: false,
	}
}

func (l *MarkovLossSimulator) shouldDrop() bool {
	x := rand.Float32() // upper bound is exclusive, i.e., never 1; problem?
	if l.lossState {
		if x >= 1-l.q {
			l.lossState = false
		}
	} else {
		if x < l.p {
			l.lossState = true
		}
	}

	return l.lossState
}
