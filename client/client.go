package client

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"math"
	"net"
	"os"
	"time"

	"gitlab.lrz.de/protocol-design-sose-2022-team-0/sanft/markov"
	"gitlab.lrz.de/protocol-design-sose-2022-team-0/sanft/messages"
)

type ClientConfig struct {
	// Implementation specific values for SANFT
	RetransmissionsMDR int    // Max number of retransmissions when sending an MDR
	InitialPacketRate  uint32 // Initial packet rate in pkt/s
	NCRRsToWait        int    // Number of virtual CRR to wait for the next CRR to arrive

	// Markov simulation of packet loss
	MarkovP float64 // Probability of losing packet n+1 if n was not lost
	MarkovQ float64 // Probability of losing packet n+1 if n was lost

	// Debugging
	DebugLogger *log.Logger
	InfoLogger  *log.Logger
	WarnLogger  *log.Logger
}

var DefaultConfig = ClientConfig{
	RetransmissionsMDR: 5,
	InitialPacketRate:  30,
	NCRRsToWait:        3,
	MarkovP:            0,
	MarkovQ:            0,
	DebugLogger:        log.New(ioutil.Discard, "DEBUG: ", log.LstdFlags),
	InfoLogger:         log.New(os.Stderr, "INFO: ", log.LstdFlags),
	WarnLogger:         log.New(os.Stderr, "WARN: ", log.LstdFlags),
}

type transferStats struct {
	requested int // Number of requested chunks
	received  int // Number of received chunks
	invalid   int // Number of invalid messages
	late      int // Number of messages with wrong message number
}

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
	stats          transferStats

	// Local file pointer

	localFile *os.File
}

// These are fixed by the specification
const (
	initialTimeout    time.Duration = 3 * time.Second
	rtt2timeoutFactor int           = 2 // timeout = rtt2timeoutFactor * rtt
	maxChunkSize      uint16        = 65517
)

// RequestFile connects to the server at address:port and tries to perform a
// complete SANFT exchange to request the file identified by URI. If the
// transfer works, the requested file will be written to localFilename.
func RequestFile(ip net.IP, port int, URI string, localFilename string, conf *ClientConfig) error {
	var err error
	err = checkConfig(conf)
	if err != nil {
		return fmt.Errorf("invalid configuration: %w", err)
	}
	conn, err := markov.CreateClientSocket(ip, port, conf.MarkovP, conf.MarkovQ)
	if err != nil {
		return fmt.Errorf("create client socket: %w", err)
	}
	defer conn.Close()

	// Request file metadata
	metadata := new(fileMetadata)
	metadata.url = URI
	metadata.timeout = initialTimeout
	metadata.packetRate = conf.InitialPacketRate
	err = updateMetadata(conn, metadata, conf)
	if err != nil {
		return fmt.Errorf("get metadata: %w", err)
	}

	localFile, err := os.Create(localFilename)
	if err != nil {
		return fmt.Errorf("open file %s: %w", localFilename, err)
	}
	metadata.localFile = localFile
	// Request chunks
	fmt.Printf("%s(0x%x): %d/%d chunks (%dchunks/s)\r", metadata.url, metadata.fileID, metadata.stats.received, metadata.fileSize, metadata.packetRate)
	for metadata.firstMissing < metadata.fileSize {
		err := getMissingChunks(conn, metadata, conf)
		if err != nil {
			localFile.Close()
			os.Remove(localFilename)
			return fmt.Errorf("get missing chunks: %w", err)
		}
		fmt.Printf("%s(0x%x): %d/%d chunks (%dchunks/s); req:%d;invalid:%d;late:%d\r", metadata.url, metadata.fileID, metadata.stats.received, metadata.fileSize, metadata.packetRate, metadata.stats.requested, metadata.stats.invalid, metadata.stats.late)
	}
	fmt.Println()
	localFile.Close()

	checksum, err := computeChecksum(localFilename)
	if err != nil {
		return fmt.Errorf("compute checksum of %s: %w", localFilename, err)
	}
	if checksum != metadata.checksum {
		os.Remove(localFilename)
		return fmt.Errorf("checksum not matching. Expected %x got %x", metadata.checksum, checksum)
	}

	return nil
}

