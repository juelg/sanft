package client

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"os"
	"regexp"
	"testing"
	"time"

	"gitlab.lrz.de/protocol-design-sose-2022-team-0/sanft/messages"
)

func startMockServer(quit chan bool, conn *net.UDPConn, filename string, chunkSize uint16, maxChunksInACR uint16, fileID uint32, fileData []byte) {
	packetRateAddC := 10
	fileSize := uint64((len(fileData) + int(chunkSize-1))/int(chunkSize))
	var checksum [32]byte
	var token [32]uint8
	h := sha256.New()
	_, err := h.Write(fileData)
	if err != nil {
		fmt.Printf("Mock Server: Error with compute checksum: %v\n", err)
		return
	}
	copy(checksum[:], h.Sum(nil))
	for {
		select {
		case <- quit:
			return
		default:
			conn.SetReadDeadline(time.Now().Add(500*time.Millisecond))
			addr, data, err := messages.ServerReceive(conn)
			if err != nil{
				if os.IsTimeout(errors.Unwrap(err)) {
					continue
				}
				fmt.Printf("timeout: %v\n", err)
				continue
			}
			addrData := new(bytes.Buffer)
			binary.Write(addrData, binary.BigEndian, addr)
			h := sha256.New()
			_, err = h.Write(addrData.Bytes())
			if err != nil {
				fmt.Printf("Mock Server: Error with compute token: %v\n", err)
				return
			}
			copy(token[:], h.Sum(nil))
			msg, err := messages.ParseClient(&data)
			if err != nil{
				fmt.Printf("Mock Server: while parsing client message: %v\n", err)
				continue
			}
			switch msg.(type){
			case messages.MDR:
				mdr := msg.(messages.MDR)
				if mdr.Header.Token != token {
					response := messages.GetNTM(mdr.Header.Number, 0, &token)
					err = response.Send(conn, addr)
					if err != nil {
						fmt.Printf("Mock Server: Error while sending NTM: %v\n", err)
					}
					break
				}
				if mdr.URI != filename {
					response := messages.ServerHeader{Version: messages.VERS, Type: messages.MDRR_t, Number: mdr.Header.Number, Error: messages.FileNotFound}
					err = response.Send(conn, addr)
					if err != nil {
						fmt.Printf("Mock Server: Error while sending header: %v\n", err)
					}
					break
				}
				response := messages.GetMDRR(mdr.Header.Number, 0, uint16(chunkSize), uint16(maxChunksInACR), fileID, *messages.Int2uint8_6_arr(fileSize), &checksum)
				err = response.Send(conn, addr)
				if err != nil {
					fmt.Printf("Mock Server: Error while sending MDRR: %v\n", err)
				}
			case messages.ACR:
				acr := msg.(messages.ACR)
				if acr.Header.Token != token {
					response := messages.GetNTM(acr.Header.Number, 0, &token)
					err = response.Send(conn, addr)
					if err != nil {
						fmt.Printf("Mock Server: Error while sending NTM: %v\n", err)
					}
					break
				}
				if acr.FileID != fileID {
					response := messages.ServerHeader{Version: messages.VERS, Type: messages.CRR_t, Number: acr.Header.Number, Error: messages.InvalidFileID}
					err = response.Send(conn, addr)
					if err != nil {
						fmt.Printf("Mock Server: Error while sending header: %v\n", err)
					}
					break
				}
				n_cr := 0
				packetRate := acr.PacketRate + uint32(packetRateAddC)
				tNext := time.Now()
				for _,cr := range acr.CRs {
					offset := messages.Uint8_6_arr2int(&cr.ChunkOffset)
					for i:=offset; i < offset+uint64(cr.Length); i++ {
						n_cr ++
						if n_cr > int(maxChunksInACR) {
							response := messages.ServerHeader{Version: messages.VERS, Type: messages.CRR_t, Number: acr.Header.Number, Error: messages.TooManyChunks}
							err = response.Send(conn, addr)
							if err != nil {
								fmt.Printf("Mock Server: Error while sending header: %v\n", err)
							}
							break
						}
						if i > fileSize {
							response := messages.GetCRR(acr.Header.Number, messages.ChunkOutOfBounds, *messages.Int2uint8_6_arr(i), &[]uint8{})
							err = response.Send(conn, addr)
							if err != nil {
								fmt.Printf("Mock Server: Error while sending CRR with error: %v\n", err)
							}
							break
						}
						time.Sleep(tNext.Sub(time.Now()))
						var chunk []byte
						if i < fileSize-1 {
							chunk = fileData[i*uint64(chunkSize):(i+1)*uint64(chunkSize)]
						} else {
							chunk = fileData[i*uint64(chunkSize):]
						}
						response := messages.GetCRR(acr.Header.Number, messages.NoError, *messages.Int2uint8_6_arr(i), &chunk)
						err = response.Send(conn, addr)
						if err != nil {
							fmt.Printf("Mock Server: Error while sending CRR: %v\n", err)
						}
						tNext = time.Now().Add(time.Second/time.Duration(packetRate))
					}
				}
			}

		}
	}
}

