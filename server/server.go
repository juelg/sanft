package server

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"net"
	"os"

	"gitlab.lrz.de/protocol-design-sose-2022-team-0/sanft/messages"
)

type Server struct {
	// keying material
	// fileID map: needs to be in both ways
	// -> maybe write own abstraction to keep in sync
	// or find library
	// File2ID map[string]uint32
	// ID2File map[uint32]string
	// read out from some config:
	ChunkSize      uint16
	MaxChunksInACR uint16
	Conn *net.UDPConn
	RootDir string
	MarkovP float32
	MarkovQ float32

	FileIDMap map[string]string

	// PrivateKey ed25519.PrivateKey
	// PublicKey ed25519.PublicKey
	// key []byte
	block cipher.Block
	key []byte
}


func createRandomKey() []byte{
    key := make([]byte, 256)
    rand.Read(key)
    return key
}

func createRandomAES128Key() []byte{
    key := make([]byte, 16)
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
		return nil, fmt.Errorf("Error while creating the socket: %v", err)
    }
	// check if root dir exists
	if _, err := os.Stat(root_dir); os.IsNotExist(err) {
		// root_dir does not exist does not exist
		return nil, fmt.Errorf("root_dir does not exist: %v", err)
	}
	// check that p and q are valid
	if markovP >1 || markovP <0 || markovQ >1 || markovQ <0 {
		return nil, fmt.Errorf("P and/or Q values for the markov chain are invalid")
	}

	s := new(Server)
	s.ChunkSize = chunk_size
	s.MaxChunksInACR = max_chunks_in_acr
	s.Conn = conn
	s.MarkovP = markovP
	s.MarkovQ = markovQ
	s.RootDir = root_dir
	// empty file ID map
	s.FileIDMap = make(map[string]string)

	// create private key
	// public_key, private_key, err := ed25519.GenerateKey(nil)
	// if err != nil{
	// 	return nil, fmt.Errorf("Error while creating key: %v", err)
	// }
	// s.PrivateKey = private_key
	// s.PublicKey = public_key

	// create key
	// s.key = createRandomAES128Key()
	// block, err := aes.NewCipher(createRandomAES128Key())

	// err = s.NewKey()
    // if err != nil{
	// 	return nil, err
    // }

	s.key = createRandomKey()

	return s, nil
}

// TODO random new key
func (s *Server) NewKey() error {
	block, err := aes.NewCipher(createRandomAES128Key())
    if err != nil{
		return fmt.Errorf("Error while creating new key: %v", err)
    }
	s.block = block
	return nil
}

// server methods

func (s Server) Listen() error {
	// TODO: listen until channel says stop?
	for {
		addr, data, err := messages.ServerReceive(s.Conn)
		if err != nil{
			return fmt.Errorf("Error while receiving form UDP socket: %v", err)
		}
		msgr, err := messages.ParseClient(&data)
		if err != nil{
			return fmt.Errorf("Error while parsing client message: %v", err)
		}
		switch msg := msgr.(type){
		case messages.MDR:
			go s.handleMDR(msg, addr)
		case messages.ACR:
			go s.handleACR(msg, addr)
		}

	}
}

func (s Server) handleMDR(msg messages.MDR, addr *net.UDPAddr){
	// - MDR: check token, lookup file id (= hash out of path + last modified), filesize, checksum

}

func (s Server) handleACR(msg messages.ACR, addr *net.UDPAddr){
	// - ACR: check token, read file chunk

}


func (s Server) createToken(addr *net.UDPAddr) [32]byte {
	ip_port_bytes := getPortIPBytes(addr)

	data := make([]byte, len(ip_port_bytes) + len(s.key))
	copy(data[:len(ip_port_bytes)], ip_port_bytes)
	copy(data[len(ip_port_bytes):], s.key)
	return sha256.Sum256(data)


	// cipherText := make([]byte, aes.BlockSize+len(ip_port_bytes))

	// iv := cipherText[:aes.BlockSize]
	// _, err := io.ReadFull(rand.Reader, iv)

	// if err != nil {
	// 	return nil, fmt.Errorf("Error while putting random data into cipher text: %v", err)
	// }

	// // encrypt
	// stream := cipher.NewCFBEncrypter(s.block, iv)
	// stream.XORKeyStream(cipherText[aes.BlockSize:], ip_port_bytes)

	// padded := make([]byte, 256)
	// copy(padded[:len(cipherText)], cipherText)

	// // convert to fixed size array
	// return (*[256]byte)(padded), nil
}

func (s Server) checkToken(addr *net.UDPAddr, Token *[32]uint8) bool{
	return s.createToken(addr) == *Token
}

// returns IP + Port bytes slice
func getPortIPBytes(addr *net.UDPAddr) []byte{
	ip_bytes := []byte(addr.IP)
	port_bytes := make([]byte, 2)
	binary.LittleEndian.PutUint16(port_bytes, uint16(addr.Port))
	return append(ip_bytes, port_bytes...)
}





