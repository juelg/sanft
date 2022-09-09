package main

import (
	"fmt"
	"log"
	"math"
	"os"
	"path"

	"gitlab.lrz.de/protocol-design-sose-2022-team-0/sanft/client"
	"gitlab.lrz.de/protocol-design-sose-2022-team-0/sanft/server"
	"gopkg.in/alecthomas/kingpin.v2"
)

var (
	host           = kingpin.Arg("host", "The host to request from (hostname or IPv4 address).").ResolvedIP()
	serverMode     = kingpin.Flag("server", "Server mode: accept incoming requests from any host. Operate in client mode if “-s” is not specified.").Short('s').Default("false").Bool()
	port           = kingpin.Flag("port", "Specify the port number to use (use 1337 as default if not given).").Default("1337").Short('t').Int()
	markovP        = kingpin.Flag("p", "Specify the loss probabilities for the Markov chain model.").Short('p').Default("0").Float64()
	markovQ        = kingpin.Flag("q", "Specify the loss probabilities for the Markov chain model.").Short('q').Default("0").Float64()
	fileDir        = kingpin.Flag("file-dir", "Server: Specify the directory containing the files that the server should serve. Client: Specify the directory where the requested files will be saved").Short('d').Default("./").ExistingDir()
	chunkSize      = kingpin.Flag("chunk-size", "The chunk size advertised and used by the server.").Default("4048").Int()
	maxChunksInACR = kingpin.Flag("max-chunks-in-acr", "The maximum number of chunks in an ACR allowed by the server.").Default("128").Int()
	rateIncrease   = kingpin.Flag("rate-increase", "Amount that the server sending rate should be increased in packet per second.").Default("256").Float64()
	files          = kingpin.Arg("files", "The name of the file(s) to fetch.").Default("").Strings()
)

func main() {
	kingpin.Parse()

	// check that p and q are valid
	if *markovP > 1 || *markovP < 0 || *markovQ > 1 || *markovQ < 0 {
		fmt.Println("error: p and/or q values for the markov chain are invalid")
		os.Exit(1)
	}
	if *host == nil {
		fmt.Println("error: When running in client mode, a server IP/hostname must be provided! When running in server mode a host ip must be provided!")
		os.Exit(1)
	}

	fmt.Printf("Host: %s, Server Mode: %t, port: %d, markovP: %f markovQ: %f, file-dir: %s, files: %s\n", *host, *serverMode, *port, *markovP, *markovQ, *fileDir, *files)
	fmt.Println("files: ", *files)

	if *serverMode { /* server mode */
		// replace empty path with "." and add trailing "/"
		if *fileDir == "" {
			*fileDir = "./"
		}
		folder := *fileDir
		if folder[len(folder)-1] != '/' {
			folder = folder + "/"
		}
		// Protocol specification limitations
		if *chunkSize == 0 || *chunkSize > 65517 {
			fmt.Println("error: Chunk size must be non-zero and no larger than 65517")
			os.Exit(1)
		}
		// Avoid an overflow when casting to uint16
		if *maxChunksInACR == 0 || *maxChunksInACR > math.MaxUint16 {
			fmt.Printf("error: max-chunks-in-acr must be non-zero and no larger than %d\n", math.MaxUint16)
			os.Exit(1)
		}

		log.Println("Starting server")

		s, err := server.Init(*host, *port, folder, uint16(*chunkSize), uint16(*maxChunksInACR),
			*markovP, *markovQ, *rateIncrease)
		if err != nil {
			log.Panicf(`Error creating server: %v`, err)
		}
		defer s.Conn.Close()

		close := make(chan bool)
		s.Listen(close)

	} else { /* client mode */
		if len(*files) < 1 {
			fmt.Println("error: When running in client mode, at least one file URI must be provided!")
			os.Exit(1)
		}

		clientConfig := client.DefaultConfig

		clientConfig.MarkovP = *markovP
		clientConfig.MarkovQ = *markovQ

		// Request files sequentially
		for _, file := range *files {
			localFileName := path.Join(*fileDir, file)
			err := client.RequestFile(*host, *port, file, localFileName, &clientConfig)
			if err != nil {
				fmt.Printf("File request for %q failed: %v\n", file, err)
			}
		}
	}

}
