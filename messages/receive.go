package messages

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"net"
	"time"
)


type UnsupporedVersionError struct{
	s string
}
func (e *UnsupporedVersionError) Error() string {
	return e.s
}

type UnsupporedTypeError struct{
	s string
}
func (e *UnsupporedTypeError) Error() string {
	return e.s
}

type WrongPacketLengthError struct{
	s string
}
func (e *WrongPacketLengthError) Error() string {
	return e.s
}


// timeout in milli seconds
func ServerReceive(conn net.PacketConn, timeout int64) (net.Addr, []byte, error) {
	buffer := make([]byte, (2<<16)-1)

	deadline := time.Now().Add(time.Duration(timeout) * time.Millisecond)
	err := conn.SetReadDeadline(deadline)
	if err != nil {
		return nil, nil, fmt.Errorf("creating the timeout deadline: %w", err)
	}

	n, raddr, err := conn.ReadFrom(buffer)
	if err != nil {
		return nil, nil, err
	}
	// TODO: is this bad because is copies memory around?
	return raddr, buffer[:n], nil
}

// timeout in milli seconds
func ClientReceive(conn net.PacketConn, timeout int64) ([]byte, error) {
	buffer := make([]byte, (2<<16)-1)

	deadline := time.Now().Add(time.Duration(timeout) * time.Millisecond)
	err := conn.SetReadDeadline(deadline)
	if err != nil {
		return nil, fmt.Errorf("creating the timeout deadline: %w", err)
	}

	n, _, err := conn.ReadFrom(buffer)
	if err != nil {
		// return error as it is to match for timeout error type
		return nil, err
	}
	// TODO: is this bad because is copies memory around?
	return buffer[:n], nil
}


// Parses messages received by the server and send by the client
func ParseClient(data *[]byte) (ClientMessage, error) {
	// error types: unsupported version, unsupported type, length miss match (TODO check packet lengths)
	d := *data

	// assert client header length
	if len(d) < 35 {
		return nil, &WrongPacketLengthError{s:fmt.Sprintf("packet too small, cant fit header, should be at least 35B is %d", len(d))}
	}

	// check version
	if d[0] != VERS {
		return nil, &UnsupporedVersionError{s:fmt.Sprintf("wrong version: %d", d[0])}
	}

	var parsed_data ClientMessage
	var err error
	switch d[1] {
	case MDR_t:
		// assert packet length: header + at least one byte URI
		if len(d) < 36 {
			return nil, &WrongPacketLengthError{s:fmt.Sprintf("packet too small, cant fit URI, should be at least 36B is %d", len(d))}
		}

		var mdr MDR
		// parse client header
		var client_header ClientHeader
		err = binary.Read(bytes.NewBuffer(d[:35]), binary.BigEndian, &client_header)
		mdr.Header = client_header
		// parse string seperatly
		mdr.URI = string(d[35:])
		parsed_data = mdr

	case ACR_t:
		// assert length: header + 8B + >= 7B (at least one CR)
		if len(d) < 50 {
			return nil, &WrongPacketLengthError{s:fmt.Sprintf("packet too small should be at least 50B is %d", len(d))}
		}

		var acr ACR
		err = binary.Read(bytes.NewBuffer(d[:35]), binary.BigEndian, &acr.Header)
		if err != nil {
			return nil, fmt.Errorf("failed to read: %w", err)
		}
		err = binary.Read(bytes.NewBuffer(d[35:35+4]), binary.BigEndian, &acr.FileID)
		if err != nil {
			return nil, fmt.Errorf("failed to read: %w", err)
		}
		err = binary.Read(bytes.NewBuffer(d[35+4:43]), binary.BigEndian, &acr.PacketRate)
		if err != nil {
			return nil, fmt.Errorf("failed to read: %w", err)
		}

		acr.CRs = make([]CR, (len(d) - 43)/7)

		for i:=0; i < (len(d) - 43)/7; i++{
			// var cr CR
			err = binary.Read(bytes.NewBuffer(d[43+7*i:43+7*i+7]), binary.BigEndian, &acr.CRs[i])
			if err != nil {
				return nil, fmt.Errorf("failed to read: %w", err)
			}
		}

		parsed_data = acr
	default:
		// no valid client packet
		return nil, &UnsupporedTypeError{s:fmt.Sprintf("unsupported client type %d", d[1])}
	}

	if err != nil {
		return nil, fmt.Errorf("failed to read: %w", err)
	}
	return parsed_data, nil

}

// Parses messages received by the client and send by a server
func ParseServer(data *[]byte) (ServerMessage, error) {
	d := *data
	// assert server header length
	if len(d) < 4 {
		return nil, &WrongPacketLengthError{s:fmt.Sprintf("packet too small, cant fit header, should be at least 4B is %d", len(d))}
	}

	// check version
	if d[0] != VERS {
		return nil, &UnsupporedVersionError{s:fmt.Sprintf("wrong version: %d", d[0])}
	}

	r := bytes.NewReader(d)
	// TODO: is this a pointer type?
	var parsed_data ServerMessage
	var err error
	switch d[1] {
	case NTM_t:
		// assert packet length: header + 32B
		if len(d) < 36 {
			return nil, &WrongPacketLengthError{s:fmt.Sprintf("packet too small, should be at least 36B is %d", len(d))}
		}
		var ntm NTM
		err = binary.Read(r, binary.BigEndian, &ntm)
		parsed_data = ntm

	case MDRR_t:
		// If an error code is set, only return the header
		if d[3] != NoError {
			var header ServerHeader
			err = binary.Read(r, binary.BigEndian, &header)
			parsed_data = header
			break
		}
		// assert packet length: header + 2*2 + 4 + 6 + 32
		if len(d) < 50 {
			return nil, &WrongPacketLengthError{s:fmt.Sprintf("packet too small, should be at least 50B is %d", len(d))}
		}
		var mdrr MDRR
		err = binary.Read(r, binary.BigEndian, &mdrr)
		parsed_data = mdrr

	case CRR_t:
		// If an error code is set, and it is not a chunkOut of bound error,
		if d[3] != NoError && d[3] != ChunkOutOfBounds {
			var header ServerHeader
			err = binary.Read(r, binary.BigEndian, &header)
			parsed_data = header
			break
		}
		// assert packet length: header + 6 + >=1 (at least one data bytes)
		// If there is an out of bound error, there is only the offset and no data
		if len(d) < 10 || (d[3] != ChunkOutOfBounds && len(d) < 11) {
			return nil, &WrongPacketLengthError{s:fmt.Sprintf("packet too small, should be at least 11B is %d", len(d))}
		}
		var crr CRR

		err = binary.Read(bytes.NewBuffer(d[:4]), binary.BigEndian, &crr.Header)
		if err != nil {
			return nil, fmt.Errorf("failed to read header: %w", err)
		}
		err = binary.Read(bytes.NewBuffer(d[4:10]), binary.BigEndian, &crr.ChunkNumber)
		if err != nil {
			return nil, fmt.Errorf("failed to read chunknumber: %w", err)
		}
		crr.Data = []uint8(d[10:])

		parsed_data = crr

	default:
		// no valid server packet
		return nil, &UnsupporedTypeError{s:fmt.Sprintf("unsupported server type %d", d[1])}
	}

	if err != nil {
		return nil, fmt.Errorf("failed to read: %w", err)
	}
	return parsed_data, nil

}
