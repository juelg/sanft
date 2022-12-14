package server

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"math"
	"net"
	"os"
	"strings"
	"time"

	"gitlab.lrz.de/protocol-design-sose-2022-team-0/sanft/markov"
	"gitlab.lrz.de/protocol-design-sose-2022-team-0/sanft/messages"
)

const KEY_VALIDITY = 12 * time.Hour

type FileM struct {
	Path string
	T    time.Time
	Try  int
	// cache checksum to avoid calculating it again if the file has not been modified
	Checksum *[32]uint8
}

type Server struct {
	// read out from some config:
	ChunkSize      uint16
	MaxChunksInACR uint16
	Conn           net.PacketConn
	RootDir        string
	MarkovP        float64
	MarkovQ        float64

	FileIDMap map[uint32]FileM

	// keying material
	key         []uint8
	valid_until time.Time

	// constant packet rate increase
	RateIncrease float64

	// Debugging
	DebugLogger *log.Logger
	InfoLogger  *log.Logger
	WarnLogger  *log.Logger
}

func createRandomKey() []uint8 {
	key := make([]uint8, 256)
	rand.Read(key)
	return key
}

// Initialize: chunksize, root folder, max chunks in acr
// work: listen for requests and answer them in go routine
// - MDR: check token, lookup file id (= hash out of path + last modified), filesize, checksum
// - ACR: check token, read file chunk
// to check whether the current file id is the latest -> map[fileid] -> path -> lookup and calc fileid
func Init(ip net.IP, port int, root_dir string, chunk_size uint16, max_chunks_in_acr uint16, markovP float64, markovQ float64, rate_increase float64) (*Server, error) {
	// check if root dir exists
	if _, err := os.Stat(root_dir); os.IsNotExist(err) {
		// root_dir does not exist does not exist
		return nil, fmt.Errorf("root_dir does not exist: %w", err)
	}
	// check that p and q are valid
	if markovP > 1 || markovP < 0 || markovQ > 1 || markovQ < 0 {
		return nil, fmt.Errorf("p and/or q values for the markov chain are invalid")
	}
	// check that the chunk size is valid
	if chunk_size == 0 || chunk_size > 65517 {
		return nil, fmt.Errorf("chunk size must be at least 1 and at most 65517")
	}
	// check that max_chunks_in_acr
	if max_chunks_in_acr == 0 {
		return nil, fmt.Errorf("max_chunks_in_acr cannot be 0")
	}
	// check if path is valid
	if root_dir[len(root_dir)-1] != '/' {
		return nil, fmt.Errorf("invalid path, must end with a slash")
	}
	// conn, err := messages.CreateServerSocket(ip, port)
	conn, err := markov.CreateServerSocket(ip, port, markovP, markovQ)
	if err != nil {
		return nil, fmt.Errorf("error while creating the socket: %w", err)
	}

	s := new(Server)
	s.ChunkSize = chunk_size
	s.MaxChunksInACR = max_chunks_in_acr
	s.Conn = conn
	s.MarkovP = markovP
	s.MarkovQ = markovQ
	s.RootDir = root_dir
	// empty file ID map
	s.FileIDMap = make(map[uint32]FileM)

	s.NewKey()
	s.RateIncrease = rate_increase

	// logger
	s.DebugLogger = log.New(ioutil.Discard, "DEBUG: ", log.LstdFlags)
	s.InfoLogger = log.New(os.Stderr, "INFO: ", log.LstdFlags)
	s.WarnLogger = log.New(os.Stderr, "WARN: ", log.LstdFlags)

	return s, nil
}

func (s *Server) NewKey() {
	s.key = createRandomKey()
	s.valid_until = time.Now().Add(KEY_VALIDITY)
}

func (s *Server) RefreshKey() {
	if time.Now().After(s.valid_until) {
		s.NewKey()
	}
}

// server methods

