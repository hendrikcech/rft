package rftp

import (
	"container/heap"
	"testing"
)

func TestChunkQueue(t *testing.T) {
	items := []*ServerPayload{
		{
			offset: 400,
		},
		{
			offset: 8,
		},
		{
			offset: 7,
		},
		{
			offset: 3,
		},
	}
	q := chunkQueue{
		items: make([]*ServerPayload, len(items)),
	}

	for i, v := range items {
		q.items[i] = v
	}

	//	fmt.Printf("%v\n", q)
	heap.Init(&q)
	//	fmt.Printf("%v\n", q)

	next := &ServerPayload{
		offset: 500,
	}
	items = append([]*ServerPayload{next}, items...)
	heap.Push(&q, next)

	//gaps := q.Gaps(0)
	//fmt.Printf("gaps: %v\n", gaps)

	for q.Len() > 0 {
		item := heap.Pop(&q).(*ServerPayload)
		if items[q.Len()] != item {
			t.Errorf("heap.Pop() = %v, want %v", item, items[q.Len()])
		}
	}
}
