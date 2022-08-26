package client

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"math"
	"net"
	"os"
	"time"

	"gitlab.lrz.de/protocol-design-sose-2022-team-0/sanft/messages"
)

type fileMetadata struct {
	// Metadata from the metadata request

	token          [32]uint8
	chunkSize      uint16
	maxChunksInACR uint16
	fileID         uint32
	fileSize       uint64
	checksum       [32]byte
	url            string

	// Information on missing chunks

	chunkMap     map[uint64]bool //chunkMap[chunk] == true iff chunk has been received
	firstMissing uint64          // The index of the first chunk not yet received

	// Information on the connection

	timeout        time.Duration
	packetRate     uint32
	messageCounter uint8
}

// TODO[or not]: make this configurable
const (
	originalTimeout    time.Duration = 3*time.Second
	retransmissionsMDR int = 5 // Max number of retransmissions when sending an MDR
	rtt2timeoutFactor  int = 2 // timeout = rtt2timeoutFactor * rtt
	initialPacketRate  uint32 = 2
	nCRRsToWait        int = 3 // Number of virtual CRR to wait for the next CRR to arrive
)

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

	// Request file metadata
	metadata, err := getMetadata(conn, URI, retransmissionsMDR, originalTimeout)
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
		err := getMissingChunks(conn, metadata, localFile)
		if err != nil {
			return fmt.Errorf("get missing chunks: %w", err)
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
	var messageCounter uint8
	// Send a Metadatarequest with a zero token
	// It will probably (certainly) be invalid but in that case just take the
	mdr := messages.GetMDR(messageCounter, &metadata.token, URL)

retransmit:
	for i := 0; i < retransmissions; i++ {
		mdr.Header.Number = messageCounter
		messageCounter++
		t_send := time.Now()
		err := mdr.Send(conn)
		if err != nil {
			if os.IsTimeout(err) {
				continue retransmit
			}
			// If this is not a timeout, it's safer to exit to see what happened
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
					// If it's a timeout, retransmit
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
				// - or binary.Read fails for some other reason
				// TODO: Investigate on which it is and act accordingly
				if errors.Is(err, new(messages.UnsupporedVersionError)) {
					// We can't do anything if we don't speak the same language
					return nil, fmt.Errorf("parse server message: %w", err)
				} else if errors.Is(err, new(messages.UnsupporedTypeError)) ||
				errors.Is(err, new(messages.WrongPacketLengthError)) {
					// TODO
				}
				fmt.Printf("Unknown error while parsing response: %v\nResponse:%x\nContinuing...\n", err, buf)
				continue receive
			}
			switch response.(type) {
			case messages.ServerHeader:
				header := response.(messages.ServerHeader)
				if header.Type != messages.MDRR_t {
					// Ignore it
					continue receive
				}
				if header.Number != mdr.Header.Number {
					// Not for us. Ignore it
					continue receive
				}

				switch header.Error {
					case messages.UnsupportedVersion:
						// The server must have set its version number to the
						// version it supports.
						// If we're here, we didn't get an UnsupporedVersionError
						// by the parser, so it means it's a version we support.
						// This is an answer to a message we sent. We only
						// support one version. Something's not right -> Error
						return nil, fmt.Errorf("MDRR server error: the server doesn't support our protocol version (%d) and answered with version %d", messages.VERS, header.Version)
					case messages.FileNotFound:
						return nil, fmt.Errorf("MDRR server error: File not found on server")
					default:
						return nil, fmt.Errorf("MDRR server error: Unknown error code for MDRR %d", header.Error)
				}
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
				metadata.url = URL
				metadata.chunkSize = mdrr.ChunkSize
				metadata.maxChunksInACR = mdrr.MaxChunksInACR
				metadata.fileID = mdrr.FileID
				metadata.fileSize = messages.Uint8_6_arr2int(&mdrr.FileSize)
				metadata.checksum = mdrr.Checksum

				// Get metadata from RTT
				metadata.timeout = time.Since(t_send) * time.Duration(rtt2timeoutFactor) // Sorry to all the physicists who will see this; go only accepts to multiply values of the same type
				metadata.packetRate = initialPacketRate
				metadata.messageCounter = messageCounter

				metadata.chunkMap = make(map[uint64]bool, metadata.fileSize)
				return metadata, nil
			default:
				continue receive
			}
		}

	}
	return nil, fmt.Errorf("Could not get metadata from server after %d retransmissions", retransmissionsMDR)
}

