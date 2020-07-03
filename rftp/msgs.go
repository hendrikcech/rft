package rftp

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"log"
	"math"
)

const (
	msgClientRequest uint8 = iota
	msgServerMetadata
	msgServerPayload
	msgClientAck
	msgClose
)

// Expects that the first 2 byte of b are already reserved for b's size
func prependSize(b []byte) {
	binary.BigEndian.PutUint16(b[:2], uint16(len(b)))
}

type option struct {
	otype  uint8
	length uint8
	value  []byte
}

func (o *option) UnmarshalBinary(data []byte) error {
	panic("not implemented") // TODO: Implement
}

func (o *option) MarshalBinary() (data []byte, err error) {
	panic("not implemented") // TODO: Implement
}

type MsgHeader struct {
	version   uint8
	msgType   uint8
	optionLen uint8
	options   []option

	hdrLen int
}

func NewMsgHeader(msgType uint8, os ...option) MsgHeader {
	olen := len(os)
	if olen > 255 {
		// TODO: Don't panic? Maybe return error
		panic("too many options")
	}
	l := 0
	for _, o := range os {
		l += 2 + int(o.length)
	}

	return MsgHeader{
		version:   0,
		msgType:   0,
		optionLen: uint8(olen),
		options:   os,

		hdrLen: l + 2,
	}
}

func (s MsgHeader) MarshalBinary() ([]byte, error) {
	buf := new(bytes.Buffer)
	vt := s.version<<4 ^ s.msgType
	binary.Write(buf, binary.BigEndian, vt)
	binary.Write(buf, binary.BigEndian, s.optionLen)
	for _, o := range s.options {
		ob, err := o.MarshalBinary()
		if err != nil {
			return nil, err
		}
		buf.Write(ob)
	}

	return buf.Bytes(), nil
}

func (s *MsgHeader) UnmarshalBinary(data []byte) error {
	if len(data) < 2 {
		return fmt.Errorf("MsgHeader too short")
	}
	vt := uint8(data[0])
	s.version = vt & 0xF0 >> 4
	s.msgType = vt & 0x0F
	s.optionLen = uint8(data[1])

	// TODO: Parse options and fix hdrLen
	s.hdrLen = 2

	return nil
}

type ClientRequest struct {
	maxTransmissionRate uint32
	files               []FileDescriptor
}

type FileDescriptor struct {
	offset   uint64
	fileName string
}

var maxFileOffset = uint64(math.Pow(2, 56)) - 1

func (s ClientRequest) MarshalBinary() ([]byte, error) {
	buf := new(bytes.Buffer)

	binary.Write(buf, binary.BigEndian, s.maxTransmissionRate)
	binary.Write(buf, binary.BigEndian, uint16(len(s.files)))

	for _, file := range s.files {
		if file.offset > maxFileOffset {
			return nil, errors.New("file offset to big")
		}

		binary.Write(buf, binary.BigEndian, sevenByteOffset(file.offset))

		pathBin := []byte(file.fileName)
		binary.Write(buf, binary.BigEndian, uint16(len(pathBin)))
		buf.Write(pathBin)
	}

	return buf.Bytes(), nil
}

func (s *ClientRequest) UnmarshalBinary(data []byte) error {
	s.maxTransmissionRate = binary.BigEndian.Uint32(data[:4])
	numFiles := binary.BigEndian.Uint16(data[4:6])

	if numFiles == 0 {
		return nil
	}

	log.Printf("extract %v file(s)\n", numFiles)
	s.files = make([]FileDescriptor, numFiles)

	dataLens := data[6:]
	for i := uint16(0); i < numFiles; i++ {
		f := FileDescriptor{}
		f.offset = uintOffset(dataLens[:7])
		log.Printf("offset: %v\n", f.offset)
		pathLen := binary.BigEndian.Uint16(dataLens[7:9])
		log.Printf("path len: %v\n", pathLen)
		f.fileName = string(dataLens[9 : 9+pathLen])
		dataLens = dataLens[9+pathLen:]
		s.files[i] = f
	}

	log.Printf("parsed CR: %v\n", s)
	return nil
}

type ServerMetaData struct {
	status    uint8
	fileIndex uint16
	size      uint64
	checkSum  [16]byte
}