func checkConfig(conf *ClientConfig) error {
	if conf.InitialPacketRate == 0 {
		return errors.New("initialPacketRate cannot be 0")
	}
	if conf.RetransmissionsMDR < 2 {
		return errors.New("retransmissionsMDR must be at least 2")
	}
	if conf.NCRRsToWait < 3 {
		conf.WarnLogger.Printf("nCRRsToWait is set to %d. The specification recommands to wait for at least 3 CRRs\n", conf.NCRRsToWait)
	}
	if conf.MarkovP < 0 || conf.MarkovP > 1 {
		return errors.New("MarkovP must be in interval [0;1]")
	}
	if conf.MarkovQ < 0 || conf.MarkovQ > 1 {
		return errors.New("MarkovQ must be in interval [0;1]")
	}
	return nil
}

// updateMetadata sends a MetaData Request to the server and parses the response
// to update metadata.
func updateMetadata(conn net.Conn, metadata *fileMetadata, conf *ClientConfig) error {
	buf := make([]byte, 0x10000) // 64kB
	if metadata.timeout == 0 {
		return errors.New("metadata.timeout cannot be 0.")
	}
retransmit:
	for i := 0; i < conf.RetransmissionsMDR; i++ {
		mdr := messages.GetMDR(metadata.messageCounter, &metadata.token, metadata.url)
		metadata.messageCounter++
		t_send := time.Now()
		err := mdr.Send(conn)
		if err != nil {
			if os.IsTimeout(err) {
				continue retransmit
			}
			// If this is not a timeout, it's safer to exit to see what happened
			return fmt.Errorf("send MDR: %w", err)
		}
		deadline := t_send.Add(metadata.timeout)
		err = conn.SetReadDeadline(deadline)
		if err != nil {
			return fmt.Errorf("set deadline: %w", err)
		}

	receive:
		for time.Now().Before(deadline) {
			n, err := conn.Read(buf)
			if err != nil {
				if errors.Is(err, os.ErrDeadlineExceeded) {
					// If it's a timeout, retransmit
					continue retransmit
				} else {
					// Otherwise, let's see what happened
					return fmt.Errorf("read from socket: %w", err)
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
					conf.WarnLogger.Printf("Invalid response received: %v. Dropped response:%x\n", err, raw)
					continue receive
				}
				conf.WarnLogger.Printf("Unknown error while parsing response: %v. Dropped response:%x\n", err, raw)
				continue receive
			}
			switch response.(type) {
			case messages.ServerHeader:
				header := response.(messages.ServerHeader)
				if header.Type != messages.MDRR_t {
					// Ignore it
					conf.WarnLogger.Printf("Unexpected server header of type %d. Dropped response: %v\n", header.Type, header)
					continue receive
				}
				if header.Number != mdr.Header.Number {
					// Not for us. Ignore it
					conf.DebugLogger.Printf("Received response with wrong message number(%d instead of %d). Dropped\n", header.Number, mdr.Header.Number)
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
				conf.DebugLogger.Printf("Updated token to %x (from %x). Retransmitting...\n", ntm.Token, mdr.Header.Token)
				continue retransmit
			case messages.MDRR:
				mdrr := response.(messages.MDRR)
				if mdrr.Header.Number != mdr.Header.Number {
					// This message is not for us. Ignore it
					conf.DebugLogger.Printf("Received response with wrong message number(%d instead of %d). Dropped\n", mdrr.Header.Number, mdr.Header.Number)
					continue receive
				}
				// Update metadata
				oldFileID := metadata.fileID
				err := getMetadataFromMDRR(metadata, &mdrr)
				if err != nil {
					return fmt.Errorf("invalid metadata: %w", err)
				}
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
					metadata.stats = *new(transferStats)
				}
				return nil
			default:
				conf.DebugLogger.Printf("Received unexpected response of type %T. Dropped\n", response)
				continue receive
			}
		}

	}
	return fmt.Errorf("Could not get metadata from server after %d retransmissions", conf.RetransmissionsMDR)
}