func TestBuildACR(t *testing.T) {
	metadata := fileMetadata{}
	metadata.token = [32]uint8{
		0x01, 0x23, 0x45, 0x67, 0x89, 0xab, 0xcd, 0xef,
		0xfe, 0xdc, 0xba, 0x98, 0x76, 0x54, 0x32, 0x01,
		0x00, 0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77,
		0x88, 0x99, 0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff,
	}
	metadata.maxChunksInACR = 30
	metadata.chunkMap = make(map[uint64]bool)
	metadata.chunkMap[10] = true
	metadata.fileSize = 25
	metadata.fileID = 0xcafebabe

	acr, requested := buildACR(&metadata)
	in_acr := make(map[uint64]bool)
	n_requests := 0

	// Check the metadata
	if acr.Header.Token != metadata.token {
		t.Fatalf("Invalid token in ACR. Expected %x got %x", metadata.token, acr.Header.Token)
	}
	if acr.FileID != metadata.fileID {
		t.Fatalf("Invalid file ID in ACR. Expected %x got %x", metadata.fileID, acr.FileID)
	}

	// Check that the ACR is valid
	for _, cr := range acr.CRs {
		offset := messages.Uint8_6_arr2int(&cr.ChunkOffset)
		length := cr.Length
		if length == 0 {
			t.Fatalf("Invalid length (0) for chunk request in %v", acr)
		}
		for i := uint64(0); i < uint64(length); i++ {
			in_acr[offset+i] = true
			if offset+i > metadata.fileSize {
				t.Fatalf("Request for an out of bound chunk %d in ACR %v", offset+i, acr)
			}
			if metadata.chunkMap[offset+i] {
				t.Fatalf("Unexpected chunk request for already received chunk %d in ACR %v", offset+i, acr)
			}
			n_requests++
		}
	}
	if n_requests > int(metadata.maxChunksInACR) {
		t.Fatalf("Too many chunks in chunk request : %d (max %d)", n_requests, metadata.maxChunksInACR)
	}

	// Check that requested matches the id of the chunks requested in the ACR:
	for _, r := range requested {
		if !in_acr[r] {
			t.Fatalf("Chunk %d is in requested but not in ACR", r)
		}
	}
	if len(requested) != n_requests {
		t.Fatalf("There is not the same number of chunks in requested (%d) and in ACR(%d)", len(requested), n_requests)
	}
}

// Generate data, compute checksum, write data to file,
// check that compute Checksum gives the same checksum
func TestChecksum(t *testing.T) {
	// Generate random data
	size := 0x234
	data := make([]byte, size)
	_, err := rand.Read(data)
	if err != nil {
		fmt.Printf("Error while generating random data: %v", err)
		return
	}

	// Compute first checksum
	var hash1 [32]byte
	h := sha256.New()
	h.Write(data)
	copy(hash1[:], h.Sum(nil))

	// Write data to file
	tmp, err := os.CreateTemp("", "checksum_test_go")
	if err != nil {
		fmt.Printf("Open temp file: %v\n", err)
		return
	}
	defer os.Remove(tmp.Name())

	_, err = tmp.Write(data)
	if err != nil {
		fmt.Printf("Write to tmp:%v\n", err)
		return
	}
	filename := tmp.Name()
	tmp.Close()

	hash2, err := computeChecksum(filename)
	if err != nil {
		t.Fatalf("Error while computing checksum: %v", err)
	}

	if hash1 != hash2 {
		t.Fatalf("Invalid checksum. Expected %x got %x", hash1, hash2)
	}
}

