package client

import (
	"errors"
	"crypto/sha256"
	"fmt"
	"net"
	"os"
	"time"
	"math"

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

	chunkMap map[uint64]bool //chunkMap[chunk] == true iff chunk has been received
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
			return nil, fmt.Errorf("Send MDR: %w", err)
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
				metadata.fileSize = messages.Uint8_6_arr2int(&mdrr.FileSize)
				metadata.checksum = mdrr.Checksum

				// Get metadata from RTT
				// TODO: Decide on multiplier for timeout or make it configurable
				metadata.timeout = time.Since(t_send) * 3
				// TODO: Decide on initial packet rate or make it configurable
				metadata.packetRate = 1

				metadata.chunkMap = make(map[uint64]bool, metadata.fileSize)
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
	// Build an ACR and send it
	acr, requested := buildACR(metadata)
	n_cr := len(requested)
	t_send := time.Now()
	err := acr.Send(conn)
	if err != nil {
		return fmt.Errorf("Send ACR: %w", err)
	}
	deadline := t_send.Add(metadata.timeout)
	err = conn.SetReadDeadline(deadline)
	if err != nil {
		return fmt.Errorf("Set deadline: %w", err)
	}
	mapTimeCRRs := make(map[int]time.Time)
	received := false
	for time.Now().Before(deadline) {
		// TODO Receive chunks and write them to memory
	}
	if received {
		newPacketRate, err := computePacketRate(mapTimeCRRs, n_cr, metadata.packetRate)
		if err != nil {
			return fmt.Errorf("compute new packetRate: %w", err)
		}
		metadata.packetRate = newPacketRate
	} else {
		// Exponential backoff
		metadata.timeout *= 2
	}
	return nil
}

// Build an ACR to request the missing chunks according to metadata.chunkMap
func buildACR(metadata *fileMetadata) (acr *messages.ACR, requested []uint64) {
	chunksInACR := 0
	requested = []uint64{}
	chunkRequests := []messages.CR{}
	offset := metadata.firstMissing
	for chunksInACR < int(metadata.maxChunksInACR) && offset < metadata.fileSize {
		requested = append(requested, offset)
		length := 1
		// Find longest length of missing chunks starting from offset 
		for uint64(length)+offset < metadata.fileSize &&
		length < int(metadata.maxChunksInACR)-chunksInACR &&
		length < 255 &&
		!metadata.chunkMap[uint64(length)+offset] {
			requested = append(requested, uint64(length)+offset)
			length++
		}
		// Create the corresponding CR
		chunkRequests = append(chunkRequests, *messages.GetCR(*messages.Int2uint8_6_arr(offset), uint8(length)))
		chunksInACR += length
		// Find the next offset of a missing chunk
		offset += uint64(length)
		for metadata.chunkMap[offset] {
			offset ++
		}
	}
	// Create the ACR
	acr = messages.GetACR(messageCounter, &metadata.token, metadata.fileID, metadata.packetRate, &chunkRequests)
	messageCounter ++
	return
}

//computePacketRate computes a new packet rate from:
//	- timeReceiveCRR: a map that contains the time each CRR was received at.
//	They are indexed by their position in the ACR that caused the response.
//	- n_expected: Number of expected packets
//	- packetRate: the packetRate field of the ACR that caused the response.
func computePacketRate(timeReceiveCRR map[int]time.Time, n_expected int, packetRate uint32) (uint32, error) {
	if packetRate == 0 {
		return 0, fmt.Errorf("packetRate must be positive")
	}
	if n_expected <= 1 {
		return 0, fmt.Errorf("Can only compute a packetRate if more than 2 packets were expected")
	}
	var timeFirst, timeLast time.Time
	var indexFirst, indexLast int
	n_received := 0
	for i:=0; i<n_expected; i++ {
		when, received := timeReceiveCRR[i]
		if received {
			n_received ++
			if timeFirst.IsZero() || when.Before(timeFirst) {
				timeFirst = when
				indexFirst = i
			}
			if timeLast.IsZero() || when.After(timeLast) {
				timeLast = when
				indexLast = i
			}
		}
	}
	if n_received == 0 {
		return 0, fmt.Errorf("Cannot measure a rate when no packet was received")
	}
	if n_received == 1 {
		// We cannot compute a packet Rate if only one packet was received
		// For now I'm just dividing the old packetRate by the number of expected
		// packets (multiplicative decrease).
		newPacketRate := packetRate/uint32(n_expected)
		if newPacketRate >= 1 {
			return newPacketRate, nil
		} else {
			return 1, nil
		}
	}

	var estimatedFirst, estimatedLast time.Time
	if indexFirst == 0 {
		estimatedFirst = timeFirst
	} else {
		estimatedFirst = timeFirst.Add(-time.Duration(uint64(indexFirst)*uint64(time.Second)/uint64(packetRate)))
	}
	if indexLast == n_expected - 1 {
		estimatedLast = timeLast
	} else {
		estimatedLast = timeLast.Add(time.Duration(uint64(n_expected-1 - indexLast)*uint64(time.Second)/uint64(packetRate)))
	}
	// Sanity check
	if !estimatedLast.After(estimatedFirst) {
		return 0, fmt.Errorf("estimatedLast(%s) should be after estimatedFirst(%s).", estimatedLast.Format(time.StampMicro), estimatedFirst.Format(time.StampMicro))
	}
	measuredRate := float64(n_received-1)/estimatedLast.Sub(estimatedFirst).Abs().Seconds()
	// Do some sanity checks to avoid an overflow
	if measuredRate > float64(math.MaxUint32) {
		// It's probably not a good idea to request a packetRate that large but 
		// this function doesn't have such considerations ¯\_(ツ)_/¯
		return math.MaxUint32, nil
	}
	if measuredRate < 1.0 {
		return 1, nil
	}
	if math.IsNaN(measuredRate) {
		return 0, fmt.Errorf("Measured rate is NaN.")
	}
	return uint32(measuredRate), nil
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
