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
		// return error as it is to match for timeout error type
		return nil, err
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
		var acr ACR
		err = binary.Read(bytes.NewBuffer(d[:259]), binary.BigEndian, &acr.Header)
		if err != nil {
			return nil, fmt.Errorf("failed to read: %v", err)
		}
		err = binary.Read(bytes.NewBuffer(d[259:259+4]), binary.BigEndian, &acr.FileID)
		if err != nil {
			return nil, fmt.Errorf("failed to read: %v", err)
		}
		err = binary.Read(bytes.NewBuffer(d[259+4:267]), binary.BigEndian, &acr.PacketRate)
		if err != nil {
			return nil, fmt.Errorf("failed to read: %v", err)
		}

		acr.CRs = make([]CR, (len(d) - 267)/7)

		for i:=0; i < (len(d) - 267)/7; i++{
			// var cr CR
			err = binary.Read(bytes.NewBuffer(d[267+7*i:267+7*i+7]), binary.BigEndian, &acr.CRs[i])
			if err != nil {
				return nil, fmt.Errorf("failed to read: %v", err)
			}
		}

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
		var crr CRR

		err = binary.Read(bytes.NewBuffer(d[:4]), binary.BigEndian, &crr.Header)
		if err != nil {
			return nil, fmt.Errorf("failed to read header: %v", err)
		}
		err = binary.Read(bytes.NewBuffer(d[4:10]), binary.BigEndian, &crr.ChunkNumber)
		if err != nil {
			return nil, fmt.Errorf("failed to read chunknumber: %v", err)
		}
		// crr.Data = make([]uint8, len(d) - 10)
		// err = binary.Read(bytes.NewBuffer(d[10:]), binary.BigEndian, &crr.Data)

		crr.Data = []uint8(d[10:])

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