// the only purpose of the channel is to tell the function
// when to stop listening
func (s *Server) Listen(close chan bool) {

	for cont(close) {
		// refreshing key every 12 hours
		s.RefreshKey()

		// short timeout to be responsive
		addr, data, err := messages.ServerReceive(s.Conn, 100)
		if os.IsTimeout(err) {
			// next iteration when timeout
			continue
		}
		if err != nil {
			s.WarnLogger.Printf("error while receiving form UDP socket: %v\n", err)
			continue
		}
		msgr, err := messages.ParseClient(&data)

		// check for parsing specific errors
		var e1 *messages.WrongPacketLengthError
		var e2 *messages.UnsupporedTypeError
		var e3 *messages.UnsupporedVersionError
		if errors.As(err, &e1) && errors.As(err, &e2) {
			// Invalid request, drop request
			s.DebugLogger.Println("Invalid request, dropped...")
			continue
		}

		if errors.As(err, &e3) {
			// wrong version
			msgr := messages.ServerHeader{Version: messages.VERS, Type: data[1], Number: data[2], Error: messages.UnsupportedVersion}
			msgr.Send(s.Conn, addr)
			continue
		}

		if err != nil {
			s.DebugLogger.Printf("error while parsing client message: %v\n", err)
			continue
		}

		switch msg := msgr.(type) {
		case messages.MDR:
			go s.handleMDR(msg, addr)
		case messages.ACR:
			go s.handleACR(msg, addr)
		}

	}
}

func (s *Server) GetPath(path string) string {
	if path[0] == '/' {
		// remove trailing "/"
		path = path[1:]
	}
	path = s.RootDir + path
	// remove ..
	path = strings.ReplaceAll(path, "..", "")
	// remove //
	path = strings.ReplaceAll(path, "//", "/")
	return path
}

func (s *Server) handleMDR(msg messages.MDR, addr net.Addr) {
	// - MDR: check token, (check if file exists) lookup file id (= hash out of path + last modified), filesize, checksum
	s.InfoLogger.Printf("MDR from %v for path %v\n", addr, string(msg.URI))

	// check token
	if !s.checkToken(addr, &msg.Header.Token) {
		s.DebugLogger.Printf("Invalid token in MDR of %v, sending new token...\n", addr)
		s.sendNTM(msg.Header.Number, messages.NoError, addr)
		return
	}

	// check if file exists
	filepath := s.GetPath(msg.URI)
	file, err := os.Stat(filepath)
	if errors.Is(err, os.ErrNotExist) {
		// URI does not exist
		s.DebugLogger.Printf("URI does not exits: %v\n", string(msg.URI))
		msg := messages.ServerHeader{Version: messages.VERS, Type: messages.MDRR_t,
			Number: msg.Header.Number, Error: messages.FileNotFound}
		msg.Send(s.Conn, addr)
		return
	}

	// filesize
	filesize := file.Size()
	// round up
	filesize_in_chunks := Ceil(filesize, int64(s.ChunkSize))
	if filesize_in_chunks > (2<<48)-1 {
		// file too large, cant serve -> return file not found: Implementation specific
		s.WarnLogger.Printf("File is too large, cant serve: %v\n", string(msg.URI))
		msg := messages.ServerHeader{Version: messages.VERS, Type: messages.MDRR_t,
			Number: msg.Header.Number, Error: messages.FileNotFound}
		msg.Send(s.Conn, addr)
		return
	}

	var fileid uint32
	var checksum *[32]uint8
	for i := 0; i < 10; i++ {
		if i == 8 {
			// many tries, space must be almost full e.g. many outdate files
			// delete old map and recreate
			s.FileIDMap = make(map[uint32]FileM)
			// run loop one more time, now we should find a valid id
			continue
		}
		// lookup last modified
		lastChanged := file.ModTime()
		fileid, err = GetFileID(filepath, lastChanged, i)
		if err != nil {
			s.DebugLogger.Printf("error while getting file id: %v\n", err)
			return
		}
		// check if file id is dict, if yes check if the same file, if no
		// try to change the file twice, if they dont succeed, empty hash map

		filem, ok := s.FileIDMap[fileid]
		if ok {
			if filem.Path == filepath && filem.T == lastChanged {
				// same file: use this file id
				checksum = filem.Checksum
				break

			} else {
				// id exists in map -> try to find new id
				continue
			}
		} else {
			checksum, err = GetFileChecksum(filepath)
			if err != nil {
				s.DebugLogger.Printf("error while getting file checksum: %v\n", err)
				return
			}
			s.FileIDMap[fileid] = FileM{Path: filepath, T: lastChanged, Try: i, Checksum: checksum}
			break
		}

	}

	msgs := messages.GetMDRR(msg.Header.Number, messages.NoError, s.ChunkSize, s.MaxChunksInACR, fileid, *messages.Int2uint8_6_arr(uint64(filesize_in_chunks)), (*[32]uint8)(checksum))
	if err = msgs.Send(s.Conn, addr); err != nil {
		s.WarnLogger.Printf("error while sending: %v\n", err)
	}

}

