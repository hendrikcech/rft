package rftp

import (
	"bytes"
	"container/heap"
	"crypto/md5"
	"errors"
	"fmt"
	"hash"
	"io"
	"log"
	"sort"
	"sync"
)

type FileResponse struct {
	index uint16

	mc chan *ServerMetaData
	pc chan *ServerPayload
	cc chan struct{}

	preader       *io.PipeReader
	pwriter       *io.PipeWriter
	buffer        *chunkQueue
	resendEntries map[uint64]struct{}
	head          uint64
	metadata      bool
	lock          sync.Mutex
	hasher        hash.Hash

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

		preader:       r,
		pwriter:       w,
		buffer:        newChunkQueue(index),
		resendEntries: make(map[uint64]struct{}),
		hasher:        md5.New(),
	}
}

func (f *FileResponse) Read(p []byte) (n int, err error) {
	n, err = f.preader.Read(p)
	if err != nil {
		return
	}
	_, err = f.hasher.Write(p[:n])
	if err != nil {
		return
	}
	return
}

// Returns true iff the checksum of the received file matches the checksum
// transmitted in the metadata message. Returns false while the file is still in
// transit.
func (f *FileResponse) ChecksumValid() bool {
	return bytes.Compare(f.checksum[:], f.hasher.Sum(nil)[:16]) == 0
}

type resendData struct {
	started    bool
	metadata   bool
	head       uint64
	res        []*ResendEntry
	bufferSize int
}

func (f *FileResponse) getResendEntries(max int) *resendData {
	f.lock.Lock()
	defer f.lock.Unlock()
	res := []*ResendEntry{}
	// TODO: make sort faster? keep it sorted? Check loss of precision when
	// converting uint64 to int?
	entries := make([]int, len(f.resendEntries))
	i := 0
	for k := range f.resendEntries {
		entries[i] = int(k)
		i++
	}
	sort.Ints(entries)
	for _, offset := range entries {
		if len(res) > max {
			break
		}
		res = append(res, &ResendEntry{
			fileIndex: f.index,
			offset:    uint64(offset),
			length:    1,
		})
	}

	if !f.metadata {
		res = append(res, &ResendEntry{
			fileIndex: f.index,
			offset:    f.head,
			length:    0,
		})
	} else if f.head < f.chunks {
		res = append(res, &ResendEntry{
			fileIndex: f.index,
			offset:    f.head,
			length:    1,
		})
	}
	return &resendData{
		started:    (f.head > 0) || f.buffer.Len() > 0,
		metadata:   f.metadata,
		head:       f.head,
		res:        res,
		bufferSize: cap(f.pc) - len(f.pc),
	}
}

func (f *FileResponse) write(done chan<- uint16) {
	log.Printf("Start processing file %v\n", f.index)
	defer func() {
		done <- f.index
		f.pwriter.Close()
		log.Printf("Finished processing file %v\n", f.index)
	}()
	for {
		select {
		case metadata := <-f.mc:
			f.lock.Lock()
			log.Printf("metadata: %+v\n", metadata)
			if metadata.status != noErr {
				f.err = fmt.Errorf("Server returned error for file %d: status %s",
					f.index, metadata.status.String())
				return
			}
			f.size = metadata.size
			f.chunks = f.size / 1024
			if f.size%1024 > 0 {
				f.chunks++
			}
			log.Printf("fileresponse received metadata: size: %v\n", f.chunks)
			f.checksum = metadata.checkSum
			f.metadata = true
			f.lock.Unlock()

		case payload := <-f.pc:
			log.Printf("fileresponse received payload %v\n", payload.offset)
			if payload.offset == f.head {
				f.pwriter.Write(payload.data)
				f.lock.Lock()
				delete(f.resendEntries, f.head)
				f.head++
				f.lock.Unlock()
			} else {
				if payload.offset > f.head {
					f.lock.Lock()
					heap.Push(f.buffer, payload)
					for i := f.head; i < payload.offset; i++ {
						f.resendEntries[i] = struct{}{}
					}
					f.lock.Unlock()
				}
			}
			f.drainBuffer()

		case <-f.cc:
			f.drainBuffer()
			f.err = fmt.Errorf("Write canceled")
			return
		}

		log.Printf("file %v at head %v and buffer size %v\n", f.index, f.head, f.buffer.Len())
		if f.metadata && f.head >= f.chunks && f.buffer.Len() == 0 {
			return
		}
	}
}

func (f *FileResponse) drainBuffer() {
	f.lock.Lock()
	defer f.lock.Unlock()
	top := f.buffer.Top()
	log.Printf("buffer top: %v, head: %v\n", top, f.head)
	for top <= f.head && f.buffer.Len() > 0 {
		payload := heap.Pop(f.buffer).(*ServerPayload)
		if top == f.head {
			f.pwriter.Write(payload.data)
			delete(f.resendEntries, f.head)
			f.head++
		}
		top = f.buffer.Top()
	}
}
