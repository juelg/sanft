package client

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"log"
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

	chunkMap       map[uint64]bool //chunkMap[chunk] == true iff chunk has been received
	firstMissing   uint64          // The index of the first chunk not yet received

	// Information on the connection

	timeout        time.Duration
	packetRate     uint32
	messageCounter uint8

	// Local file pointer

	localFile      *os.File
}

// TODO[or not]: make this configurable
const (
	initialTimeout    time.Duration = 3*time.Second
	retransmissionsMDR int = 5 // Max number of retransmissions when sending an MDR
	rtt2timeoutFactor  int = 2 // timeout = rtt2timeoutFactor * rtt
	initialPacketRate  uint32 = 10
	nCRRsToWait        int = 3 // Number of virtual CRR to wait for the next CRR to arrive
)

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
	metadata := new(fileMetadata)
	metadata.url = URI
	metadata.timeout = initialTimeout
	metadata.packetRate = initialPacketRate
	err = updateMetadata(conn, metadata)
	if err != nil {
		return fmt.Errorf("get metadata: %w", err)
	}

	localFile, err := os.Create(localFilename)
	if err != nil {
		return fmt.Errorf("Open file %s: %w", localFilename, err)
	}
	metadata.localFile = localFile
	// Request chunks
	for metadata.firstMissing < metadata.fileSize {
		err := getMissingChunks(conn, metadata)
		if err != nil {
			localFile.Close()
			return fmt.Errorf("get missing chunks: %w", err)
		}
	}
	localFile.Close()

	checksum, err := computeChecksum(localFilename)
	if err != nil {
		return fmt.Errorf("Compute checksum of %s: %w", localFilename, err)
	}
	if checksum != metadata.checksum {
		os.Remove(localFilename)
		return fmt.Errorf("Checksum not matching. Expected %x got %x", metadata.checksum, checksum)
	}

	return nil
}

// updateMetadata sends a MetaData Request to the server and parses the response
// to update metadata.
func updateMetadata(conn *net.UDPConn, metadata *fileMetadata) error {
	buf := make([]byte, 0x10000) // 64kB
	if metadata.timeout == 0 {
		return errors.New("metadata.timeout cannot be 0.")
	}
retransmit:
	for i := 0; i < retransmissionsMDR; i++ {
		mdr := messages.GetMDR(metadata.messageCounter, &metadata.token, metadata.url)
		metadata.messageCounter++
		t_send := time.Now()
		err := mdr.Send(conn)
		if err != nil {
			if os.IsTimeout(err) {
				continue retransmit
			}
			// If this is not a timeout, it's safer to exit to see what happened
			return fmt.Errorf("Send MDR: %w", err)
		}
		deadline := t_send.Add(metadata.timeout)
		err = conn.SetReadDeadline(deadline)
		if err != nil {
			return fmt.Errorf("Set deadline: %w", err)
		}

	receive:
		for time.Now().Before(deadline) {
			n, _, err := conn.ReadFromUDP(buf)
			if err != nil {
				if errors.Is(err, os.ErrDeadlineExceeded) {
					// If it's a timeout, retransmit
					continue retransmit
				} else {
					// Otherwise, let's see what happened
					return fmt.Errorf("Read from UDP: %w", err)
				}
			}
			raw := buf[:n]
			response, err := messages.ParseServer(&raw)
			if err != nil {
				if errors.Is(err, new(messages.UnsupporedVersionError)) {
					// We can't do anything if we don't speak the same language
					return fmt.Errorf("parse server message: %w", err)
				} else if errors.Is(err, new(messages.UnsupporedTypeError)) ||
				errors.Is(err, new(messages.WrongPacketLengthError)) {
					// Ignore unknown messages, but still log the error
					log.Printf("WARN: Invalid response received: %v. Dropped response:%x\n", err, raw)
					continue receive
				}
				log.Printf("WARN: Unknown error while parsing response: %v. Dropped response:%x\n", err, raw)
				continue receive
			}
			switch response.(type) {
			case messages.ServerHeader:
				header := response.(messages.ServerHeader)
				if header.Type != messages.MDRR_t {
					// Ignore it
					log.Printf("WARN: Unexpected server header of type %d. Dropped response: %v\n", header.Type, header)
					continue receive
				}
				if header.Number != mdr.Header.Number {
					// Not for us. Ignore it
					log.Printf("INFO: Received response with wrong message number(%d instead of %d). Dropped\n", header.Number, mdr.Header.Number)
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
						return fmt.Errorf("MDRR server error: the server doesn't support our protocol version (%d) and answered with version %d", mdr.Header.Version, header.Version)
					case messages.FileNotFound:
						return fmt.Errorf("MDRR server error: File not found on server")
					default:
						return fmt.Errorf("MDRR server error: Unknown error code for MDRR %d", header.Error)
				}
			case messages.NTM:
				ntm := response.(messages.NTM)
				metadata.token = ntm.Token
				// Note: even if this is expected in the protocol, this still counts as one retransmission
				log.Printf("INFO: Updated token to %x (from %x). Retransmitting...\n", ntm.Token, mdr.Header.Token)
				continue retransmit
			case messages.MDRR:
				mdrr := response.(messages.MDRR)
				if mdrr.Header.Number != mdr.Header.Number {
					// This message is not for us. Ignore it
					log.Printf("INFO: Received response with wrong message number(%d instead of %d). Dropped\n", mdrr.Header.Number, mdr.Header.Number)
					continue receive
				}
				// Update metadata
				oldFileID := metadata.fileID
				getMetadataFromMDRR(metadata, &mdrr)
				metadata.timeout = time.Since(t_send) * time.Duration(rtt2timeoutFactor) // Sorry to all the physicists who will see this; go only accepts to multiply values of the same type

				if metadata.chunkMap == nil || metadata.fileID != oldFileID {
					// Erase the old file
					if metadata.localFile != nil {
						err := metadata.localFile.Truncate(0)
						if err != nil {
							return fmt.Errorf("erase file content of %s: %w", metadata.localFile.Name(), err)
						}
					}
					metadata.chunkMap = make(map[uint64]bool, metadata.fileSize)
					metadata.firstMissing = 0
				}
				return nil
			default:
				log.Printf("WARN: Received unexpected response of type %T. Dropped\n", response)
				continue receive
			}
		}

	}
	return fmt.Errorf("Could not get metadata from server after %d retransmissions", retransmissionsMDR)
}