func TestComputePacketRate(t *testing.T) {
	n_expected := 40
	timeMap := make(map[int]time.Time)
	prevRate := uint32(20)
	t0 := time.Now()

	for i := 0; i < n_expected; i++ {
		timeMap[i] = t0.Add(time.Duration(i * int(time.Second) / int(prevRate)))
	}

	newRate, err := computePacketRate(timeMap, n_expected, prevRate)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if newRate == 0 {
		t.Fatalf("packetRate cannot be zero")
	}

	if newRate != prevRate {
		t.Fatalf("New packetRate (%d) differs from previous packetRate (%d)", newRate, prevRate)
	}
}

func TestComputePacketRateMissing(t *testing.T) {
	n_expected := 324
	timeMap := make(map[int]time.Time)
	prevRate := uint32(2000)
	t0 := time.Now()

	for _, i := range []int{1, 4, 23, 24, 45, 46} {
		timeMap[i] = t0.Add(time.Duration(i * int(time.Second) / int(prevRate)))
	}

	newRate, err := computePacketRate(timeMap, n_expected, prevRate)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if newRate == 0 {
		t.Fatalf("packetRate cannot be zero")
	}

	if newRate > prevRate {
		t.Fatalf("New packetRate (%d) is larger than previous packetRate (%d)", newRate, prevRate)
	}
}

func TestComputePacketRateLate(t *testing.T) {
	n_expected := 10
	timeMap := make(map[int]time.Time)
	prevRate := uint32(4)
	t0 := time.Now()

	for i := 0; i < n_expected; i++ {
		timeMap[i] = t0.Add(time.Duration(i * int(time.Second) / int(prevRate)))
	}

	timeMap[0] = t0.Add(2 * time.Second)

	newRate, err := computePacketRate(timeMap, n_expected, prevRate)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if newRate == 0 {
		t.Fatalf("packetRate cannot be zero")
	}

	/*
		if newRate > prevRate {
			t.Fatalf("New packetRate (%d) is larger than previous packetRate (%d)", newRate, prevRate)
		}
	*/
}

