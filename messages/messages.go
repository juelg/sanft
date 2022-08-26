package messages

import (
	"encoding/binary"
	"net"
)

// TODO: do we need to pack the structs? -> should already be packed when casting in binary

// version
const VERS uint8 = 0

// server message types
const (
	NTM_t  uint8 = 0
	MDRR_t uint8 = 2
	CRR_t		 = 4
)


// client message types
const (
	MDR_t uint8 = 1
	ACR_t uint8	= 3
)

// error codes
const (
	NoError            uint8 = 0
	UnsupportedVersion uint8 = 1
	InvalidFileID      uint8 = 2
	FileNotFound       uint8 = 2
	TooManyChunks      uint8 = 3
	ChunkOutOfBounds   uint8 = 4
	ZeroLengthCR       uint8 = 5
)

func Int2uint8_6_arr(a uint64) *[6]uint8 {
	b := make([]uint8, 8)
	binary.LittleEndian.PutUint64(b, a)
    return (*[6]uint8)(b[:6])
}


type ServerMessage interface {
	Send(conn *net.UDPConn, addr *net.UDPAddr) error
}

type ClientMessage interface {
	Send(conn *net.UDPConn) error
}

type ClientHeader struct {
	Version uint8
	Type    uint8
	Number  uint8
	Token   [32]uint8
}

type ServerHeader struct {
	Version uint8
	Type    uint8
	Number  uint8
	Error   uint8
}

type NTM struct {
	Header ServerHeader
	Token  [32]uint8
}

func GetNTM(number uint8, err uint8, token *[32]uint8) *NTM{
	ntm := new(NTM)
	ntm.Header = ServerHeader{Version: VERS, Type: NTM_t, Number: number, Error: err}
	ntm.Token = *token
	return ntm
}

type MDR struct {
	Header ClientHeader
	URI    string /* this must be handled manualy when sending */
}

func GetMDR(number uint8, token *[32]uint8, uri string) *MDR{
	mdr := new(MDR)
	mdr.Header = ClientHeader{Version: VERS, Type: MDR_t, Number: number, Token: *token}
	mdr.URI = uri
	return mdr
}

type MDRR struct {
	Header         ServerHeader
	ChunkSize      uint16
	MaxChunksInACR uint16
	FileID         uint32
	FileSize       [6]uint8 /* alias, there is no uint48 :-( */
	Checksum       [32]uint8
}

func GetMDRR(number uint8, err uint8, chunk_size uint16,
			max_chunks_in_acr uint16, fileid uint32, filesize [6]uint8, checksum *[32]uint8) *MDRR{
	mdrr := new(MDRR)
	mdrr.Header = ServerHeader{Version: VERS, Type: MDRR_t, Number: number, Error: err}
	mdrr.ChunkSize = chunk_size
	mdrr.MaxChunksInACR = max_chunks_in_acr
	mdrr.FileID = fileid
	mdrr.FileSize = filesize
	mdrr.Checksum = *checksum
	return mdrr
}

type CR struct {
	ChunkOffset [6]uint8 /* note to self: 48-bit field sizes were a dumb idea */
	Length      uint8
}

func GetCR(chunkoffset [6]uint8, length uint8) *CR{
	cr := new(CR)
	cr.ChunkOffset = chunkoffset
	cr.Length = length
	return cr
}

type ACR struct {
	Header     ClientHeader
	FileID     uint32
	PacketRate uint32
	CRs        []CR
}

func GetACR(number uint8, token *[32]uint8, fileid uint32, packet_rate uint32, crlist *[]CR) *ACR{
	acr := new(ACR)
	acr.Header = ClientHeader{Version: VERS, Type: ACR_t, Number: number, Token: *token}
	acr.FileID = fileid
	acr.PacketRate = packet_rate
	acr.CRs = *crlist
	return acr
}

type CRR struct {
	Header      ServerHeader
	ChunkNumber [6]uint8 /* :-( */
	Data        []uint8
}

func GetCRR(number uint8, err uint8, chunknumber [6]uint8, data *[]uint8) *CRR{
	crr := new(CRR)
	crr.Header = ServerHeader{Version: VERS, Type: CRR_t, Number: number, Error: err}
	crr.ChunkNumber = chunknumber
	crr.Data = *data
	return crr
}
