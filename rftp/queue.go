package rftp

import (
	"container/heap"
	"fmt"
	"strings"
)

type chunkQueue struct {
	items     []*ServerPayload
	max       uint64 // filesize
	fileIndex uint16
}

func newChunkQueue(fi uint16) *chunkQueue {
	return &chunkQueue{
		items:     make([]*ServerPayload, 0),
		max:       0,
		fileIndex: fi,
	}
}

func (c chunkQueue) String() string {
	offsets := []string{}
	for _, i := range c.items {
		offsets = append(offsets, fmt.Sprintf("%v", i.offset))
	}
	return strings.Join(offsets, ", ")
}

// Len is the number of elements in the collection.
func (c chunkQueue) Len() int {
	return len(c.items)
}

// Less reports whether the element with
// index i should sort before the element with index j.
func (c chunkQueue) Less(i int, j int) bool {
	return c.items[i].offset < c.items[j].offset
}

// Swap swaps the elements with indexes i and j.
func (c chunkQueue) Swap(i int, j int) {
	c.items[i], c.items[j] = c.items[j], c.items[i]
}

func (c *chunkQueue) Push(x interface{}) {
	payload := x.(*ServerPayload)
	c.items = append(c.items, payload)
}

func (c *chunkQueue) Pop() interface{} {
	old := c.items
	n := len(old)
	item := old[n-1]
	old[n-1] = nil
	c.items = old[0 : n-1]
	return item
}

func (c *chunkQueue) Top() uint64 {
	if c.Len() <= 0 {
		return 0
	}
	item := heap.Pop(c)
	defer heap.Push(c, item)
	if item != nil {
		return item.(*ServerPayload).offset
	}
	return 0
}
