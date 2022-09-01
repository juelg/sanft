package messages

import (
	"fmt"
	"net"
)

func CreateServerSocket(ip net.IP, port int) (*net.UDPConn, error) {
	laddr := net.UDPAddr{
		Port: port,
		IP:   ip,
	}
	conn, err := net.ListenUDP("udp", &laddr)
	if err != nil {
		return nil, fmt.Errorf("error creating ListenUDP: %w", err)
	}
	return conn, nil
}

func CreateClientSocket(ip net.IP, port int) (*net.UDPConn, error) {
	raddr := &net.UDPAddr{
		Port: port,
		IP:   ip,
	}

	// this automatically takes local laddr
	conn, err := net.DialUDP("udp", nil, raddr)
	if err != nil {
		return nil, fmt.Errorf("error dialing to server: %w", err)
	}
	return conn, nil
}
