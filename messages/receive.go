package messages

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"net"

	// "strings"
	// "strconv"
	"math"
	"time"
	// "binary"
)

// global map which saves the received messages

// timeout given in milli seconds
func Receive(conn *net.UDPConn, addr *net.UDPAddr, timeout int64) ([]byte, error) {
	// buf := new(bytes.Buffer)
	buffer := make([]byte, (2<<16)-1)
	var n int
	var raddr *net.UDPAddr
	var err error

	// deadline in nanoseconds
	deadline := time.Now().Add(time.Duration(timeout * int64(math.Pow10(6))))
	err = conn.SetReadDeadline(deadline)
	if err != nil {
		return nil, fmt.Errorf("creating the timeout deadline: %v", err)
	}

	for {
		n, raddr, err = conn.ReadFromUDP(buffer)
		if addr == raddr {
			break
		}
	}
	if err != nil {
		return nil, fmt.Errorf("error receiving message: %v", err)
	}
	return buffer[:n], nil
}

func ServerReceive(conn *net.UDPConn) (*net.UDPAddr, []byte, error) {
	buffer := make([]byte, (2<<16)-1)

	n, raddr, err := conn.ReadFromUDP(buffer)
	if err != nil {
		return nil, nil, fmt.Errorf("error receiving message: %v", err)
	}
	// TODO: is this bad because is copies memory around?
	return raddr, buffer[:n], nil
}

// timeout in milli seconds
func ClientReceive(conn *net.UDPConn, timeout int64) ([]byte, error) {
	buffer := make([]byte, (2<<16)-1)

	deadline := time.Now().Add(time.Duration(timeout * int64(math.Pow10(6))))
	err := conn.SetReadDeadline(deadline)
	if err != nil {
		return nil, fmt.Errorf("creating the timeout deadline: %v", err)
	}

	n, _, err := conn.ReadFromUDP(buffer)
	if err != nil {
		return nil, fmt.Errorf("error receiving message: %v", err)
	}
	// TODO: is this bad because is copies memory around?
	return buffer[:n], nil
}

// func Parse(data *[]byte) error {
// 	d := *data
// 	// check version
// 	if d[0] != 0{
// 		return fmt.Errorf("Wrong Version: %i", d[0])
// 	}
// 	// client or server message
// 	if d[1] == 1 || d[1] == 3{
// 		// from client for server
// 	} else if d[1] == 0 || d[1] == 2 || d[1] == 4{
// 		// from server for client
// 	} else{
// 		return fmt.Errorf("Received wrong message type: %i", d[1])
// 	}
// }

func ParseClient(data *[]byte) (Message, error) {
	d := *data
	// check version
	if d[0] != 0 {
		return nil, fmt.Errorf("wrong version: %d", d[0])
	}
	if d[1] != 1 && d[1] != 3 {
		// no valid client packet
		return nil, fmt.Errorf("received wrong client message type: %d", d[1])
	}
	// err := binary.Write(buf, binary.BigEndian, m)
	var parsed_data Message
	var err error
	switch d[1] {
	case 1:
		var mdr MDR
		// parse client header
		var client_header ClientHeader
		err = binary.Read(bytes.NewBuffer(d[:259]), binary.BigEndian, &client_header)
		mdr.Header = client_header
		// parse string seperatly
		mdr.URI = string(d[259:])
		parsed_data = mdr

	case 3:
		// TODO: maybe this needs to be done manually
		r := bytes.NewReader(d)
		var acr ACR
		err = binary.Read(r, binary.BigEndian, &acr)
		parsed_data = acr
	}

	if err != nil {
		return nil, fmt.Errorf("failed to read: %v", err)
	}
	return parsed_data, nil

}

func ParseServer(data *[]byte) (Message, error) {
	d := *data
	// check version
	if d[0] != 0 {
		return nil, fmt.Errorf("wrong version: %d", d[0])
	}
	if d[1] != 0 && d[1] != 2 && d[1] != 4 {
		// no valid server packet
		return nil, fmt.Errorf("received wrong server message type: %d", d[1])
	}
	r := bytes.NewReader(d)
	var parsed_data Message
	var err error
	switch d[1] {
	case 0:
		var ntm NTM
		err = binary.Read(r, binary.BigEndian, &ntm)
		parsed_data = ntm

	case 2:
		var mdrr MDRR
		err = binary.Read(r, binary.BigEndian, &mdrr)
		parsed_data = mdrr

	case 4:
		// TODO: maybe this needs to be done manually
		var crr CRR
		err = binary.Read(r, binary.BigEndian, &crr)
		parsed_data = crr
	}

	if err != nil {
		return nil, fmt.Errorf("failed to read")
	}
	return parsed_data, nil

}