func TestWriteChunkToFile(t *testing.T) {
	var readBuf []byte
	var metadata fileMetadata
	metadata.chunkMap = make(map[uint64]bool)
	metadata.chunkSize = 512
	metadata.fileSize = 234

	chunkNumber := uint64(5)

	data := make([]byte, metadata.chunkSize)
	readBuf = make([]byte, metadata.chunkSize)

	copy(data, "Lorem ipsum dolores amet etcaetera etcaetera...")

	// Write data to file
	tmp, err := os.CreateTemp("", "checksum_test_go")
	if err != nil {
		fmt.Printf("Open temp file: %v\n", err)
		return
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()

	err = writeChunkToFile(&metadata, chunkNumber, data, tmp)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if !metadata.chunkMap[chunkNumber] {
		t.Fatalf("chunkMap has not been updated")
	}

	_, err = tmp.ReadAt(readBuf, int64(chunkNumber)*int64(metadata.chunkSize))
	if err != nil {
		t.Fatalf("Error while reading file: %v", err)
	}

	for i:=0 ; i<int(metadata.chunkSize) ; i++ {
		if data[i] != readBuf[i] {
			t.Fatalf("Different byte at position %d between original data %x and written data %x", i, data, readBuf)
		}
	}
}

func TestUpdateMetadata(t *testing.T) {
	IP := "127.0.0.200"
	port := 6666
	URI := "foo"
	chunkSize := uint16(8)
	maxChunksInACR := uint16(10)
	fileID := uint32(0x1337c001)
	data := []byte("Not important")
	quit := make(chan bool)

    conn_server, err := messages.CreateServerSocket(IP, port)
    if err != nil{
        t.Fatalf(`Creating server failed: %v`, err)
    }
	defer conn_server.Close()

	go startMockServer(quit, conn_server, URI, chunkSize, maxChunksInACR, fileID, data)
	defer func() {fmt.Println("Closing server...");quit <- true}()

    conn_client, err := messages.CreateClientSocket(IP, port)
    if err != nil{
        t.Fatalf(`Creating client failed: %v`, err)
    }
	defer conn_client.Close()

	metadata := new(fileMetadata)
	metadata.timeout = 3*time.Second
	metadata.url = URI

	err = updateMetadata(conn_client, metadata)
	if err != nil {
		t.Fatalf("updateMetadata failed: %v", err)
	}

	if metadata.fileID != fileID {
		t.Fatalf("Invalid fileID. Expected %x got %x", fileID, metadata.fileID)
	}
	if metadata.chunkSize != chunkSize {
		t.Fatalf("Invalid chunkSize. Expected %d got %d", chunkSize, metadata.chunkSize)
	}
	if metadata.maxChunksInACR != maxChunksInACR {
		t.Fatalf("Invalid maxChunksInACR. Expected %d got %d", maxChunksInACR, metadata.maxChunksInACR)
	}

	metadata.chunkMap[2] = true
	// Simulate a file ID change and a new metadata request
	metadata.fileID = 0xf00dbad1
	err = updateMetadata(conn_client, metadata)
	if err != nil {
		t.Fatalf("updateMetadata failed: %v", err)
	}
	if metadata.fileID != fileID {
		t.Fatalf("Invalid fileID. Expected %x got %x", fileID, metadata.fileID)
	}
	if metadata.chunkSize != chunkSize {
		t.Fatalf("Invalid chunkSize. Expected %d got %d", chunkSize, metadata.chunkSize)
	}
	if metadata.maxChunksInACR != maxChunksInACR {
		t.Fatalf("Invalid maxChunksInACR. Expected %d got %d", maxChunksInACR, metadata.maxChunksInACR)
	}
	if metadata.chunkMap[2] {
		t.Fatal("chunkMap was not reset after fileID change.")
	}
}

func TestUpdateMetadataNotFound(t *testing.T) {
	IP := "127.0.0.200"
	port := 6666
	URI := "foo"
	wrongURI := "notfoo"
	chunkSize := uint16(8)
	maxChunksInACR := uint16(10)
	fileID := uint32(0x1337c001)
	data := []byte("Not important")
	want := regexp.MustCompile(`not found`)
	quit := make(chan bool)

    conn_server, err := messages.CreateServerSocket(IP, port)
    if err != nil{
        t.Fatalf(`Creating server failed: %v`, err)
    }
	defer conn_server.Close()

	go startMockServer(quit, conn_server, URI, chunkSize, maxChunksInACR, fileID, data)
	defer func() {quit <- true}()

    conn_client, err := messages.CreateClientSocket(IP, port)
    if err != nil{
        t.Fatalf(`Creating client failed: %v`, err)
    }
	defer conn_client.Close()

	metadata := new(fileMetadata)
	metadata.timeout = 3*time.Second
	metadata.url = wrongURI

	err = updateMetadata(conn_client, metadata)
	if err == nil {
		t.Fatalf("updateMetadata should have failed. It didn't")
	}
	if !want.MatchString(err.Error()) {
		t.Fatalf("Expected %s in error, got %v", want, err)
	}
}

func TestRequestFile(t *testing.T) {
	IP := "127.0.0.200"
	port := 6666
	URI := "foo"
	filename := "/tmp/sanftTestRequest.dat"
	var tests = []struct {
		name string
		chunkSize uint16
		maxChunksInACR uint16
		fileID uint32
		dataSize int
	}{
		{"filesize is less than chunk size", 64, 10, 0x00facade, 20},
		{"filesize is a multiple of chunk size", 32, 4, 0x1337c001, 256},
		{"maxChunksINACR = 1", 64, 1, 0xc0de600d, 223},
		{"File ID is 0", 54, 10, 0x00000000, 45},
		{"Large file", 0x100, 20, 0xb16f11e, 0x4abcd},
	}

	conn_server, err := messages.CreateServerSocket(IP, port)
	if err != nil{
		t.Fatalf(`Creating server failed: %v`, err)
	}
	defer conn_server.Close()

	for _, tt := range tests {
		data := make([]byte, tt.dataSize)
		_, err := rand.Read(data)
		if err != nil {
			t.Errorf("Could not read random data")
		}
		t.Run(tt.name, func(t *testing.T) {
			quit := make(chan bool)

			go startMockServer(quit, conn_server, URI, tt.chunkSize, tt.maxChunksInACR, tt.fileID, data)
			defer func() {quit <- true}()

			err = RequestFile(IP, port, URI, filename)
			if err != nil {
				t.Fatalf("RequestFile failed : %v", err)
			}

			// Check that the file exists and that the data is correct
			fileData, err := os.ReadFile(filename)
			if err != nil {
				t.Fatalf("Could not open created file: %v", err)
			}
			defer os.Remove(filename)

			if len(fileData) != len(data) {
				t.Fatalf("The received file doesn't have the right length. Expected %d got %d", len(data), len(fileData))
			}

			for i,b := range fileData {
				if b != data[i] {
					t.Fatalf("The received data and sent data differ at position %d. Sent: %x; Received: %x", i, data, fileData)
				}
			}
		})
	}
}
