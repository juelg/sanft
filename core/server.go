package core

import (
	"net"
)

type ServerState struct {
	// keying material
	// fileID map: needs to be in both ways
	// -> maybe write own abstraction to keep in sync
	// or find library
	File2ID map[string]uint32
	ID2File map[uint32]string
	// read out from some config:
	ChunkSize      uint16
	MaxChunksInACR uint16
	Conn *net.UDPConn
}