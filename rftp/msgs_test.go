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

func TestMsgHeaderMarshalling(t *testing.T) {
	tests := map[string]msgHeader{
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
				{8, []byte{1, 2, 3, 4, 5}, 7},
				{9, []byte{}, 2},
			},

			hdrLen: 12,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			testConversion(t, &tc, &msgHeader{})
		})
	}
}

func TestClientRequestMarshalling(t *testing.T) {
	tests := map[string]clientRequest{
		"empty": {},
		"one file": {
			maxTransmissionRate: 0,
			files:               []fileDescriptor{{5, "path1"}},
		},
		"two files": {
			maxTransmissionRate: 0,
			files:               []fileDescriptor{{5, "path1"}, {10, "path2"}},
		},
		"whitespace": {
			maxTransmissionRate: 0,
			files:               []fileDescriptor{{5, "path 1"}, {10, "path2"}},
		},
		"new line": {
			maxTransmissionRate: 0,
			files:               []fileDescriptor{{5, "path\n1"}, {10, "path \n2"}},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			testConversion(t, &tc, &clientRequest{})
		})
	}
}

func TestFileRequestMarshalling(t *testing.T) {
	cs := []byte("846e302501dfdab67f93c10f831d7eee")
	var csa [16]byte
	copy(csa[:], cs[:16])
	tests := map[string]serverMetaData{
		"empty":             {},
		"zero":              {0, 0, 0, 0, [16]byte{}},
		"non-zero-uints":    {0, 1, 2, 3, [16]byte{}},
		"non-zero-checksum": {0, 1, 2, 3, csa},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			testConversion(t, &tc, &serverMetaData{})
		})
	}
}

func TestDataMarshalling(t *testing.T) {
	tests := map[string]serverPayload{
		"empty": {},
		"zero": {
			fileIndex: 0,
			offset:    0,
		},
		"non-zero": {0, 0, 0, []byte("some data")},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			r := &serverPayload{}
			testConversion(t, &tc, r)
		})
	}
}

func TestAcknowledgementMarshalling(t *testing.T) {
	tests := map[string]clientAck{
		"no-missing":   {0, 0, 0, false, 0, 0, nil},
		"resend-entry": {0, 0, 0, false, 0, 0, []*resendEntry{{0, 1, 2}}},
		"offset-2":     {0, 0, 0, false, 0, 2, []*resendEntry{{0, 1, 2}}},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			r := &clientAck{}
			testConversion(t, &tc, r)
		})
	}
}

func testConversion(t *testing.T, a UnMarshalBinary, b UnMarshalBinary) {
	binA, err := a.MarshalBinary()
	checkErr(t, err)

	err = b.UnmarshalBinary(binA)
	checkErr(t, err)

	if !reflect.DeepEqual(a, b) {
		t.Errorf("%+v != %+v", a, b)
	}

	binB, err := b.MarshalBinary()
	checkErr(t, err)

	if !reflect.DeepEqual(binA, binB) {
		t.Errorf("%+v != %+v", binA, binB)
	}
}
