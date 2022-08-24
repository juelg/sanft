package client

import (
	"errors"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"net"
	"os"
	"time"

	"gitlab.lrz.de/protocol-design-sose-2022-team-0/sanft/messages"
)


type fileMetadata struct {
	// Metadata from the metadata request

	token [32]uint8
	chunkSize uint16
	maxChunksInACR uint16
	fileID uint32
	fileSize uint64
	checksum [32]byte

	// Information on missing chunks

	chunkMap map[int64]bool //chunkMap[chunk] == true iff chunk has been received
	firstMissing uint64 // The index of the first chunk not yet received

	// Information on the connection

	timeout time.Duration
	packetRate uint32
}

var messageCounter uint8

// General TODO : Check error handling at each step. For now nothing happens
// when the server sends an error for example.

// RequestFile connects to the server at address:port and tries to perform a
// complete SANFT exchange to request the file identified by URI. If the
// transfer works, the requested file will be written to localFilename.
func RequestFile(address string, port int, URI string, localFilename string) error {
    conn, err := messages.CreateClientSocket(address, port)
	if err != nil {
		return fmt.Errorf("Create client socket: %w", err)
	}
	defer conn.Close()

	// TODO: make retransmissions and timeout configurable.
	retransmissions := 5
	timeout := 3*time.Second

	// Request file metadata
	metadata, err := getMetadata(conn, URI, retransmissions, timeout)
	if err != nil {
		return fmt.Errorf("get metadata: %w", err)
	}

	localFile, err := os.Create(localFilename)
	defer localFile.Close()
	if err != nil {
		return fmt.Errorf("Open file %s: %w", localFilename, err)
	}
	// Request chunks
	for metadata.firstMissing <= metadata.fileSize {
		// TODO congestion control
		err := getMissingChunks(conn, metadata, localFile)
		if err != nil {
			// TODO error handling
		}
	}

	checksum, err := computeChecksum(localFilename)
	if err != nil {
		return fmt.Errorf("Compute checksum of %s: %w", localFilename, err)
	}
	if checksum != metadata.checksum {
		// TODO: Delete file (and retry ?)
		return fmt.Errorf("Checksum not matching. Expected %x got %x", metadata.checksum, checksum)
	}

	return nil
}

// Send a MetaData Request to the server and parse the response to populate
// a fileMetaData struct
//
// For a chance at a successful request, retransmissions needs to be larger
// than 1.
func getMetadata(conn *net.UDPConn, URL string, retransmissions int, timeout time.Duration) (*fileMetadata, error) {
	var buf []byte
	metadata := new(fileMetadata)
	// Send a Metadatarequest with a zero token
	// It will probably (certainly) be invalid but in that case just take the 
	mdr := messages.GetMDR(messageCounter, &metadata.token, URL)

	retransmit:
	for i:=0; i<retransmissions; i++{
		mdr.Header.Number = messageCounter
		messageCounter ++
		t_send := time.Now()
		err := mdr.Send(conn)
		if err != nil {
			// TODO : differentiate between multiple errors
			// I assumed broken pipe, thus the exit but there might be other ones
			return nil, fmt.Errorf("Send MDR with invalid token: %w", err)
		}
		deadline := t_send.Add(timeout)
		err = conn.SetReadDeadline(deadline)
		if err != nil {
			return nil, fmt.Errorf("Set deadline: %w", err)
		}

		receive:
		for time.Now().Before(deadline) {
			_, _, err := conn.ReadFromUDP(buf)
			if err != nil {
				if errors.Is(err, os.ErrDeadlineExceeded) {
					// If it's a timeout retransmit
					continue retransmit
				} else {
					// Otherwise ? IDK 
					// TODO: Add error handling for other errors
					return nil, fmt.Errorf("Read from UDP: %w", err)
				}
			}
			response, err := messages.ParseServer(&buf)
			if err != nil {
				// This either means that:
				//	- the message has an invalid version; 
				//	- the message type is not recognised;
				//	- the message has not the right format for its type;
				//		- Either because its plain invalid;
				//	    - Or because there is an error from the server;
				// - or binary.Read fails for some other reason
				// TODO: Investigate on which it is and act accordingly
				fmt.Printf("Error while parsing response: %v\nResponse:%x\n", err, buf);
				continue receive
			}
			switch response.(type) {
			case messages.NTM:
				ntm := response.(messages.NTM)
				metadata.token = ntm.Token
				// Note: even if this is expected in the protocol, this still counts as one retransmission
				continue retransmit
			case messages.MDRR:
				mdrr := response.(messages.MDRR)
				if mdrr.Header.Number != mdr.Header.Number {
					// This message is not for us. Ignore it
					continue receive
				}
				// Get metadata from the response
				metadata.chunkSize = mdrr.ChunkSize
				metadata.maxChunksInACR = mdrr.MaxChunksInACR
				metadata.fileID = mdrr.FileID
				metadata.fileSize = binary.LittleEndian.Uint64(mdrr.FileSize[:])
				metadata.checksum = mdrr.Checksum

				// Get metadata from RTT
				// TODO: Decide on multiplier for timeout or make it configurable
				metadata.timeout = time.Since(t_send) * 3
				// TODO: Decide on initial packet rate or make it configurable
				metadata.packetRate = 1

				// metadata.chunkMap and metadata.firstMissing are already set to 0
				return metadata, nil
			default:
				continue receive
			}
		}

	}
	return nil, fmt.Errorf("Could not get metadata from server")
}

// Sends one ACR to get missing chunks.
// This function also receives the CRRs, write them to localFile, update the
// chunkMap and perform packet rate measurements.
func getMissingChunks(conn *net.UDPConn, metadata *fileMetadata, localFile *os.File) error {
	// TODO Send File chunk request for missing chunks
	// TODO Receive chunks and write them to memory
	// TODO update measured rate and timeout
	return nil
}

func computeChecksum(filename string) ([32]byte, error) {
	var hash [32]byte
	data, err := os.ReadFile(filename)
	if err != nil {
		return hash, fmt.Errorf("Compute checksum: %w", err)
	}
	h := sha256.New()
	h.Write(data)
	copy(hash[:], h.Sum(nil))

	return hash, nil
}