func (s ServerMetaData) MarshalBinary() ([]byte, error) {
	buf := new(bytes.Buffer)
	binary.Write(buf, binary.BigEndian, byte(0))
	binary.Write(buf, binary.BigEndian, s.status)
	binary.Write(buf, binary.BigEndian, s.fileIndex)
	binary.Write(buf, binary.BigEndian, s.size)
	_, err := buf.Write(s.checkSum[:])
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), err
}

func (s *ServerMetaData) UnmarshalBinary(data []byte) error {
	s.status = uint8(data[1])
	s.fileIndex = binary.BigEndian.Uint16(data[2:4])
	s.size = binary.BigEndian.Uint64(data[4:12])

	cs := data[12:28]

	for i, c := range cs {
		s.checkSum[i] = c
	}
	return nil
}

type ServerPayload struct {
	fileIndex uint16
	ackNumber uint8
	offset    uint64
	data      []byte
}

func (s ServerPayload) MarshalBinary() ([]byte, error) {
	buf := new(bytes.Buffer)
	binary.Write(buf, binary.BigEndian, s.fileIndex)
	binary.Write(buf, binary.BigEndian, s.ackNumber)
	binary.Write(buf, binary.BigEndian, sevenByteOffset(s.offset))

	_, err := buf.Write(s.data)
	bs := buf.Bytes()
	return bs, err
}

func (s *ServerPayload) UnmarshalBinary(data []byte) error {
	s.fileIndex = binary.BigEndian.Uint16(data[0:2])
	s.ackNumber = uint8(data[2])

	s.offset = uintOffset(data[3:10])

	if len(data) > 10 {
		s.data = data[10:]
	}
	return nil
}

type ResendEntry struct {
	fileIndex uint16
	offset    uint64
	length    uint8
}

type ClientAck struct {
	ackNumber           uint8
	fileIndex           uint16
	status              uint8
	maxTransmissionRate uint32
	offset              uint64
	resendEntries       []ResendEntry
}

// make offset BigEndian and cut off the first (most significant) byte
func sevenByteOffset(offset uint64) []byte {
	offsetBuffer := new(bytes.Buffer)
	binary.Write(offsetBuffer, binary.BigEndian, offset)
	return offsetBuffer.Bytes()[1:]
}

// pad 7 byte with another zero byte to make reading easy
func uintOffset(seven []byte) uint64 {
	offsetPad := append([]byte{0}, seven...)
	return binary.BigEndian.Uint64(offsetPad)
}

func (c ClientAck) MarshalBinary() ([]byte, error) {
	buf := new(bytes.Buffer)
	binary.Write(buf, binary.BigEndian, c.ackNumber)
	binary.Write(buf, binary.BigEndian, c.fileIndex)
	binary.Write(buf, binary.BigEndian, c.status)
	binary.Write(buf, binary.BigEndian, c.maxTransmissionRate)
	binary.Write(buf, binary.BigEndian, sevenByteOffset(c.offset))

	for _, re := range c.resendEntries {
		binary.Write(buf, binary.BigEndian, re.fileIndex)
		binary.Write(buf, binary.BigEndian, sevenByteOffset(re.offset))
		binary.Write(buf, binary.BigEndian, re.length)
	}
	bs := buf.Bytes()
	return bs, nil
}

func (c *ClientAck) UnmarshalBinary(data []byte) error {
	c.ackNumber = uint8(data[0])
	c.fileIndex = binary.BigEndian.Uint16(data[1:3])
	c.status = uint8(data[3])
	c.maxTransmissionRate = binary.BigEndian.Uint32(data[4:8])
	c.offset = uintOffset(data[8:15])

	if len(data) > 15 {
		reBytes := data[15:]
		for i := 0; i < len(reBytes)/10; i++ {
			re := ResendEntry{}
			re.fileIndex = binary.BigEndian.Uint16(reBytes[:2])
			re.offset = uintOffset(reBytes[2:9])
			re.length = uint8(reBytes[9])
			c.resendEntries = append(c.resendEntries, re)
			reBytes = reBytes[10:]
		}

	}
	return nil
}

type CloseConnection struct {
	reason uint16
}

func (c CloseConnection) MarshalBinary() ([]byte, error) {
	buf := new(bytes.Buffer)
	binary.Write(buf, binary.BigEndian, c.reason)
	return buf.Bytes(), nil
}

func (c *CloseConnection) UnmarshalBinary(data []byte) error {
	c.reason = binary.BigEndian.Uint16(data[:2])
	return nil
}
