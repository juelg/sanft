package server

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"time"
	"gitlab.lrz.de/protocol-design-sose-2022-team-0/sanft/messages"
)

type FileM struct{
	URI string
	T time.Time
	Try int
	// cache checksum to avoid calculating it again if the file has not been modified
	Checksum *[32]uint8
}

type Server struct {
	// read out from some config:
	ChunkSize      uint16
	MaxChunksInACR uint16
	Conn *net.UDPConn
	RootDir string
	MarkovP float32
	MarkovQ float32

	FileIDMap map[uint32] FileM

	// keying material
	key []uint8
}


func createRandomKey() []uint8{
    key := make([]uint8, 256)
    rand.Read(key)
    return key
}

// initialize: chunksize, root folder, max chunks in acr, markov chain probabilities
// TODO: markov chain for client and server together
// work: listen for requests and answer them in go routine
// - MDR: check token, lookup file id (= hash out of path + last modified), filesize, checksum
// - ACR: check token, read file chunk
// to check whether the current file id is the latest -> map[fileid] -> path -> lookup and calc fileid


// The values should be sanity checked before putting into this function
// valid ip and port, markov p and q between 0 and 1, root_dir exists
func Init(ip string, port int, root_dir string, chunk_size uint16, max_chunks_in_acr uint16, markovP float32, markovQ float32) (*Server, error){
    conn, err := messages.CreateServerSocket(ip, port)
    if err != nil{
		return nil, fmt.Errorf("error while creating the socket: %w", err)
    }
	// check if root dir exists
	if _, err := os.Stat(root_dir); os.IsNotExist(err) {
		// root_dir does not exist does not exist
		return nil, fmt.Errorf("root_dir does not exist: %w", err)
	}
	// check that p and q are valid
	if markovP >1 || markovP <0 || markovQ >1 || markovQ <0 {
		return nil, fmt.Errorf("p and/or q values for the markov chain are invalid")
	}
	// check if path is valid
	if root_dir[len(root_dir)-1] != '/'{
		return nil, fmt.Errorf("invalid path, must end with a slash")
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

	return s, nil
}

func (s *Server) NewKey() {
	s.key = createRandomKey()
}

// server methods
func (s *Server) Listen() error {
	// TODO: listen until channel says stop?
	// TODO this shouldnt return an error but instead just log
	for {
		addr, data, err := messages.ServerReceive(s.Conn)
		if err != nil{
			return fmt.Errorf("error while receiving form UDP socket: %w", err)
		}
		msgr, err := messages.ParseClient(&data)
		// TODO check for parsing specific errors

    	var e1 *messages.WrongPacketLengthError
    	var e2 *messages.UnsupporedTypeError
    	var e3 *messages.UnsupporedVersionError
		if errors.As(err, &e1) && errors.As(err, &e2) {
			// Invalid request, drop request
			log.Println("Invalid request, dropped..")
			continue
		}

		if errors.As(err, &e3){
			// wrong version
			// TODO: should the token be checked before? should we still include valid number?
			msgr := messages.ServerHeader{Version: messages.VERS, Type: data[1], Number: data[2], Error: messages.UnsupportedVersion}
			msgr.Send(s.Conn, addr)
			continue
		}

		if err != nil{
			return fmt.Errorf("error while parsing client message: %w", err)
		}

		switch msg := msgr.(type){
		case messages.MDR:
			go s.handleMDR(msg, addr)
		case messages.ACR:
			go s.handleACR(msg, addr)
		}

	}
}

func (s *Server) GetPath(path string) string{
	// re := s.RootDir + path
	// return strings.Replace(re, "//", "/", 1)
	return s.RootDir + path
}

func (s *Server) handleMDR(msg messages.MDR, addr *net.UDPAddr){
	// - MDR: check token, (check if file exists) lookup file id (= hash out of path + last modified), filesize, checksum

	// check token
	if !s.checkToken(addr, &msg.Header.Token){
		s.sendNTM(msg.Header.Number, messages.NoError, addr)
		return
	}
	if msg.URI[0] == '/'{
		// remove trailing "/"
		msg.URI = msg.URI[1:]
	}


	// check if file exists
	filepath := s.GetPath(msg.URI)
	file, err := os.Stat(filepath)
	if errors.Is(err, os.ErrNotExist) {
		// URI does not exist
		msg := messages.ServerHeader{Version: messages.VERS, Type: messages.MDRR_t,
							Number: msg.Header.Number, Error: messages.FileNotFound}
		msg.Send(s.Conn, addr)
		return
	}

	// filesize
	filesize := file.Size()
	filesize_in_chunks := filesize / int64(s.ChunkSize)
	if filesize_in_chunks > (2<<48)-1{
		// file too large, cant serve -> return file not found: Implementation specific
		msg := messages.ServerHeader{Version: messages.VERS, Type: messages.MDRR_t,
							Number: msg.Header.Number, Error: messages.FileNotFound}
		msg.Send(s.Conn, addr)
		return
	}


	var fileid uint32
	var checksum *[32]uint8
	for i := 0; i < 10; i++ {
		if i == 8{
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
			log.Printf("error while getting file id: %v\n", err)
			return
		}
		// check if file id is dict, if yes check if the same file, if no
		// try to change the file twice, if they dont succeed, empty hash map

		filem, ok := s.FileIDMap[fileid]
		if ok {
			if filem.URI == msg.URI && filem.T == lastChanged {
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
				log.Printf("error while getting file checksum: %v\n", err)
				return
			}
			s.FileIDMap[fileid] = FileM{URI: msg.URI, T: lastChanged, Try: i, Checksum: checksum}
			break
		}

	}

	msgs := messages.GetMDRR(msg.Header.Number, messages.NoError, s.ChunkSize, s.MaxChunksInACR, fileid, *messages.Int2uint8_6_arr(uint64(filesize_in_chunks)), (*[32]uint8)(checksum))
	if err = msgs.Send(s.Conn, addr); err != nil{
		log.Printf("error while sending: %v\n",  err)
	}

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

func GetFileID(path string, t time.Time, try int) (uint32, error){
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

func (s *Server) handleACR(msg messages.ACR, addr *net.UDPAddr){
	// - ACR: check token, check file id in dict (what happens if mdr hasnt been send before?), read file chunk and return
	if !s.checkToken(addr, &msg.Header.Token){
		s.sendNTM(msg.Header.Number, messages.NoError, addr)
		return
	}
	// check if file id in dict otherwise invalid file id

}

func (s *Server) sendNTM(number uint8, err uint8, addr *net.UDPAddr){
	token := s.createToken(addr)
	ntm := messages.GetNTM(number, err, &token)
	ntm.Send(s.Conn, addr)
}


func (s *Server) createToken(addr *net.UDPAddr) [32]uint8 {
	ip_port_bytes := getPortIPBytes(addr)

	data := make([]byte, len(ip_port_bytes) + len(s.key))
	copy(data[:len(ip_port_bytes)], ip_port_bytes)
	copy(data[len(ip_port_bytes):], s.key)
	return sha256.Sum256(data)
}

func (s *Server) checkToken(addr *net.UDPAddr, Token *[32]uint8) bool{
	return s.createToken(addr) == *Token
}

// returns IP + Port bytes slice
func getPortIPBytes(addr *net.UDPAddr) []byte{
	ip_bytes := []byte(addr.IP)
	port_bytes := make([]byte, 2)
	binary.LittleEndian.PutUint16(port_bytes, uint16(addr.Port))
	return append(ip_bytes, port_bytes...)
}





