package markov

import (
	"fmt"
	"net"
)
func CreateServerSocket(ip string, port int, p float64, q float64) (net.PacketConn, error){
	laddr := net.UDPAddr{
		Port: port,
		IP:   net.ParseIP(ip),
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

func CreateClientSocket(address string, port int, p float64, q float64) (net.Conn, error){
	raddr, err := net.ResolveUDPAddr("udp", address + ":" + fmt.Sprint(port))
	if err != nil {
		return nil, fmt.Errorf("error resolving addr: %w", err)
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
