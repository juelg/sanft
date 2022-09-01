package messages

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"net"
)

func (m ServerHeader) Send(conn net.PacketConn, addr net.Addr) error {
	buf := new(bytes.Buffer)
	err := binary.Write(buf, binary.BigEndian, m)
	if err != nil {
		return fmt.Errorf("error encoding message: %w", err)
	}
	_, err = conn.WriteTo(buf.Bytes(), addr)
	if err != nil {
		return fmt.Errorf("error sending message: %w", err)
	}
	return nil
}

func (m NTM) Send(conn net.PacketConn, addr net.Addr) error {
	buf := new(bytes.Buffer)
	err := binary.Write(buf, binary.BigEndian, m)
	if err != nil {
		return fmt.Errorf("error encoding message: %w", err)
	}
	_, err = conn.WriteTo(buf.Bytes(), addr)
	if err != nil {
		return fmt.Errorf("error sending message: %w", err)
	}
	return nil
}

func (m MDR) Send(conn net.Conn) error {
	buf := new(bytes.Buffer)
	err := binary.Write(buf, binary.BigEndian, m.Header)
	if err != nil {
		return fmt.Errorf("error encoding message: %w", err)
	}
	/* variable length field of type string must be handled separately */
	_, err = buf.WriteString(m.URI)
	if err != nil {
		return fmt.Errorf("error encoding message: %w", err)
	}
	_, err = conn.Write(buf.Bytes())
	if err != nil {
		return fmt.Errorf("error sending message: %w", err)
	}
	return nil
}

func (m MDRR) Send(conn net.PacketConn, addr net.Addr) error {
	buf := new(bytes.Buffer)
	err := binary.Write(buf, binary.BigEndian, m)
	if err != nil {
		return fmt.Errorf("error encoding message: %w", err)
	}
	_, err = conn.WriteTo(buf.Bytes(), addr)
	if err != nil {
		return fmt.Errorf("error sending message: %w", err)
	}
	return nil
}

func (m ACR) Send(conn net.Conn) error {
	buf := new(bytes.Buffer)
	/* Once again, we must encode things manually and not all at once */
	/* encode header */
	err := binary.Write(buf, binary.BigEndian, m.Header)
	if err != nil {
		return fmt.Errorf("error encoding message: %w", err)
	}
	/* encode FileID */
	err = binary.Write(buf, binary.BigEndian, m.FileID)
	if err != nil {
		return fmt.Errorf("error encoding message: %w", err)
	}
	/* encode PacketRate */
	err = binary.Write(buf, binary.BigEndian, m.PacketRate)
	if err != nil {
		return fmt.Errorf("error encoding message: %w", err)
	}
	/* encode CRs */
	err = binary.Write(buf, binary.BigEndian, m.CRs)
	if err != nil {
		return fmt.Errorf("error encoding message: %w", err)
	}
	/* and finally, send it! */
	_, err = conn.Write(buf.Bytes())
	if err != nil {
		return fmt.Errorf("error sending message: %w", err)
	}
	return nil
}

func (m CRR) Send(conn net.PacketConn, addr net.Addr) error {
	buf := new(bytes.Buffer)
	/* encode header */
	err := binary.Write(buf, binary.BigEndian, m.Header)
	if err != nil {
		return fmt.Errorf("error encoding message: %w", err)
	}
	/* encode ChunkNumber */
	err = binary.Write(buf, binary.BigEndian, m.ChunkNumber)
	if err != nil {
		return fmt.Errorf("error encoding message: %w", err)
	}
	/* encode Data */
	err = binary.Write(buf, binary.BigEndian, m.Data)
	if err != nil {
		return fmt.Errorf("error encoding message: %w", err)
	}
	/* …and send it… */
	_, err = conn.WriteTo(buf.Bytes(), addr)
	if err != nil {
		return fmt.Errorf("error sending message: %w", err)
	}
	return nil
}
