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
	tests := map[string]MsgHeader{
		"zero": {
			version:   0,
			msgType:   0,
			optionLen: 0,

			hdrLen: 2,
		},
		"version1": {
			version:   1,
			msgType:   0,
			optionLen: 0,

			hdrLen: 2,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			testConversion(t, &tc, &MsgHeader{})
		})
	}
}

func TestClientRequestMarshalling(t *testing.T) {
	tests := map[string]ClientRequest{
		"empty": {},
		"one file": {
			maxTransmissionRate: 0,
			files:               []FileDescriptor{{5, "path1"}},
		},
		"two files": {
			maxTransmissionRate: 0,
			files:               []FileDescriptor{{5, "path1"}, {10, "path2"}},
		},
		"whitespace": {
			maxTransmissionRate: 0,
			files:               []FileDescriptor{{5, "path 1"}, {10, "path2"}},
		},
		"new line": {
			maxTransmissionRate: 0,
			files:               []FileDescriptor{{5, "path\n1"}, {10, "path \n2"}},
		},
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
	tests := map[string]ServerMetaData{
		"empty":             {},
		"zero":              {0, 0, 0, [64]byte{}},
		"non-zero-uints":    {1, 2, 3, [64]byte{}},
		"non-zero-checksum": {1, 2, 3, csa},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			testConversion(t, &tc, &ServerMetaData{})
		})
	}
}

func TestDataMarshalling(t *testing.T) {
	tests := map[string]ServerPayload{
		"empty": {},
		"zero": {
			fileIndex: 0,
			offset:    0,
		},
		"non-zero": {0, 0, 0, []byte("some data")},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			r := &ServerPayload{}
			testConversion(t, &tc, r)
		})
	}
}

func TestAcknowledgementMarshalling(t *testing.T) {
	tests := map[string]ClientAck{
		"no-missing": {0, 0, 0, 0, 0, nil},
		"missing":    {0, 0, 0, 0, 0, []ResendEntry{{0, 1, 2}}},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			r := &ClientAck{}
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
