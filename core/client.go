package core

import (
	"net"
)

type ClientState struct {
	// token
	Token [256]uint8
	ServerAddr net.UDPAddr
	Conn *net.UDPConn
	PacketRate uint32
	// maybe file path and a map to file id
	// TODO: question: is the client meant to only
	// exist for a single file transfer?
}