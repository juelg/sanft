package messages

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"net"
	"math"
	"time"
)


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


func ParseClient(data *[]byte) (ClientMessage, error) {
	d := *data
	// check version
	if d[0] != VERS {
		return nil, fmt.Errorf("wrong version: %d", d[0])
	}

	var parsed_data ClientMessage
	var err error
	switch d[1] {
	case MDR_t:
		var mdr MDR
		// parse client header
		var client_header ClientHeader
		err = binary.Read(bytes.NewBuffer(d[:259]), binary.BigEndian, &client_header)
		mdr.Header = client_header
		// parse string seperatly
		mdr.URI = string(d[259:])
		parsed_data = mdr

	case ACR_t:
		// TODO: maybe this needs to be done manually
		r := bytes.NewReader(d)
		var acr ACR
		err = binary.Read(r, binary.BigEndian, &acr)
		parsed_data = acr
	default:
		// no valid client packet
		return nil, fmt.Errorf("received wrong client message type: %d", d[1])
	}

	if err != nil {
		return nil, fmt.Errorf("failed to read: %v", err)
	}
	return parsed_data, nil

}

func ParseServer(data *[]byte) (ServerMessage, error) {
	d := *data
	// check version
	if d[0] != VERS {
		return nil, fmt.Errorf("wrong version: %d", d[0])
	}

	r := bytes.NewReader(d)
	// TODO: is this a pointer type?
	var parsed_data ServerMessage
	var err error
	switch d[1] {
	case NTM_t:
		var ntm NTM
		err = binary.Read(r, binary.BigEndian, &ntm)
		parsed_data = ntm

	case MDRR_t:
		var mdrr MDRR
		err = binary.Read(r, binary.BigEndian, &mdrr)
		parsed_data = mdrr

	case CRR_t:
		// TODO: maybe this needs to be done manually
		var crr CRR
		err = binary.Read(r, binary.BigEndian, &crr)
		parsed_data = crr

	default:
		// no valid server packet
		return nil, fmt.Errorf("received wrong server message type: %d", d[1])
	}

	if err != nil {
		return nil, fmt.Errorf("failed to read")
	}
	return parsed_data, nil

}