// getMetadataFromMDRR overwrites relevant field in metadata with fields from
// the MDRR.
func getMetadataFromMDRR(metadata *fileMetadata, mdrr *messages.MDRR) error {
	metadata.chunkSize = mdrr.ChunkSize
	metadata.maxChunksInACR = mdrr.MaxChunksInACR
	metadata.fileID = mdrr.FileID
	metadata.fileSize = messages.Uint8_6_arr2Int(mdrr.FileSize)
	metadata.checksum = mdrr.Checksum

	// Perform some sanity checks on the metadata
	if metadata.maxChunksInACR == 0 {
		return errors.New("maxChunkInACR cannot be 0")
	}
	if metadata.chunkSize == 0 {
		return errors.New("chunkSize cannot be 0")
	}
	if metadata.chunkSize > maxChunkSize {
		return fmt.Errorf("chunkSize is too large: %d, max is %d", metadata.chunkSize, maxChunkSize)
	}

	return nil
}

// Sends one ACR to get missing chunks.
// This function also receives the CRRs, write them to localFile, update the
// chunkMap and perform packet rate measurements.
func getMissingChunks(conn net.Conn, metadata *fileMetadata, conf *ClientConfig) error {
	buf := make([]byte, 0x10000) // 64kB
	// Build an ACR and send it
	acr, requested := buildACR(metadata)
	conf.DebugLogger.Printf("Requesting chunks %v\n", requested)
	n_cr := len(requested)
	if n_cr == 0 {
		return fmt.Errorf("no missing chunks.%v", metadata)
	}
	metadata.stats.requested += n_cr
	t_send := time.Now()
	err := acr.Send(conn)
	if err != nil {
		return fmt.Errorf("send ACR: %w", err)
	}
	deadline := t_send.Add(metadata.timeout)
	mapTimeCRRs := make(map[int]time.Time)
	received := false
	for time.Now().Before(deadline) {
		err = conn.SetReadDeadline(deadline)
		if err != nil {
			return fmt.Errorf("set deadline: %w", err)
		}
		n, err := conn.Read(buf)
		t_recv := time.Now()
		if err != nil {
			if os.IsTimeout(err) {
				// If it's a timeout, continue in case the deadline was extended
				continue
			} else {
				// Return to see what the error was
				return fmt.Errorf("read from socket: %w", err)
			}
		}
		raw := buf[:n]
		response, err := messages.ParseServer(&raw)
		if err != nil {
			metadata.stats.invalid ++
			if errors.Is(err, new(messages.UnsupporedVersionError)) {
				// We can't do anything if we don't speak the same language
				return fmt.Errorf("parse server message: %w", err)
			} else if errors.Is(err, new(messages.UnsupporedTypeError)) ||
				errors.Is(err, new(messages.WrongPacketLengthError)) {
				// Ignore unknown messages, but still log the error
				conf.WarnLogger.Printf("Invalid response received: %v. Dropped response:%x\n", err, raw)
				continue
			}
			conf.WarnLogger.Printf("Unknown error while parsing response: %v. Dropped response:%x\n", err, raw)
			continue
		}
		switch response.(type) {
		case messages.ServerHeader:
			header := response.(messages.ServerHeader)
			if header.Number != acr.Header.Number {
				// Ignore it
				metadata.stats.invalid ++
				conf.DebugLogger.Printf("Received response with wrong message number(%d instead of %d). Dropped\n", header.Number, acr.Header.Number)
				continue
			}
			switch header.Error {
			case messages.UnsupportedVersion:
				return fmt.Errorf("CRR server error: the server doesn't support our protocol version (%d) and answered with version %d", messages.VERS, header.Version)
			case messages.InvalidFileID:
				// Request new metadata and update it
				oldFileID := metadata.fileID
				err := updateMetadata(conn, metadata, conf)
				conf.InfoLogger.Printf("Updated metadata. Old fileID:%x, new fileID:%x\n", oldFileID, metadata.fileID)
				if err != nil {
					return fmt.Errorf("get metadata after invalid fileID: %w", err)
				}
			case messages.TooManyChunks:
				// Let's check
				if n_cr > int(metadata.maxChunksInACR) {
					// Our mistake... Let's throw an error to investigate
					return fmt.Errorf("malformed ACR: we requested %d chunks in an ACR. Max is %d. (%v)", n_cr, metadata.maxChunksInACR, acr)
				} else {
					metadata.stats.invalid ++
					conf.WarnLogger.Printf("Received TooManyChunks error for ACR %v. MaxChunksInACR=%d; # of chunks in ACR:%d. Ignored...\n", acr, metadata.maxChunksInACR, n_cr)
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
				metadata.stats.invalid ++
				conf.WarnLogger.Printf("Received ZeroLengthCR error for ACR %v. Ignored.\n", acr)
				continue
			default:
				return fmt.Errorf("CRR server error: Unknown error code for CRR %d", header.Error)
			}
		case messages.NTM:
			ntm := response.(messages.NTM)
			if ntm.Token != metadata.token {
				metadata.token = ntm.Token
				conf.DebugLogger.Printf("Updated token to %x (from %x). Retransmitting...\n", ntm.Token, acr.Header.Token)
				return nil // We shouldn't receive any further chunks if we used a wrong token.
				// Should it be an error, though ? No, we just want to resume normally
			}
		case messages.CRR:
			crr := response.(messages.CRR)
			if crr.Header.Number != acr.Header.Number {
				// This message is not for us. Ignore it
				metadata.stats.late ++
				conf.DebugLogger.Printf("Received response with wrong message number(%d instead of %d). Dropped\n", crr.Header.Number, acr.Header.Number)
				continue
			}
			chunkNumber := messages.Uint8_6_arr2Int(crr.ChunkNumber)
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
				metadata.stats.invalid ++
				conf.InfoLogger.Printf("Received CRR for unrequested chunk %d. (The requested chunks are %v). Dropped.\n", chunkNumber, requested)
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
					metadata.stats.invalid ++
					conf.WarnLogger.Printf("Received ChunkOutOfBounds error for chunk #%d in file of size %d. (%v)\n", chunkNumber, metadata.fileSize, acr)
					continue
				}
			}
			if !received {
				// If it's the first CRR we receive, update RTT
				metadata.timeout = time.Now().Sub(t_send) * time.Duration(rtt2timeoutFactor)
				received = true
			}
			mapTimeCRRs[chunkIndexInACR] = t_recv
			deadline = t_recv.Add(time.Duration(conf.NCRRsToWait+n_cr-chunkIndexInACR) * time.Second / time.Duration(metadata.packetRate))

			err = writeChunkToFile(metadata, chunkNumber, crr.Data, metadata.localFile)
			if err != nil {
				conf.WarnLogger.Printf("Could not write chunk #%d to file %s : %v", chunkNumber, metadata.localFile.Name(), err)
				continue
			}
		default:
			// Ignore irrelevant messages
			metadata.stats.received ++
			conf.WarnLogger.Printf("Received unexpected response type to ACR: %T. Dropped...\n", response)
			continue
		}
	}
	if received {
		if n_cr > 1 {
			newPacketRate, err := computePacketRate(mapTimeCRRs, n_cr, metadata.packetRate)
			if err != nil {
				return fmt.Errorf("compute new packetRate: %w", err)
			}
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
		if chunkNumber != metadata.fileSize-1 && len(data) != int(metadata.chunkSize) {
			return fmt.Errorf("invalid chunk size. Expected %d got %d", metadata.chunkSize, len(data))
		}
		offset := int64(chunkNumber) * int64(metadata.chunkSize)
		_, err := file.WriteAt(data, offset)
		if err != nil {
			return fmt.Errorf("write data at offset: %w", err)
		}
		// Update chunkMap and first Missing
		metadata.chunkMap[chunkNumber] = true
		metadata.stats.received++
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
		return 0, fmt.Errorf("can only compute a packetRate if more than 2 packets were expected")
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
		return 0, fmt.Errorf("cannot measure a rate when no packet was received")
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
	measuredRate := float64(n_received) / estimatedLast.Sub(estimatedFirst).Seconds()
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
		return 0, fmt.Errorf("measured rate is NaN.")
	}
	return uint32(measuredRate), nil
}

func computeChecksum(filename string) ([32]byte, error) {
	var hash [32]byte
	data, err := os.ReadFile(filename)
	if err != nil {
		return hash, fmt.Errorf("compute checksum: %w", err)
	}
	h := sha256.New()
	h.Write(data)
	copy(hash[:], h.Sum(nil))

	return hash, nil
}
