
package messages

import (
	"fmt"
	"net"
)


// type Server struct {

// }

func CreateServerSocket(ip string, port int) (*net.UDPConn, error){
	laddr := net.UDPAddr{
		Port: port,
		IP:   net.ParseIP(ip),
	}
	conn, err := net.ListenUDP("udp", &laddr)
	// fmt.Println("Hello, World!")
	if err != nil {
		return nil, fmt.Errorf("Error creating ListenUDP: %v", err)
	}
	return conn, nil
}

func CreateClientSocket(address string, port int) (*net.UDPConn, error){
	raddr, err := net.ResolveUDPAddr("udp", address + ":" + fmt.Sprint(port))
	if err != nil {
		return nil, fmt.Errorf("Error resolving addr: %v", err)
	}

	// take local laddr
	conn, err := net.DialUDP("udp", nil, raddr)
	if err != nil {
		return nil, fmt.Errorf("Error dialing to server: %v", err)
	}
	return conn, nil
}