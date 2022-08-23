package main

import (
	"fmt"
	"gitlab.lrz.de/protocol-design-sose-2022-team-0/sanft/messages"
	"net"
)

func main() {

	// TODO: cli

	/* open a socket */
	laddr := net.UDPAddr{
		Port: 1234,
		IP:   net.ParseIP("127.0.0.1"),
	}
	conn, err := net.ListenUDP("udp", &laddr)
	// fmt.Println("Hello, World!")
	if err != nil {
		fmt.Printf("Error while listening: %v", err)
		return
	}

	// Connection should also be closed later on
	defer conn.Close()

	/* Send an NTM message */
	Token := [256]uint8{1, 2, 3, 4, 5}
	sh := messages.ServerHeader{Number: 5}
	m0 := messages.NTM{Header: sh, Token: Token}
	err = m0.Send(conn, &laddr)
	if err != nil {
		fmt.Printf("Error while sending:  %v", err)
		return
	}

	/* Send an MDR message */
	ch := messages.ClientHeader{Number: 6}
	m1 := messages.MDR{Header: ch, URI: "/test/bla/blub"}
	err = m1.Send(conn, &laddr)
	if err != nil {
		fmt.Printf("Error while sending:  %v", err)
		return
	}

	/* Send an MDRR message */
	sh = messages.ServerHeader{Number: 7}
	m2 := messages.MDRR{Header: sh, FileID: 5}
	err = m2.Send(conn, &laddr)
	if err != nil {
		fmt.Printf("Error while sending:  %v", err)
		return
	}

	/* Send an ACR message */
	ch = messages.ClientHeader{Number: 7}
	cr1 := messages.CR{Length: 5}
	cr2 := messages.CR{Length: 7}
	m3 := messages.ACR{Header: ch, CRs: []messages.CR{cr1, cr2}}
	err = m3.Send(conn, &laddr)
	if err != nil {
		fmt.Printf("Error while sending:  %v", err)
		return
	}

	/* Send an CRR message */
	sh = messages.ServerHeader{Number: 7}
	data := []uint8{1, 2, 3, 4, 5}
	m4 := messages.CRR{Header: sh, Data: data}
	err = m4.Send(conn, &laddr)
	if err != nil {
		fmt.Printf("Error while sending:  %v", err)
		return
	}
}
