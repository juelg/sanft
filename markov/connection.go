package markov

import (
	"fmt"
	"net"
)

		// IP:   net.ParseIP(ip),
func CreateServerSocket(ip net.IP, port int, p float64, q float64) (net.PacketConn, error){
	laddr := net.UDPAddr{
		Port: port,
		IP:   ip,
	}
	conn, err := net.ListenUDP("udp", &laddr)
	if err != nil {
		return nil, fmt.Errorf("error creating ListenUDP: %w", err)
	}
	markovConn := &MarkovConn{
		UDPConn: conn,
		P: p,
		Q: q,
		lastDropped: false,
	}
	return markovConn, nil
}

func CreateClientSocket(ip net.IP, port int, p float64, q float64) (net.Conn, error){
	raddr := &net.UDPAddr{
		Port: port,
		IP:   ip,
	}

	// this automatically takes local laddr
	conn, err := net.DialUDP("udp", nil, raddr)
	if err != nil {
		return nil, fmt.Errorf("error dialing to server: %w", err)
	}
	markovConn := &MarkovConn{
		UDPConn: conn,
		P: p,
		Q: q,
		lastDropped: false,
	}
	return markovConn, nil
}
