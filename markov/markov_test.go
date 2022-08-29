package markov_test

import (
	"net"
	"testing"

	"gitlab.lrz.de/protocol-design-sose-2022-team-0/sanft/markov"
)

func TestCreateServerSocket(t *testing.T) {
	var conn net.PacketConn
	conn, err := markov.CreateServerSocket("127.0.0.100", 12345, 0.5, 0.6)
	if err != nil {
		t.Fatalf("Could not create server socket: %v", err)
	}
	err = conn.Close()
	if err != nil {
		t.Fatalf("Could not close server socket: %v", err)
	}
}

func TestCreateClientSocket(t *testing.T) {
	var conn net.Conn
	conn, err := markov.CreateClientSocket("127.0.0.100", 12345, 0.5, 0.6)
	if err != nil {
		t.Fatalf("Could not create client socket: %v", err)
	}
	err = conn.Close()
	if err != nil {
		t.Fatalf("Could not close server socket: %v", err)
	}
}

// TODO add test on other functions of MarkovConn
