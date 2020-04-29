package main

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"io"
	"math/rand"
	"net"
	"time"
)

const (
	ENROLL_INIT     uint16 = 680
	ENROLL_REGISTER uint16 = 681
	ENROLL_SUCCESS  uint16 = 682
	ENROLL_FAILURE  uint16 = 683
)

func checkMsgType(r io.Reader, expected uint16) error {
	var msgType uint16
	binary.Read(r, binary.BigEndian, &msgType)
	if msgType != expected {
		return fmt.Errorf("Unexpected message type: %v instead of %v",
			msgType, expected)
	}
	return nil
}

func getMsgType(r *bufio.Reader) (uint16, error) {
	b, err := r.Peek(4)
	if err != nil {
		return 0, err
	}

	return binary.BigEndian.Uint16(b[2:]), nil
}

type EnrollInit struct {
	challenge [8]byte
}

func ParseEnrollInit(r *bufio.Reader) (EnrollInit, error) {
	r.Discard(2) // Skip size

	if err := checkMsgType(r, ENROLL_INIT); err != nil {
		return EnrollInit{}, err
	}

	var msg EnrollInit
	r.Read(msg.challenge[:])

	return msg, nil
}

type RegisterBody struct {
	challenge  [8]byte
	teamNumber uint16
	project    uint16
	nonce      [8]byte
	email      string
	firstname  string
	lastname   string
}

func (msg *RegisterBody) MarshalBinary() ([]byte, error) {
	buf := new(bytes.Buffer)
	buf.Write(make([]byte, 2)) // size placeholder
	binary.Write(buf, binary.BigEndian, ENROLL_REGISTER)
	buf.Write(msg.challenge[:])
	binary.Write(buf, binary.BigEndian, msg.teamNumber)
	binary.Write(buf, binary.BigEndian, msg.project)
	buf.Write(msg.nonce[:])
	buf.Write([]byte(msg.email))
	buf.Write([]byte("\r\n"))
	buf.Write([]byte(msg.firstname))
	buf.Write([]byte("\r\n"))
	buf.Write([]byte(msg.lastname))

	b := buf.Bytes()
	binary.BigEndian.PutUint16(b[:2], uint16(buf.Len()))
	return b, nil
}

type EnrollSuccess struct {
	teamNumber uint16
}

func ParseEnrollSuccess(r *bufio.Reader) (EnrollSuccess, error) {
	d, err := r.Peek(8)
	if err != nil {
		fmt.Printf("EnrollSuccess peek failed: %v\n", err)
	}
	fmt.Printf("EnrollSuccess peek: %v\n", d)

	r.Discard(2) // Skip size

	if err := checkMsgType(r, ENROLL_SUCCESS); err != nil {
		return EnrollSuccess{}, err
	}

	r.Discard(2) // Skip reserved

	var msg EnrollSuccess
	if err = binary.Read(r, binary.BigEndian, &msg.teamNumber); err != nil {
		return msg, err
	}

	return msg, nil
}

type EnrollFailure struct {
	errorNumber uint16
	description string
}

func ParseEnrollFailure(r *bufio.Reader) (EnrollFailure, error) {
	r.Discard(2) // Skip size

	if err := checkMsgType(r, ENROLL_FAILURE); err != nil {
		return EnrollFailure{}, err
	}

	r.Discard(2) // Skip reserved

	var msg EnrollFailure
	if err := binary.Read(r, binary.BigEndian, &msg.errorNumber); err != nil {
		return EnrollFailure{}, err
	}

	buf := new(bytes.Buffer)
	if _, err := buf.ReadFrom(r); err != nil {
		return EnrollFailure{}, err
	}
	msg.description = buf.String()

	return msg, nil
}

func main() {
	conn, err := net.Dial("tcp", "p2psec.net.in.tum.de:13337")
	if err != nil {
		fmt.Println(err)
		return
	}

	r := bufio.NewReader(conn)

	enrollInit, err := ParseEnrollInit(r)
	if err != nil {
		fmt.Println(err)
		return
	}

	register := RegisterBody{
		challenge:  enrollInit.challenge,
		teamNumber: 0,
		project:    2961, // Gossip
		email:      "",
		firstname:  "",
		lastname:   "",
	}

	fmt.Println("Calculating proof-of-work...")
	rand.Seed(time.Now().UnixNano())
	var registerBin []byte
	for {
		rand.Read(register.nonce[:])

		registerBin, _ = register.MarshalBinary()

		sha := sha256.Sum256(registerBin[4:]) // Skip header

		if sha[0] == 0 && sha[1] == 0 && sha[2] == 0 {
			break
		}
	}

	fmt.Println("Sending registration message")

	conn.Write(registerBin)

	respType, err := getMsgType(r)
	if err != nil {
		fmt.Println(err)
		return
	}

	fmt.Printf("Received response with msgType %d\n", respType)

	if respType == ENROLL_SUCCESS {
		enrollSuccess, err := ParseEnrollSuccess(r)
		if err != nil {
			fmt.Println(err)
			return
		}
		fmt.Printf("%+v\n", enrollSuccess)
	} else if respType == ENROLL_FAILURE {
		enrollFailure, err := ParseEnrollFailure(r)
		if err != nil {
			fmt.Println(err)
			return
		}
		fmt.Printf("%+v\n", enrollFailure)
	} else {
		fmt.Printf("Unexpected message type: %d\n", respType)
		return
	}

	return
}