func (s *Server) handleACR(msg messages.ACR, addr net.Addr) {
	// - ACR: check token, check file id in dict (what happens if mdr hasnt been send before?), read file chunk and return
	s.InfoLogger.Printf("ACR from %v for id: 0x%x, with rate: %v and %v CRs \n", addr, msg.FileID, msg.PacketRate, len(msg.CRs))
	if !s.checkToken(addr, &msg.Header.Token) {
		s.DebugLogger.Printf("Invalid token in ACR of %v, sending new token\n", addr)
		s.sendNTM(msg.Header.Number, messages.NoError, addr)
		return
	}
	// check if file id in dict otherwise invalid file id
	filem, ok := s.FileIDMap[msg.FileID]
	if !ok {
		// fileid does not exist (yet)
		s.DebugLogger.Printf("FileID 0x%x does not exist\n", msg.FileID)
		msg := messages.ServerHeader{Version: messages.VERS, Type: messages.CRR_t,
			Number: msg.Header.Number, Error: messages.InvalidFileID}
		msg.Send(s.Conn, addr)
		return
	}
	// check file exists with the save timestamp
	file, err := os.Stat(filem.Path)
	if errors.Is(err, os.ErrNotExist) || file.ModTime() != filem.T {
		// file does no longer exist or has been modified
		s.DebugLogger.Printf("FileID %x does no longer exist\n", msg.FileID)
		// delete from dict
		delete(s.FileIDMap, msg.FileID)
		// send error message
		msg := messages.ServerHeader{Version: messages.VERS, Type: messages.CRR_t,
			Number: msg.Header.Number, Error: messages.InvalidFileID}
		msg.Send(s.Conn, addr)
		return
	}

	// wait with specify rate: 1/rate
	delta_t := 1.0 / (float64(msg.PacketRate) + s.RateIncrease) * float64(time.Second)

	amount_chunks := 0

	f, err := os.Open(filem.Path)
	if err != nil {
		s.DebugLogger.Printf("error while opening file: %v\n", err)
		return
	}
	defer f.Close()

	// open chunk after each other and send with given rate + add some constant (todo: define constant in server struct)
	for _, i := range msg.CRs {
		offset := messages.Uint8_6_arr2Int(i.ChunkOffset)
		f.Seek(int64(offset*uint64(s.ChunkSize)), 0)

		// for stupid "zero length has the last priority reasons":
		var l int
		if i.Length == 0 {
			l = 1
		} else {
			l = int(i.Length)
		}

		for j := 0; j < l; j++ {
			chunk_number := offset + uint64(j)
			amount_chunks++
			// check too many chunks
			if amount_chunks > int(s.MaxChunksInACR) {
				s.DebugLogger.Printf("Too many chunks requested by %v\n", addr)
				msg := messages.ServerHeader{Version: messages.VERS, Type: messages.CRR_t,
					Number: msg.Header.Number, Error: messages.TooManyChunks}
				msg.Send(s.Conn, addr)
				return
			}

			// check chunk out of bounds
			if (offset+uint64(j))*uint64(s.ChunkSize) > uint64(file.Size()) {
				s.DebugLogger.Printf("CR %v out of bounds from %v\n", j, addr)
				zero_data := make([]uint8, 0)
				msg := messages.GetCRR(msg.Header.Number, messages.ChunkOutOfBounds, *messages.Int2uint8_6_arr(chunk_number), &zero_data)
				msg.Send(s.Conn, addr)
				break
			}

			// check zero length
			if i.Length == 0 {
				s.DebugLogger.Printf("CR %v has zero length from %v\n", j, addr)
				msg := messages.ServerHeader{Version: messages.VERS, Type: messages.CRR_t,
					Number: msg.Header.Number, Error: messages.ZeroLengthCR}
				msg.Send(s.Conn, addr)
				break
			}

			// read up to chunk size bytes
			buf := make([]uint8, s.ChunkSize)
			n, err := f.Read(buf)
			if err != nil {
				s.WarnLogger.Printf("error while reading from the file: %v\n", err)
				continue
			}
			// send the read bytes
			chunk := buf[:n]
			msg := messages.GetCRR(msg.Header.Number, messages.NoError, *messages.Int2uint8_6_arr(chunk_number), &chunk)
			msg.Send(s.Conn, addr)

			time.Sleep(time.Duration(delta_t))

		}

	}

}

