package messages

import (
	"net"
)

// TODO: do we need to pack the structs?

type Message interface {
	Send(conn *net.UDPConn, addr *net.UDPAddr) error
}

type ClientHeader struct {
	Version uint8
	Type    uint8
	Number  uint8
	Token   [256]uint8
}

type ServerHeader struct {
	Version uint8
	Type    uint8
	Number  uint8
	Error   uint8
}

// TODO constructors
type NTM struct {
	Header ServerHeader
	Token  [256]uint8
}

type MDR struct {
	Header ClientHeader
	URI    string /* this must be handled manualy when sending */
}

type MDRR struct {
	Header         ServerHeader
	ChunkSize      uint16
	MaxChunksInACR uint16
	FileID         uint32
	FileSize       [6]uint8 /* alias, there is no uint48 :-( */
	Checksum       [256]uint8
}

type CR struct {
	ChunkOffset [6]uint8 /* note to self: 48-bit field sizes were a dumb idea */
	Length      uint8
}

type ACR struct {
	Header     ClientHeader
	FileID     uint32
	PacketRate uint32
	CRs        []CR
}

type CRR struct {
	Header      ServerHeader
	ChunkNumber [6]uint8 /* :-( */
	Data        []uint8
}
