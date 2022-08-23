
package messages

import (
	"fmt"
	"net"
)


func CreateServerSocket(ip string, port int) (*net.UDPConn, error){
	laddr := net.UDPAddr{
		Port: port,
		IP:   net.ParseIP(ip),
	}
	conn, err := net.ListenUDP("udp", &laddr)
	if err != nil {
		return nil, fmt.Errorf("error creating ListenUDP: %v", err)
	}
	return conn, nil
}

func CreateClientSocket(address string, port int) (*net.UDPConn, error){
	raddr, err := net.ResolveUDPAddr("udp", address + ":" + fmt.Sprint(port))
	if err != nil {
		return nil, fmt.Errorf("error resolving addr: %v", err)
	}

	// this automatically takes local laddr
	conn, err := net.DialUDP("udp", nil, raddr)
	if err != nil {
		return nil, fmt.Errorf("error dialing to server: %v", err)
	}
	return conn, nil
}