// getMetadataFromMDRR overwrites relevant field in metadata with fields from
// the MDRR.
func getMetadataFromMDRR(metadata *fileMetadata, mdrr *messages.MDRR) {
	metadata.chunkSize = mdrr.ChunkSize
	metadata.maxChunksInACR = mdrr.MaxChunksInACR
	metadata.fileID = mdrr.FileID
	metadata.fileSize = messages.Uint8_6_arr2int(&mdrr.FileSize)
	metadata.checksum = mdrr.Checksum
}

// Sends one ACR to get missing chunks.
// This function also receives the CRRs, write them to localFile, update the
// chunkMap and perform packet rate measurements.
func getMissingChunks(conn *net.UDPConn, metadata *fileMetadata) error {
	buf := make([]byte, 0x10000) // 64kB
	// Build an ACR and send it
	acr, requested := buildACR(metadata)
	log.Printf("INFO: Requesting chunks %v\n", requested)
	n_cr := len(requested)
	if n_cr == 0 {
		return fmt.Errorf("No missing chunks.%v", metadata)
	}
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
		n, _, err := conn.ReadFromUDP(buf)
		t_recv := time.Now()
		if err != nil {
			if os.IsTimeout(err) {
				// If it's a timeout, continue in case the deadline was extended
				continue
			} else {
				// Return to see what the error was
				return fmt.Errorf("Read from UDP: %w", err)
			}
		}
		raw := buf[:n]
		response, err := messages.ParseServer(&raw)
		if err != nil {
			if errors.Is(err, new(messages.UnsupporedVersionError)) {
				// We can't do anything if we don't speak the same language
				return fmt.Errorf("parse server message: %w", err)
			} else if errors.Is(err, new(messages.UnsupporedTypeError)) ||
			errors.Is(err, new(messages.WrongPacketLengthError)) {
				// Ignore unknown messages, but still log the error
				log.Printf("WARN: Invalid response received: %v. Dropped response:%x\n", err, raw)
				continue
			}
			log.Printf("WARN: Unknown error while parsing response: %v. Dropped response:%x\n", err, raw)
			continue
		}
		switch response.(type) {
		case messages.ServerHeader:
			header := response.(messages.ServerHeader)
			if header.Number != acr.Header.Number {
				// Ignore it
				log.Printf("INFO: Received response with wrong message number(%d instead of %d). Dropped\n", header.Number, acr.Header.Number)
				continue
			}
			switch header.Error {
			case messages.UnsupportedVersion:
				return fmt.Errorf("CRR server error: the server doesn't support our protocol version (%d) and answered with version %d", messages.VERS, header.Version)
			case messages.InvalidFileID:
				// Request new metadata and update it
				oldFileID := metadata.fileID
				err := updateMetadata(conn, metadata)
				log.Printf("INFO: Updated metadata. Old fileID:%x, new fileID:%x\n", oldFileID, metadata.fileID)
				if err != nil {
					return fmt.Errorf("get metadata after invalid fileID: %w", err)
				}
			case messages.TooManyChunks:
				// Let's check
				if n_cr > int(metadata.maxChunksInACR) {
					// Our mistake... Let's throw an error to investigate
					return fmt.Errorf("malformed ACR: we requested %d chunks in an ACR. Max is %d. (%v)", n_cr, metadata.maxChunksInACR, acr)
				} else {
					log.Printf("WARN: Received TooManyChunks error for ACR %v. MaxChunksInACR=%d; # of chunks in ACR:%d. Ignored...\n", acr, metadata.maxChunksInACR, n_cr)
					// TODO: Send a MDR to update metadata
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
				log.Printf("WARN: Received ZeroLengthCR error for ACR %v. Ignored.\n", acr)
				continue
			default:
				return fmt.Errorf("CRR server error: Unknown error code for CRR %d", header.Error)
			}
		case messages.NTM:
			ntm := response.(messages.NTM)
			if ntm.Token != metadata.token {
				metadata.token = ntm.Token
				log.Printf("INFO: Updated token to %x (from %x). Retransmitting...\n", ntm.Token, acr.Header.Token)
				return nil // We shouldn't receive any further chunks if we used a wrong token.
				// Should it be an error, though ? No, we just want to resume normally
			}
		case messages.CRR:
			crr := response.(messages.CRR)
			if crr.Header.Number != acr.Header.Number {
				// This message is not for us. Ignore it
				log.Printf("INFO: Received response with wrong message number(%d instead of %d). Dropped\n", crr.Header.Number, acr.Header.Number)
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
				log.Printf("WARN: Received CRR for unrequested chunk %d. (The requested chunks are %v). Dropped.\n", chunkNumber, requested)
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
					log.Printf("WARN: Received ChunkOutOfBounds error for chunk #%d in file of size %d. (%v)\n", chunkNumber, metadata.fileSize, acr)
					// TODO: Send new MDR to update metadata just in case
					continue
				}
			}
			received = true
			mapTimeCRRs[chunkIndexInACR] = t_recv
			deadline = t_recv.Add(time.Duration(nCRRsToWait+n_cr-chunkIndexInACR)*time.Second/time.Duration(metadata.packetRate))

			err = writeChunkToFile(metadata, chunkNumber, crr.Data, metadata.localFile)
			if err != nil {
				log.Printf("WARN: Could not write chunk #%d to file %s : %v", chunkNumber, metadata.localFile.Name(), err)
				continue
			}
		default:
			// Ignore irrelevant messages
			log.Printf("WARN: Received unexpected response type to ACR: %T. Dropped...\n", response)
			continue
		}
	}
	if received {
		if n_cr > 1 {
			newPacketRate, err := computePacketRate(mapTimeCRRs, n_cr, metadata.packetRate)
			if err != nil {
				return fmt.Errorf("compute new packetRate: %w", err)
			}
			log.Printf("INFO: Updated packetRate: before:%d, now:%d", metadata.packetRate, newPacketRate)
			metadata.packetRate = newPacketRate
		}
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
		if chunkNumber != metadata.fileSize - 1 && len(data) != int(metadata.chunkSize) {
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
