package rftp

import (
	"sync"
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
	onAck(*clientAck)

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

	resetTicker         *time.Ticker
	closedTicker        chan struct{}
	availableChan       chan struct{}
	notifyAvailableLock sync.Mutex
}

var _ RateControl = (*aimd)(nil)

func (c *aimd) start() {
	c.resetTicker = time.NewTicker(1 * time.Second)
	c.closedTicker = make(chan struct{}, 1)
	c.availableChan = make(chan struct{}, 1)
	c.notifyAvailableLock = sync.Mutex{}

	go func() {
		for {
			atomic.StoreUint32(&c.sent, 0)
			c.notifyAvailable()
			select {
			case <-c.resetTicker.C:
			case <-c.closedTicker:
				return
			}
		}
	}()
}

func (c *aimd) stop() {
	c.resetTicker.Stop() // does not close resetTicker.C
	c.closedTicker <- struct{}{}
}

func (c *aimd) awaitAvailable() <-chan struct{} {
	return c.availableChan
}

func (c *aimd) notifyAvailable() {
	// If last notification (value of c.availableChan) has not been read, a write
	// would block.
	c.notifyAvailableLock.Lock()
	if len(c.availableChan) < cap(c.availableChan) {
		c.availableChan <- struct{}{}
	}
	c.notifyAvailableLock.Unlock()
}

// Returns true if both congestion and flow control allow sending one packet
// at this moment.
func (c *aimd) isAvailable() bool {
	sent := atomic.LoadUint32(&c.sent)
	//	log.Printf("isAvailable: sent: %v, c.congRate: %v, c.flowRate: %v\n", sent, c.congRate, c.flowRate)
	if c.flowRate > 0 {
		return sent < c.congRate && sent < c.flowRate
	}
	return sent < c.congRate
}

func (c *aimd) onAck(ack *clientAck) {
	if ack.ackNumber < c.lastAck {
		// Should we make sure that out-of-order ACKs are handled earlier?
		c.lastAck = ack.ackNumber
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

	if len(ack.resendEntries) > 0 {
		c.congRate += c.congRate / 2
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
