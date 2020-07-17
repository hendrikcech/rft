package rftp

import (
	"sync/atomic"
	"time"
)

type RateControl interface {
	// Must be called before any other function of RateControl.
	start()
	stop()

	// Returns true if both congestion and flow control allow sending one packet
	// at this moment.
	isAvailable() bool

	// Element added each time the awaitAvailable rate changes.
	awaitAvailable() <-chan struct{}

	// Must be called with a newly received client acknowledgment.
	onAck(*ClientAck)

	// Must be called once for each packet that is sent on a connection.
	onSend()
}

const (
	// The received ACK was sent before the resent packets had a chance to arrive
	// at the client. Minimal time needed: 1 RTT. Give it a bit of room to account
	// for the processing delay etc.
	aimdDecreaseCoolOffPeriod = 6 // unit in number of ACKs. 6 acks = 1.5 RTTs
)

type aimd struct {
	congRate              uint32
	flowRate              uint32
	sent                  uint32
	lastAck               uint8
	decreaseCoolOffPeriod uint8

	resetTicker   *time.Ticker
	availableChan chan struct{}
}

func (c *aimd) start() {
	c.resetTicker = time.NewTicker(1 * time.Second)
	go func() {
		for {
			atomic.StoreUint32(&c.sent, 0)
			c.notifyAvailable()
			_, ok := <-c.resetTicker.C
			if !ok {
				break
			}
		}
	}()

	c.availableChan = make(chan struct{}, 1)
}

func (c *aimd) stop() {
	c.resetTicker.Stop()
}

func (c *aimd) awaitAvailable() <-chan struct{} {
	return c.availableChan
}

func (c *aimd) notifyAvailable() {
	// If last notification (value of c.availableChan) has not been read, a write
	// would block.
	if len(c.availableChan) < cap(c.availableChan) {
		c.availableChan <- struct{}{}
	}
}

// Returns true if both congestion and flow control allow sending one packet
// at this moment.
func (c *aimd) isAvailable() bool {
	sent := atomic.LoadUint32(&c.sent)
	return sent < c.congRate && sent < c.flowRate
}

func (c *aimd) onACK(ack *ClientAck) {
	if ack.ackNumber < c.lastAck {
		// Should we make sure that out-of-order ACKs are handled earlier?
		return
	}

	if c.decreaseCoolOffPeriod > 0 {
		diff := ack.ackNumber - c.lastAck
		if diff > c.decreaseCoolOffPeriod {
			c.decreaseCoolOffPeriod = 0
		} else {
			c.decreaseCoolOffPeriod -= diff
		}
	}

	c.flowRate = ack.maxTransmissionRate

	if len(ack.resendEntries) == 0 {
		c.congRate++
	} else if c.decreaseCoolOffPeriod == 0 {
		c.congRate /= 2
		c.decreaseCoolOffPeriod = aimdDecreaseCoolOffPeriod
	}

	c.lastAck = ack.ackNumber
	if c.isAvailable() {
		c.notifyAvailable()
	}
}

func (c *aimd) onSend() {
	atomic.AddUint32(&c.sent, 1)
}
