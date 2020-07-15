package rftp

import (
	"container/heap"
	"errors"
	"io"
	"log"
)

type FileResponse struct {
	index uint16

	mc chan *ServerMetaData
	pc chan *ServerPayload
	cc chan struct{}

	preader *io.PipeReader
	pwriter *io.PipeWriter
	buffer  *chunkQueue
	head    uint64

	size     uint64
	chunks   uint64
	checksum [16]byte
	err      error
}

func newFileResponse(index uint16) *FileResponse {
	r, w := io.Pipe()

	return &FileResponse{
		index: index,

		mc: make(chan *ServerMetaData),
		pc: make(chan *ServerPayload, 1024),
		cc: make(chan struct{}),

		preader: r,
		pwriter: w,
		buffer:  newChunkQueue(index),
	}
}

func (f *FileResponse) Read(p []byte) (n int, err error) {
	return f.preader.Read(p)
}

func (f *FileResponse) getResendEntries() []*ResendEntry {
	return f.buffer.Gaps(f.head)
}

func (f *FileResponse) write(done chan<- uint16) {
	log.Printf("start writing file %v\n", f.index)
	defer log.Printf("finished writing file %v\n", f.index)
	for {
		select {
		case metadata := <-f.mc:
			log.Println("fileresponse received metadata")
			f.size = metadata.size
			f.chunks = f.size / 1024
			if f.size%1024 > 0 {
				f.chunks++
			}
			f.checksum = metadata.checkSum

		case payload := <-f.pc:
			log.Printf("fileresponse received payload %v\n", payload.offset)
			if payload.offset == f.head {
				f.pwriter.Write(payload.data)
				f.head++
			} else {
				heap.Push(f.buffer, payload)
			}
			f.drainBuffer()

		case <-f.cc:
			log.Println("abort file writer")
			f.drainBuffer()
			f.err = errors.New("Write canceled")
			f.pwriter.Close()
			return
		}

		log.Printf("file %v at head %v and buffer size %v\n", f.index, f.head, f.buffer.Len())
		if f.head == f.chunks && f.buffer.Len() == 0 {
			done <- f.index
			f.pwriter.Close()
			log.Println("done, leaving writer")
			return
		}
	}
}

func (f *FileResponse) drainBuffer() {
	log.Println("draining buffer")
	defer log.Println("drained buffer")
	top := f.buffer.Top()
	for top == f.head {
		payload := heap.Pop(f.buffer).(*ServerPayload)
		f.pwriter.Write(payload.data)
		top = f.buffer.Top()
		f.head++
	}
}
