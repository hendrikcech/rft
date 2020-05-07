package rftp

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
)

const (
	msgClientRequest uint16 = iota
	msgFileResponse
	msgData
	msgAck
	msgClose
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

type FileResponse struct {
	fileIndex uint32
	offset    uint64
	size      uint64
	checkSum  [64]byte
}

func (fr *FileResponse) MarshalBinary() ([]byte, error) {
	buf := new(bytes.Buffer)
	binary.Write(buf, binary.BigEndian, fr.fileIndex)
	binary.Write(buf, binary.BigEndian, fr.offset)
	binary.Write(buf, binary.BigEndian, fr.size)
	_, err := buf.Write(fr.checkSum[:])
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), err
}

func (fr *FileResponse) UnmarshalBinary(data []byte) error {
	fr.fileIndex = binary.BigEndian.Uint32(data[0:4])
	fr.offset = binary.BigEndian.Uint64(data[4:12])
	fr.size = binary.BigEndian.Uint64(data[12:20])

	cs := data[20:84]

	for i, c := range cs {
		fr.checkSum[i] = c
	}
	return nil
}

type Data struct {
	header    *MsgHeader
	fileIndex uint32
	offset    uint64
	data      []byte
}

func (d Data) MarshalBinary() ([]byte, error) {
	buf := new(bytes.Buffer)
	binary.Write(buf, binary.BigEndian, d.fileIndex)
	binary.Write(buf, binary.BigEndian, d.offset)
	_, err := buf.Write(d.data)
	return buf.Bytes(), err
}

func (d *Data) UnmarshalBinary(data []byte) error {
	if d.header == nil {
		return errors.New("header and size need to be known to decode a data packet")
	}
	d.fileIndex = binary.BigEndian.Uint32(data[0:4])
	d.offset = binary.BigEndian.Uint64(data[4:12])

	if d.header.size > 0 {
		d.data = data[12 : 12+d.header.size]
	}

	return nil
}

type Acknowledgement struct {
	header       *MsgHeader
	fileIndex    uint32
	receivedUpTo uint64
	missing      []uint64
}

func (a Acknowledgement) MarshalBinary() ([]byte, error) {
	buf := new(bytes.Buffer)
	binary.Write(buf, binary.BigEndian, a.fileIndex)
	binary.Write(buf, binary.BigEndian, a.receivedUpTo)
	binary.Write(buf, binary.BigEndian, a.missing)
	return buf.Bytes(), nil
}
func (a *Acknowledgement) UnmarshalBinary(data []byte) error {
	if a.header == nil {
		return errors.New("header and size need to be known to decode a data packet")
	}
	a.fileIndex = binary.BigEndian.Uint32(data[0:4])
	a.receivedUpTo = binary.BigEndian.Uint64(data[4:12])
	buf := bytes.NewBuffer(data[12:])

	if a.header.size > 0 {
		a.missing = make([]uint64, a.header.size)
	}
	binary.Read(buf, binary.BigEndian, a.missing)
	return nil
}

type Cancel struct {
	fileIndex uint32
}

func (c Cancel) MarshalBinary() ([]byte, error) {
	buf := new(bytes.Buffer)
	binary.Write(buf, binary.BigEndian, c.fileIndex)
	return buf.Bytes(), nil
}

func (c *Cancel) UnmarshalBinary(data []byte) error {
	c.fileIndex = binary.BigEndian.Uint32(data[:4])
	return nil
}
