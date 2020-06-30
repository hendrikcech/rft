package rftp

import (
	"bytes"
	"encoding/binary"
	"fmt"
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
	fileIndex uint16
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
	fr.fileIndex = binary.BigEndian.Uint16(data[0:2])
	fr.offset = binary.BigEndian.Uint64(data[2:10])
	fr.size = binary.BigEndian.Uint64(data[10:18])

	cs := data[18:82]

	for i, c := range cs {
		fr.checkSum[i] = c
	}
	return nil
}

type Data struct {
	fileIndex uint16
	offset    uint64
	data      []byte
}

func (d Data) MarshalBinary() ([]byte, error) {
	buf := new(bytes.Buffer)
	binary.Write(buf, binary.BigEndian, d.fileIndex)
	binary.Write(buf, binary.BigEndian, d.offset)
	_, err := buf.Write(d.data)
	bs := buf.Bytes()
	return bs, err
}

func (d *Data) UnmarshalBinary(data []byte) error {
	d.fileIndex = binary.BigEndian.Uint16(data[0:2])
	d.offset = binary.BigEndian.Uint64(data[2:10])
	if len(data) > 10 {
		d.data = data[10:]
	}
	return nil
}

type Acknowledgement struct {
	fileIndex    uint16
	receivedUpTo uint64
	missing      []uint64
}

func (a Acknowledgement) MarshalBinary() ([]byte, error) {
	buf := new(bytes.Buffer)
	binary.Write(buf, binary.BigEndian, a.fileIndex)
	binary.Write(buf, binary.BigEndian, a.receivedUpTo)
	if len(a.missing) > 0 {
		binary.Write(buf, binary.BigEndian, a.missing)
	}
	bs := buf.Bytes()
	return bs, nil
}
func (a *Acknowledgement) UnmarshalBinary(data []byte) error {
	a.fileIndex = binary.BigEndian.Uint16(data[0:2])
	a.receivedUpTo = binary.BigEndian.Uint64(data[2:10])

	if len(data) > 10 {
		a.missing = make([]uint64, len(data[10:])/8)
		buf := bytes.NewBuffer(data[10:])
		err := binary.Read(buf, binary.BigEndian, a.missing)
		if err != nil {
			return err
		}
	}
	return nil
}

type Cancel struct {
	fileIndex uint16
}

func (c Cancel) MarshalBinary() ([]byte, error) {
	buf := new(bytes.Buffer)
	binary.Write(buf, binary.BigEndian, c.fileIndex)
	return buf.Bytes(), nil
}

func (c *Cancel) UnmarshalBinary(data []byte) error {
	c.fileIndex = binary.BigEndian.Uint16(data[:2])
	return nil
}
