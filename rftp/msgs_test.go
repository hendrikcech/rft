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

func testConversion(t *testing.T, a UnMarshalBinary, b UnMarshalBinary) {
	bin, err := a.MarshalBinary()
	checkErr(t, err)

	err = b.UnmarshalBinary(bin)
	checkErr(t, err)

	if !reflect.DeepEqual(a, b) {
		t.Errorf("%+v != %+v", a, b)
	}
}