// Sends one ACR to get missing chunks.
// This function also receives the CRRs, write them to localFile, update the
// chunkMap and perform packet rate measurements.
func getMissingChunks(conn *net.UDPConn, metadata *fileMetadata, localFile *os.File) error {
	var buf []byte
	// Build an ACR and send it
	acr, requested := buildACR(metadata)
	n_cr := len(requested)
	t_send := time.Now()
	err := acr.Send(conn)
	if err != nil {
		return fmt.Errorf("Send ACR: %w", err)
	}
	deadline := t_send.Add(metadata.timeout)
	mapTimeCRRs := make(map[int]time.Time)
	received := false
	for time.Now().Before(deadline) {
		err = conn.SetReadDeadline(deadline)
		if err != nil {
			return fmt.Errorf("Set deadline: %w", err)
		}
		_, _, err := conn.ReadFromUDP(buf)
		t_recv := time.Now()
		if err != nil {
			if errors.Is(err, os.ErrDeadlineExceeded) {
				// If it's a timeout, continue in case the deadline was extended
				continue
			} else {
				// TODO: Add error handling for other errors
				return fmt.Errorf("Read from UDP: %w", err)
			}
		}
		response, err := messages.ParseServer(&buf)
		if err != nil {
			// TODO: Error handling
			fmt.Printf("Error while parsing response: %v\nResponse:%x\n", err, buf)
			continue
		}
		switch response.(type) {
		case messages.ServerHeader:
			header := response.(messages.ServerHeader)
			if header.Number != acr.Header.Number {
				// Ignore it
				continue
			}
			switch header.Error {
			case messages.UnsupportedVersion:
				return fmt.Errorf("CRR server error: the server doesn't support our protocol version (%d) and answered with version %d", messages.VERS, header.Version)
			case messages.InvalidFileID:
				// Request new metadata and update it
				newMetadata, err := getMetadata(conn, metadata.url, retransmissionsMDR, metadata.timeout)
				if err != nil {
					return fmt.Errorf("get metadata after invalid fileID: %w", err)
				}
				// Let's keep the old packetRate
				oldPacketRate := metadata.packetRate
				// TODO: Does this actually replace metadata with newMetadata ?
				metadata = newMetadata
				metadata.packetRate = oldPacketRate
			case messages.TooManyChunks:
				// Let's check
				if n_cr > int(metadata.maxChunksInACR) {
					// Our mistake... Let's throw an error to investigate
					return fmt.Errorf("malformed ACR: we requested %d chunks in an ACR. Max is %d. (%v)", n_cr, metadata.maxChunksInACR, acr)
				} else {
					// TODO: Investigate (with a MDR) or ignore
					continue
				}
			case messages.ZeroLengthCR:
				// Let's check
				for _, cr := range acr.CRs {
					if cr.Length == 0 {
						// Our mistake
						return fmt.Errorf("malformed ACR: we sent a CR with length 0. (%v)", acr)
					}
				}
				// TODO: Investigate (with a MDR) or ignore
				continue
			default:
				return fmt.Errorf("CRR server error: Unknown error code for CRR %d", header.Error)
			}
		case messages.NTM:
			ntm := response.(messages.NTM)
			if ntm.Token != metadata.token {
				metadata.token = ntm.Token
				return nil // We shouldn't receive any further chunks if we used a wrong token.
				// Should it be an error, though ?
			}
		case messages.CRR:
			crr := response.(messages.CRR)
			if crr.Header.Number != acr.Header.Number {
				// This message is not for us. Ignore it
				continue
			}
			chunkNumber := messages.Uint8_6_arr2int(&crr.ChunkNumber)
			// Let's find its position in the ACR
			chunkIndexInACR := -1
			for i, cn := range requested {
				if cn == chunkNumber {
					chunkIndexInACR = i
					break
				}
			}
			if chunkIndexInACR == -1 {
				// This is not a chunk we requested. Ignore it.
				continue
			}
			// We need to check that there is no error
			if crr.Header.Error != messages.NoError {
				if crr.Header.Error != messages.ChunkOutOfBounds {
					return fmt.Errorf("CRR server error: Unexpected Error for CRR with content: %d", crr.Header.Error)
				}
				// We requested a chunk out of bound. Strange...
				// Let's do a few checks
				if chunkNumber >= metadata.fileSize {
					// That's our fault. Let's throw an error to investigate
					return fmt.Errorf("malformed ACR: we requested chunk #%d for a file of size %d (%v)", chunkNumber, metadata.fileSize, acr)
				} else {
					// TODO: Investigate (with a MDR) or ignore
					continue
				}
			}
			received = true
			mapTimeCRRs[chunkIndexInACR] = t_recv
			deadline = t_recv.Add(time.Duration(uint64(nCRRsToWait)*uint64(time.Second)/uint64(metadata.packetRate)))

			err = writeChunkToFile(metadata, chunkNumber, crr.Data, localFile)
			if err != nil {
				// TODO: error handling
				continue
			}
		default:
			// Ignore irrelevant messages
			continue
		}
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
			offset++
		}
	}
	// Create the ACR
	acr = messages.GetACR(metadata.messageCounter, &metadata.token, metadata.fileID, metadata.packetRate, &chunkRequests)
	metadata.messageCounter++
	return
}

