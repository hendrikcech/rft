package rftp

import (
	"encoding"
	"log"
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

func TestMsgHeaderMarshalling(t *testing.T) {
	tests := map[string]MsgHeader{
		"zero": {
			version:   0,
			msgType:   0,
			optionLen: 0,

			hdrLen: 3,
		},
		"version1": {
			version:   1,
			msgType:   0,
			optionLen: 0,

			hdrLen: 3,
		},
		"option1": {
			version:   0,
			msgType:   0,
			optionLen: 2,
			options: []option{
				{0, 5, []byte{1, 2, 3, 4, 5}},
				{1, 0, []byte{}},
			},

			hdrLen: 12,
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
	cs := []byte("846e302501dfdab67f93c10f831d7eee")
	var csa [16]byte
	copy(csa[:], cs[:16])
	tests := map[string]ServerMetaData{
		"empty":             {},
		"zero":              {0, 0, 0, 0, [16]byte{}},
		"non-zero-uints":    {0, 1, 2, 3, [16]byte{}},
		"non-zero-checksum": {0, 1, 2, 3, csa},
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
		"no-missing":   {0, 0, 0, 0, 0, nil},
		"resend-entry": {0, 0, 0, 0, 0, []*ResendEntry{{0, 1, 2}}},
		"offset-2":     {0, 0, 0, 0, 2, []*ResendEntry{{0, 1, 2}}},
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
	log.Printf("%v\n", bin)
	checkErr(t, err)

	err = b.UnmarshalBinary(bin)
	checkErr(t, err)

	if !reflect.DeepEqual(a, b) {
		t.Errorf("%+v != %+v", a, b)
	}
}
