package rftp

import (
	"container/heap"
	"testing"
)

func TestChunkQueue(t *testing.T) {
	items := []*serverPayload{
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
		items: make([]*serverPayload, len(items)),
	}

	for i, v := range items {
		q.items[i] = v
	}

	//	fmt.Printf("%v\n", q)
	heap.Init(&q)
	//	fmt.Printf("%v\n", q)

	next := &serverPayload{
		offset: 500,
	}
	items = append([]*serverPayload{next}, items...)
	heap.Push(&q, next)

	//gaps := q.Gaps(0)
	//fmt.Printf("gaps: %v\n", gaps)

	for q.Len() > 0 {
		item := heap.Pop(&q).(*serverPayload)
		if items[q.Len()] != item {
			t.Errorf("heap.Pop() = %v, want %v", item, items[q.Len()])
		}
	}
}
