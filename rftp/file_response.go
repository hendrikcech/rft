package rftp

import (
	"bytes"
	"container/heap"
	"crypto/md5"
	"fmt"
	"hash"
	"io"
	"log"
	"sort"
	"sync"
)

func print(done, queue, total uint64) {
	fmt.Printf("\r%v/%v/%v", done, queue, total)
}

type FileResponse struct {
	index uint16
	Name  string

	mc chan *ServerMetaData
	pc chan *ServerPayload
	cc chan struct{}

	preader       *io.PipeReader
	pwriter       *io.PipeWriter
	buffer        *chunkQueue
	maxBufferSize int
	resendEntries map[uint64]struct{}
	outOfOrder    map[uint64]struct{}
	head          uint64
	metadata      bool
	lock          sync.Mutex
	hasher        hash.Hash

	size     uint64
	chunks   uint64
	checksum [16]byte
	Err      error
}

func newFileResponse(name string, index uint16) *FileResponse {
	r, w := io.Pipe()

	return &FileResponse{
		index: index,
		Name:  name,

		mc: make(chan *ServerMetaData),
		pc: make(chan *ServerPayload, 1024*1024),
		cc: make(chan struct{}),

		preader:       r,
		pwriter:       w,
		buffer:        newChunkQueue(index),
		maxBufferSize: 10 * 1024,
		resendEntries: make(map[uint64]struct{}),
		hasher:        md5.New(),

		outOfOrder: make(map[uint64]struct{}),
	}
}

func (f *FileResponse) Read(p []byte) (n int, err error) {
	n, readErr := f.preader.Read(p)
	_, hashErr := f.hasher.Write(p[:n])
	if readErr == io.EOF {
		if !bytes.Equal(f.checksum[:], f.hasher.Sum(nil)[:16]) {
			f.lock.Lock()
			if f.Err == nil {
				f.Err = fmt.Errorf("Checksum validation failed")
			}
			f.lock.Unlock()
		}
	}
	if readErr != nil {
		err = readErr
	} else if hashErr != nil {
		err = hashErr
	}
	return
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
		if _, ok := f.outOfOrder[uint64(offset)]; !ok {
			res = append(res, &ResendEntry{
				fileIndex: f.index,
				offset:    uint64(offset),
				length:    1,
			})
		}
	}

	if !f.metadata {
		res = append(res, &ResendEntry{
			fileIndex: f.index,
			offset:    f.head,
			length:    0,
		})
	}
	// This would be a nice addition to force a server to be more aggressive,
	// but does only work, when a server sends resend entries sorted from low to
	// high. Otherwise the client get's stuck because it will repeatedly
	// re-request these chunks and the server only always resends the same
	// chunks.
	//else if f.head < f.chunks {
	//		l := f.chunks - f.head
	//		for l > 255 {
	//			res = append(res, &ResendEntry{
	//				fileIndex: f.index,
	//				offset:    f.head,
	//				length:    255,
	//			})
	//			l -= 255
	//		}
	//		if l > 0 {
	//			res = append(res, &ResendEntry{
	//				fileIndex: f.index,
	//				offset:    f.head,
	//				length:    uint8(l),
	//			})
	//		}
	//}
	return &resendData{
		started:    (f.head > 0) || f.buffer.Len() > 0,
		metadata:   f.metadata,
		head:       f.head,
		res:        res,
		bufferSize: f.getMaxTransmissionRate(),
	}
}

func (f *FileResponse) getMaxTransmissionRate() int {
	if f.maxBufferSize > f.buffer.Len() {
		return f.maxBufferSize - f.buffer.Len()
	} else {
		if f.buffer.Top() < f.head {
			return 0
		}
		return 10 * int(f.buffer.Top()-f.head)
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
			log.Printf("metadata: %v\n", metadata)
			f.lock.Lock()
			if metadata.status != noErr {
				f.Err = fmt.Errorf("Server returned error for file %d: status %s",
					f.index, metadata.status.String())
				f.lock.Unlock()
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
				if f.metadata && payload.offset == f.chunks-1 {
					log.Printf("writing last chunk")
					lastSize := f.size - (f.chunks-1)*1024
					f.pwriter.Write(payload.data[:lastSize])
				} else {
					f.pwriter.Write(payload.data)
				}
				f.lock.Lock()
				delete(f.resendEntries, f.head)
				f.head++
				f.lock.Unlock()
			} else if payload.offset > f.head {
				if payload.offset > f.head {
					f.lock.Lock()
					heap.Push(f.buffer, payload)
					f.outOfOrder[payload.offset] = struct{}{}
					for i := f.head; i < payload.offset; i++ {
						f.resendEntries[i] = struct{}{}
					}
					f.lock.Unlock()
				}
			}
			f.drainBuffer()

		case <-f.cc:
			f.drainBuffer()
			f.Err = fmt.Errorf("Write canceled")
			return
		}

		log.Printf("file %v at head %v and buffer size %v\n", f.index, f.head, f.buffer.Len())
		print(f.head, uint64(f.buffer.Len()), f.chunks)
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
			if f.metadata && payload.offset == f.chunks-1 {
				log.Printf("writing last chunk")
				lastSize := f.size - (f.chunks-1)*1024
				f.pwriter.Write(payload.data[:lastSize])
			} else {
				f.pwriter.Write(payload.data)
			}
			delete(f.resendEntries, f.head)
			f.head++
		}
		top = f.buffer.Top()
	}
}
