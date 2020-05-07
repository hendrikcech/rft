package rftp

import (
	"encoding"
	"reflect"
	"testing"
)

func checkErr(t *testing.T, err error) {
	if err != nil {
		t.Error(err)
	}
}

type UnMarshalBinary interface {
	encoding.BinaryMarshaler
	encoding.BinaryUnmarshaler
}

func checkErrWithMsg(t *testing.T, err error, msg string) {
	if err != nil {
		t.Errorf("%s: %v", msg, err)
	}
}

func TestMsgHeaderMarshalling(t *testing.T) {
	h := MsgHeader{12, 13, 14}

	testConversion(t, &h, &MsgHeader{})
}

func TestClientRequestMarshalling(t *testing.T) {
	tests := map[string]ClientRequest{
		"empty":      {},
		"one file":   {[]FileRequest{{5, "path1"}}},
		"two files":  {[]FileRequest{{5, "path1"}, {10, "path2"}}},
		"whitespace": {[]FileRequest{{5, "path 1"}, {10, "path2"}}},
		"new line":   {[]FileRequest{{5, "path\n1"}, {10, "path \n2"}}},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			testConversion(t, &tc, &ClientRequest{})
		})
	}
}

func TestFileRequestMarshalling(t *testing.T) {
	cs := []byte("a948904f2f0f479b8f8197694b30184b0d2ed1c1cd2a1ec0fb85d299a192a447")
	var csa [64]byte
	copy(csa[:], cs[:64])
	tests := map[string]FileResponse{
		"empty":             {},
		"zero":              {0, 0, 0, [64]byte{}},
		"non-zero-uints":    {1, 2, 3, [64]byte{}},
		"non-zero-checksum": {1, 2, 3, csa},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			testConversion(t, &tc, &FileResponse{})
		})
	}
}

func TestDataMarshalling(t *testing.T) {
	tests := map[string]Data{
		"empty": {header: &MsgHeader{0, 0, msgData}},
		"zero": {
			header:    &MsgHeader{0, 0, msgData},
			fileIndex: 0,
			offset:    0,
		},
		"non-zero": {&MsgHeader{9, 0, msgData}, 0, 0, []byte("some data")},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			r := &Data{header: tc.header}
			testConversion(t, &tc, r)
		})
	}
}

func TestAcknowledgementMarshalling(t *testing.T) {
	tests := map[string]Acknowledgement{
		"no-missing": {&MsgHeader{0, 0, 0}, 0, 0, nil},
		"missing":    {&MsgHeader{3, 0, 0}, 0, 0, []uint64{0, 1, 2}},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			r := &Acknowledgement{header: tc.header}
			testConversion(t, &tc, r)
		})
	}
}

func testConversion(t *testing.T, a UnMarshalBinary, b UnMarshalBinary) {
	bin, err := a.MarshalBinary()
	checkErr(t, err)

	err = b.UnmarshalBinary(bin)
	checkErr(t, err)

	if !reflect.DeepEqual(a, b) {
		t.Errorf("%+v != %+v", a, b)
	}
}
