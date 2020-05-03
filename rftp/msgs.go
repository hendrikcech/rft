package rftp

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

const (
	MSG_CLIENT_REQUEST uint16 = iota
)

// Expects that the first 2 byte of b are already reserved for b's size
func prependSize(b []byte) {
	binary.BigEndian.PutUint16(b[:2], uint16(len(b)))
}

type MsgHeader struct {
	size    uint32
	version uint16
	msgType uint16
}

func NewMsgHeader(msgType uint16) MsgHeader {
	return MsgHeader{0, 0, msgType}
}

func (s MsgHeader) MarshalBinary() ([]byte, error) {
	buf := new(bytes.Buffer)
	binary.Write(buf, binary.BigEndian, s.size)
	binary.Write(buf, binary.BigEndian, s.version)
	binary.Write(buf, binary.BigEndian, s.msgType)
	return buf.Bytes(), nil
}

func (s *MsgHeader) UnmarshalBinary(data []byte) error {
	if len(data) != 8 {
		return fmt.Errorf("MsgHeader's size != 8")
	}

	s.size = binary.BigEndian.Uint32(data[:4])
	s.version = binary.BigEndian.Uint16(data[4:6])
	s.msgType = binary.BigEndian.Uint16(data[6:8])

	return nil
}

type ClientRequest struct {
	files []FileRequest
}

type FileRequest struct {
	offset uint64
	path   string
}

func (s ClientRequest) MarshalBinary() ([]byte, error) {
	buf := new(bytes.Buffer)

	binary.Write(buf, binary.BigEndian, uint16(len(s.files)))

	for _, file := range s.files {
		binary.Write(buf, binary.BigEndian, file.offset)
		pathBin := []byte(file.path)
		binary.Write(buf, binary.BigEndian, uint16(len(pathBin)))
		buf.Write(pathBin)
	}

	return buf.Bytes(), nil
}

func (s *ClientRequest) UnmarshalBinary(data []byte) error {
	numFiles := binary.BigEndian.Uint16(data[:2])

	if numFiles == 0 {
		return nil
	}

	s.files = make([]FileRequest, numFiles)

	dataLens := data[2:]
	for i := uint16(0); i < numFiles; i++ {
		f := FileRequest{}
		f.offset = binary.BigEndian.Uint64(dataLens[:8])
		pathLen := binary.BigEndian.Uint16(dataLens[8:10])
		f.path = string(dataLens[10 : 10+pathLen])
		dataLens = dataLens[10+pathLen:]
		s.files[i] = f
	}

	return nil
}
