package messages

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"net"
)

// todo: client messages should have an addr argument and should just use the Write method

func (m NTM) Send(conn *net.UDPConn, addr *net.UDPAddr) error {
	buf := new(bytes.Buffer)
	// TODO: we want to send big endian, right?
	err := binary.Write(buf, binary.BigEndian, m)
	if err != nil {
		return fmt.Errorf("Error encoding message: %v", err)
	}
	_, err = conn.WriteToUDP(buf.Bytes(), addr)
	if err != nil {
		return fmt.Errorf("Error sending message: %v", err)
	}
	return nil
}

func (m MDR) Send(conn *net.UDPConn, addr *net.UDPAddr) error {
	buf := new(bytes.Buffer)
	err := binary.Write(buf, binary.BigEndian, m.Header)
	if err != nil {
		return fmt.Errorf("Error encoding message: %v", err)
	}
	/* variable length field of type string must be handled separately */
	_, err = buf.WriteString(m.URI)
	if err != nil {
		return fmt.Errorf("Error encoding message: %v", err)
	}
	// _, err = conn.WriteToUDP(buf.Bytes(), addr)
	_, err = conn.Write(buf.Bytes())
	if err != nil {
		return fmt.Errorf("Error sending message: %v", err)
	}
	return nil
}

func (m MDRR) Send(conn *net.UDPConn, addr *net.UDPAddr) error {
	buf := new(bytes.Buffer)
	err := binary.Write(buf, binary.BigEndian, m)
	if err != nil {
		return fmt.Errorf("Error encoding message: %v", err)
	}
	_, err = conn.WriteToUDP(buf.Bytes(), addr)
	if err != nil {
		return fmt.Errorf("Error sending message: %v", err)
	}
	return nil
}

func (m ACR) Send(conn *net.UDPConn, addr *net.UDPAddr) error {
	buf := new(bytes.Buffer)
	/* Once again, we must encode things manually and not all at once */
	/* encode header */
	err := binary.Write(buf, binary.BigEndian, m.Header)
	if err != nil {
		return fmt.Errorf("Error encoding message: %v", err)
	}
	/* encode FileID */
	err = binary.Write(buf, binary.BigEndian, m.FileID)
	if err != nil {
		return fmt.Errorf("Error encoding message: %v", err)
	}
	/* encode PacketRate */
	err = binary.Write(buf, binary.BigEndian, m.PacketRate)
	if err != nil {
		return fmt.Errorf("Error encoding message: %v", err)
	}
	/* encode CRs */
	err = binary.Write(buf, binary.BigEndian, m.CRs)
	if err != nil {
		return fmt.Errorf("Error encoding message: %v", err)
	}
	/* and finally, send it! */
	_, err = conn.WriteToUDP(buf.Bytes(), addr)
	if err != nil {
		return fmt.Errorf("Error sending message: %v", err)
	}
	return nil
}

func (m CRR) Send(conn *net.UDPConn, addr *net.UDPAddr) error {
	buf := new(bytes.Buffer)
	/* encode header */
	err := binary.Write(buf, binary.BigEndian, m.Header)
	if err != nil {
		return fmt.Errorf("Error encoding message: %v", err)
	}
	/* encode ChunkNumber */
	err = binary.Write(buf, binary.BigEndian, m.ChunkNumber)
	if err != nil {
		return fmt.Errorf("Error encoding message: %v", err)
	}
	/* encode Data */
	err = binary.Write(buf, binary.BigEndian, m.Data)
	if err != nil {
		return fmt.Errorf("Error encoding message: %v", err)
	}
	/* …and send it… */
	_, err = conn.WriteToUDP(buf.Bytes(), addr)
	if err != nil {
		return fmt.Errorf("Error sending message: %v", err)
	}
	return nil
}