func writeChunkToFile(metadata *fileMetadata, chunkNumber uint64, data []byte, file *os.File) error {
	if !metadata.chunkMap[chunkNumber] {
		if len(data) != int(metadata.chunkSize) {
			return fmt.Errorf("Invalid chunk size. Expected %d got %d", metadata.chunkSize, len(data))
		}
		offset := int64(chunkNumber) * int64(metadata.chunkSize)
		_, err := file.WriteAt(data, offset)
		if err != nil {
			return fmt.Errorf("write data at offset: %w", err)
		}
		// Update chunkMap and first Missing
		metadata.chunkMap[chunkNumber] = true
		for metadata.chunkMap[metadata.firstMissing] {
			metadata.firstMissing++
		}
	}
	return nil
}

// computePacketRate computes a new packet rate from:
//   - timeReceiveCRR: a map that contains the time each CRR was received at.
//     They are indexed by their position in the ACR that caused the response.
//   - n_expected: Number of expected packets
//   - packetRate: the packetRate field of the ACR that caused the response.
func computePacketRate(timeReceiveCRR map[int]time.Time, n_expected int, packetRate uint32) (uint32, error) {
	if packetRate == 0 {
		return 0, fmt.Errorf("packetRate must be positive")
	}
	if n_expected <= 1 {
		return 0, fmt.Errorf("Can only compute a packetRate if more than 2 packets were expected")
	}
	var timeFirst, timeLast time.Time
	indexFirst := -1
	indexLast := -1
	n_received := 0
	for i := 0; i < n_expected; i++ {
		when, received := timeReceiveCRR[i]
		if received {
			n_received++
			if indexFirst == -1 {
				timeFirst = when
				indexFirst = i
			}
			timeLast = when
			indexLast = i
		}
	}
	if n_received == 0 {
		return 0, fmt.Errorf("Cannot measure a rate when no packet was received")
	}

	var estimatedFirst, estimatedLast time.Time
	if indexFirst == 0 {
		estimatedFirst = timeFirst
	} else {
		estimatedFirst = timeFirst.Add(-time.Duration(uint64(indexFirst) * uint64(time.Second) / uint64(packetRate)))
	}
	if indexLast == n_expected-1 {
		estimatedLast = timeLast
	} else {
		estimatedLast = timeLast.Add(time.Duration(uint64(n_expected-1-indexLast) * uint64(time.Second) / uint64(packetRate)))
	}
	if !estimatedLast.After(estimatedFirst) {
		// We just return an the previous rate multiplied by the ratio of received packets
		newPacketRate := packetRate * uint32(n_received) / uint32(n_expected)
		if newPacketRate > 0 {
			return newPacketRate, nil
		} else {
			return 1, nil
		}
	}
	measuredRate := float64(n_received) / estimatedLast.Sub(estimatedFirst).Abs().Seconds()
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
