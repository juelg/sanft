package client

import (
	"fmt"
	"gitlab.lrz.de/protocol-design-sose-2022-team-0/sanft/messages"
	"net"
	"time"
	"os"
)


type fileMetadata struct {
	// Metadata from the metadata request
	chunkSize uint16
	maxChunksInACR uint16
	fileID uint32
	fileSize uint64
	checksum [256]byte

	// Information on missing chunks
	chunkMap map[int64]bool //chunkMap[chunk] == true iff chunk has been received
	firstMissing uint64

	// Information on the connection
	timeout time.Duration
	packetRate uint32
}

func RequestFile(address string, port int, URL string, localFilename string) error {
    conn, err := messages.CreateClientSocket(address, port)
	if err != nil {
		return fmt.Errorf("Create client socket: %v", err)
	}
	defer conn.Close()

	// Request file metadata
	metadata, err := getMetadata(conn, URL)
	if err != nil {
		// TODO Error handling.
		// If timeout: retransmit
		// If protocol error: abort
	}

	localFile, err := os.Create(localFilename)
	defer localFile.Close()
	if err != nil {
		return fmt.Errorf("Open file %s: %v", localFilename, err)
	}
	// Request chunks
	for metadata.firstMissing <= metadata.fileSize {
		// TODO congestion control
		err := getMissingChunks(conn, &metadata, localFile)
		if err != nil {
			// TODO error handling
		}
	}

	// TODO Check checksum
	checksum, err := computeChecksum(localFilename)
	if err != nil {
		return fmt.Errorf("Compute checksum of %s: %v", localFilename, err)
	}
	if checksum != metadata.checksum {
		// TODO: Delete file (and retry ?)
		return fmt.Errorf("Checksum not matching. Expected %x got %x", metadata.checksum, checksum)
	}

	return nil
}

func getMetadata(conn *net.UDPConn, URL string) (fileMetadata, error) {
	// TODO Send a metadata request and receive response
	// TODO Decide on values for timeout and number of retransmissions
	return fileMetadata{}, nil
}

func getMissingChunks(conn *net.UDPConn, metadata *fileMetadata, localFile *os.File) error {
	// TODO Send File chunk request for missing chunks
	// TODO Receive chunks and write them to memory
	// TODO update measured rate and timeout
	return nil
}

func computeChecksum(filename string) ([256]byte, error) {
	var hash [256]byte
	// TODO compute checksum of file

	return hash, nil
}