func (s *Server) sendNTM(number uint8, err uint8, addr net.Addr) {
	token := s.createToken(addr)
	ntm := messages.GetNTM(number, err, &token)
	ntm.Send(s.Conn, addr)
}

func (s *Server) createToken(addr net.Addr) [32]uint8 {
	ip_port_bytes := getPortIPBytes(addr)

	data := make([]byte, len(ip_port_bytes)+len(s.key))
	copy(data[:len(ip_port_bytes)], ip_port_bytes)
	copy(data[len(ip_port_bytes):], s.key)
	return sha256.Sum256(data)
}

func (s *Server) checkToken(addr net.Addr, Token *[32]uint8) bool {
	return s.createToken(addr) == *Token
}

func (s *Server) StopListening(cl chan bool) {
	cl <- false
}

// false if something is send to the close channel, else otherwise
// whether to continue the loop in the Listen method
func cont(cl chan bool) bool {
	select {
	case <-cl:
		return false
	default:
		return true
	}
}

// returns IP + Port bytes slice
func getPortIPBytes(addr net.Addr) []byte {
	// cast to net.UDPAddr
	udpaddr := addr.(*net.UDPAddr)
	ip_bytes := []byte(udpaddr.IP)
	port_bytes := make([]byte, 2)
	binary.LittleEndian.PutUint16(port_bytes, uint16(udpaddr.Port))
	return append(ip_bytes, port_bytes...)
}

func Max(x, y uint64) uint64 {
	if x < y {
		return y
	}
	return x
}

func Ceil(a, b int64) int64 {
	return int64(math.Ceil(float64(a) / float64(b)))
}

func GetFileChecksum(filepath string) (*[32]uint8, error) {
	// checksum
	f, err := os.Open(filepath)
	if err != nil {
		return nil, fmt.Errorf("error while opening file: %w", err)
	}
	defer f.Close()
	h := sha256.New()
	if _, err = io.Copy(h, f); err != nil {
		return nil, fmt.Errorf("error while copying from file: %w", err)
	}
	checksum := h.Sum(nil)
	return (*[32]uint8)(checksum), nil
}

func GetFileID(path string, t time.Time, try int) (uint32, error) {
	buf := new(bytes.Buffer)
	_, err := buf.WriteString(path)
	if err != nil {
		return 0, fmt.Errorf("error encoding message: %w", err)
	}

	_, err = buf.WriteString(t.String())
	if err != nil {
		return 0, fmt.Errorf("error encoding message: %w", err)
	}
	binary.Write(buf, binary.LittleEndian, (int64)(try))

	longid := sha256.Sum256(buf.Bytes())

	return binary.LittleEndian.Uint32(longid[:4]), nil
}
