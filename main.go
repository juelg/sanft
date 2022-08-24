package main

import (
	"fmt"
	"gitlab.lrz.de/protocol-design-sose-2022-team-0/sanft/messages"
	"gopkg.in/alecthomas/kingpin.v2"
	"net"
	"os"
)

var (
	host           = kingpin.Arg("host", "The host to request from (hostname or IPv4 address).").ResolvedIP()
	serverMode     = kingpin.Flag("server", "Server mode: accept incoming requests from any host. Operate in client mode if “-s” is not specified.").Short('s').Default("false").Bool()
	port           = kingpin.Flag("port", "Specify the port number to use (use 1337 as default if not given).").Default("1337").Short('t').Int()
	markovP        = kingpin.Flag("p", "Specify the loss probabilities for the Markov chain model.").Short('p').Int()
	markovQ        = kingpin.Flag("q", "Specify the loss probabilities for the Markov chain model.").Short('q').Int()
	fileDir        = kingpin.Flag("file-dir", "Specify the directory containing the files that the server should serve.").Short('d').Default("/var/www/sanft").ExistingDir()
	chunkSize      = kingpin.Flag("chunk-size", "The chunk size advertised and used by the server.").Default("4048").Int()
	maxChunksInACR = kingpin.Flag("max-chunks-in-acr", "The maximum number of chunks in an ACR allowed by the server.").Default("128").Int()
	files          = kingpin.Arg("files", "The name of the file(s) to fetch.").Strings()
)

func main() {
	kingpin.Parse()

	fmt.Printf("Host: %s, Server Mode: %b, port: %d, markovP: %d markovQ: %d, file-dir: %s, files: %s\n", *host, *serverMode, *port, *markovP, *markovQ, *fileDir, *files)
	fmt.Println("files: ", *files)

	if *serverMode { /* server mode */
		// TODO start server
	} else { /* client mode */
		if *host == nil {
			fmt.Printf("error: When running in client mode, a server IP/hostname must be provided!\n")
			os.Exit(1)
		}
		if len(*files) < 1 {
			fmt.Printf("error: When running in client mode, at least one file URI must be provided!\n")
			os.Exit(1)
		}
		// TODO start client threads
	}

	return

	/* open a socket */
	laddr := net.UDPAddr{
		Port: 1234,
		IP:   net.ParseIP("127.0.0.1"),
	}
	conn, err := net.ListenUDP("udp", &laddr)
	if err != nil {
		fmt.Printf("Error while listening: %v", err)
		return
	}

	// Connection should also be closed later on
	defer conn.Close()

	/* Send an NTM message */
	Token := [32]uint8{1, 2, 3, 4, 5}
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
	err = m1.Send(conn)
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
	err = m3.Send(conn)
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
