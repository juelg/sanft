package client

import (
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"os"
	"regexp"
	"sync"
	"testing"
	"time"

	"gitlab.lrz.de/protocol-design-sose-2022-team-0/sanft/messages"
)

var testConfig = ClientConfig{
	RetransmissionsMDR: 3,
	InitialPacketRate:  40,
	NCRRsToWait:        3,
	DebugLogger:        log.New(ioutil.Discard, "DEBUG: ", log.LstdFlags),
	InfoLogger:         log.New(ioutil.Discard, "INFO: ", log.LstdFlags),
	WarnLogger:         log.New(os.Stderr, "WARN: ", log.LstdFlags),
}


func startMockServer(quit <-chan bool, conn *net.UDPConn, filename string, chunkSize uint16, maxChunksInACR uint16, fileID uint32, fileData []byte) {
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
				continue
			}
			addrData, err := addr.AddrPort().MarshalBinary()
			if err != nil {
				fmt.Printf("Mock server: can't compute token: %v\n", err)
			}
			h := sha256.New()
			_, err = h.Write(addrData)
			if err != nil {
				fmt.Printf("Mock Server: can't compute token: %v\n", err)
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

func checkFileContains(file *os.File, data []byte) error {
	fileData := make([]byte, len(data)+10)
	n, err := file.ReadAt(fileData, 0)
	if err != io.EOF {
		return fmt.Errorf("reading file data: %v", err)
	}

	if n != len(data) {
		return fmt.Errorf("file contains %d bytes of data. Expected %d", n, len(data))
	}

	for i:=0; i<n; i++ {
		if data[i] != fileData[i] {
			return fmt.Errorf("file data differs from expected data at position %d. Expected %x got %x", i, data, fileData)
		}
	}

	return nil
}

func FuzzBuildACR(f *testing.F) {
	metadata := fileMetadata{}
	metadata.token = [32]uint8{
		0x01, 0x23, 0x45, 0x67, 0x89, 0xab, 0xcd, 0xef,
		0xfe, 0xdc, 0xba, 0x98, 0x76, 0x54, 0x32, 0x01,
		0x00, 0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77,
		0x88, 0x99, 0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff,
	}

	f.Add(uint16(30), uint64(25), []byte{10}, uint32(0xcafebabe))
	f.Fuzz(func(t *testing.T, maxChunksInACR uint16, fileSize uint64, receivedChunks []byte, fileID uint32) {
		metadata.maxChunksInACR = maxChunksInACR
		metadata.chunkMap = make(map[uint64]bool)
		for _,rc := range receivedChunks {
			metadata.chunkMap[uint64(rc)] = true
		}
		metadata.firstMissing = 0
		for metadata.chunkMap[metadata.firstMissing] {
			metadata.firstMissing ++
		}
		metadata.fileSize = fileSize
		metadata.fileID = fileID

		acr, requested := buildACR(&metadata)
		err := checkValidACR(acr, requested, &metadata)
		if err != nil {
			t.Fatalf("Invalid ACR: %v", err)
		}
	})
}

func checkValidACR(acr *messages.ACR, requested []uint64, metadata *fileMetadata) error {
	in_acr := make(map[uint64]bool)
	n_requests := 0

	// Check the metadata
	if acr.Header.Token != metadata.token {
		return fmt.Errorf("Invalid token in ACR. Expected %x got %x", metadata.token, acr.Header.Token)
	}
	if acr.FileID != metadata.fileID {
		return fmt.Errorf("Invalid file ID in ACR. Expected %x got %x", metadata.fileID, acr.FileID)
	}

	// Check that the ACR is valid
	for _, cr := range acr.CRs {
		offset := messages.Uint8_6_arr2int(&cr.ChunkOffset)
		length := cr.Length
		if length == 0 {
			return fmt.Errorf("Invalid length (0) for chunk request in %v", acr)
		}
		for i := uint64(0); i < uint64(length); i++ {
			in_acr[offset+i] = true
			if offset+i > metadata.fileSize {
				return fmt.Errorf("Request for an out of bound chunk %d in ACR %v", offset+i, acr)
			}
			if metadata.chunkMap[offset+i] {
				return fmt.Errorf("Unexpected chunk request for already received chunk %d in ACR %v", offset+i, acr)
			}
			n_requests++
		}
	}
	if n_requests > int(metadata.maxChunksInACR) {
		return fmt.Errorf("Too many chunks in chunk request : %d (max %d)", n_requests, metadata.maxChunksInACR)
	}

	// Check that requested matches the id of the chunks requested in the ACR:
	for _, r := range requested {
		if !in_acr[r] {
			return fmt.Errorf("Chunk %d is in requested but not in ACR", r)
		}
	}
	if len(requested) != n_requests {
		return fmt.Errorf("There is not the same number of chunks in requested (%d) and in ACR(%d)", len(requested), n_requests)
	}

	return nil
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

func FuzzComputePacketRate(f *testing.F) {
	f.Add(10, []byte{0,1,2,3,4,5,6,7,8,9}, []byte{50}, uint32(20))
	f.Add(10, []byte{2,3,4,5,6,7,8,9}, []byte{100}, uint32(10))
	f.Add(10, []byte{1,2,3,4,5,6,7,8,0,9}, []byte{80}, uint32(20))
	// chunksArrived is the list of chunksNumber in the order in which they arrived
	// arrivalTime is the time between each arrival in ms. If the list is not long enough, it loops.
	f.Fuzz(func(t *testing.T, n_expected int, chunksArrived []byte, arrivalTime []byte, prevRate uint32) {
		timeMap := make(map[int]time.Time)
		var tNext time.Time

		// Checks on inputs parameters
		if prevRate == 0 || len(arrivalTime) == 0 || n_expected < 2 || len(chunksArrived) == 0 {
			return
		}

		for i,c := range chunksArrived {
			if int(c) >= n_expected {
				return
			}
			timeMap[i] = tNext
			tNext = tNext.Add(time.Duration(arrivalTime[i%len(arrivalTime)])*time.Millisecond)
		}

		newRate, err := computePacketRate(timeMap, n_expected, prevRate)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		if newRate == 0 {
			t.Fatalf("packetRate cannot be zero")
		}
	})
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
	fileID := uint32(0xc0ffee11)
	data := []byte("Not important")
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
	metadata.url = URI

	err = updateMetadata(conn_client, metadata, &testConfig)
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
	err = updateMetadata(conn_client, metadata, &testConfig)
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
	fileID := uint32(0xbad15bad)
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

	err = updateMetadata(conn_client, metadata, &testConfig)
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
	filename := "/tmp/sanftTestRequest.dat"
	var tests = []struct {
		name string
		URI string
		chunkSize uint16
		maxChunksInACR uint16
		fileID uint32
		dataSize int
	}{
		{"filesize is less than chunk size", "small", 64, 10, 0x00facade, 20},
		{"one char URL", "/", 12, 34, 0x1, 10},
		{"filesize is a multiple of chunk size", "exact", 32, 4, 0x1337c001, 256},
		{"maxChunksINACR = 1", "CR1", 64, 1, 0xc0de600d, 223},
		{"maxChunksINACR = 2", "CR2", 4, 2, 0xf00d0dd, 2225},
		{"large ACRs", "bigACR", 8, 300, 0xb16ac2, 3025},
		{"File ID is 0", "zero", 54, 10, 0x00000000, 45},
		{"Empty file", "empty", 12, 10, 0xc1ea2, 0},
		{"Large file", "large", 0x100, 20, 0xb16f11e, 0x4abcd},
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

			go startMockServer(quit, conn_server, tt.URI, tt.chunkSize, tt.maxChunksInACR, tt.fileID, data)
			defer func() {quit <- true}()

			err = RequestFile(IP, port, tt.URI, filename, &testConfig)
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

func TestConnectionMigration(t *testing.T) {
	IP := "127.0.0.200"
	port := 6666
	URI := "foo"
	chunkSize := uint16(8)
	maxChunksInACR := uint16(2)
	fileID := uint32(0xd1ffc0)
	data := []byte("The rabbit-hole went straight on like a tunnel for some way, and then dipped suddenly down, so suddenly that Alice had not a moment to think about stopping herself before she found herself falling down a very deep well.")
	filename := "/tmp/sanftTest.dat"
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
	metadata.url = URI
	metadata.packetRate = 10

	err = updateMetadata(conn_client, metadata, &testConfig)
	if err != nil {
		t.Fatalf("updateMetadata failed: %v", err)
	}

	metadata.localFile, err = os.Create(filename)
	if err != nil {
		t.Fatalf("Could not open file: %v", err)
	}
	defer metadata.localFile.Close()

	err = getMissingChunks(conn_client, metadata, &testConfig)
	if err != nil {
		t.Fatalf("getMissingChunks failed: %v", err)
	}

	// Connection migration !
    conn_client2, err := messages.CreateClientSocket(IP, port)
    if err != nil{
        t.Fatalf(`Creating client failed: %v`, err)
    }
	defer conn_client2.Close()

	for metadata.firstMissing < metadata.fileSize {
		err = getMissingChunks(conn_client2, metadata, &testConfig)
		if err != nil {
			t.Fatalf("getMissingChunks failed: %v", err)
		}
	}

	err = checkFileContains(metadata.localFile, data)
	if err != nil {
		t.Fatalf("received data differs from sent data: %v", err)
	}
}

func TestFileIDChange(t *testing.T) {
	IP := "127.0.0.200"
	port := 6666
	URI := "alice"
	chunkSize := uint16(8)
	maxChunksInACR := uint16(2)
	fileID1 := uint32(0x600d1)
	data1 := []byte("Down, down, down. Would the fall never come to an end? “I wonder how many miles I’ve fallen by this time?” she said aloud. “I must be getting somewhere near the centre of the earth. Let me see: that would be four thousand miles down, I think—” (for, you see, Alice had learnt several things of this sort in her lessons in the schoolroom, and though this was not a very good opportunity for showing off her knowledge, as there was no one to listen to her, still it was good practice to say it over) “—yes, that’s about the right distance—but then I wonder what Latitude or Longitude I’ve got to?” (Alice had no idea what Latitude was, or Longitude either, but thought they were nice grand words to say.)")
	fileID2 := uint32(0x2bad)
	data2 := []byte("Presently she began again. “I wonder if I shall fall right through the earth! How funny it’ll seem to come out among the people that walk with their heads downward! The Antipathies, I think—” (she was rather glad there was no one listening, this time, as it didn’t sound at all the right word) “—but I shall have to ask them what the name of the country is, you know. Please, Ma’am, is this New Zealand or Australia?” (and she tried to curtsey as she spoke—fancy curtseying as you’re falling through the air! Do you think you could manage it?) “And what an ignorant little girl she’ll think me for asking! No, it’ll never do to ask: perhaps I shall see it written up somewhere.”")
	filename := "/tmp/sanftTest.dat"
	quit := make(chan bool)

    conn_server, err := messages.CreateServerSocket(IP, port)
    if err != nil{
        t.Fatalf(`Creating server failed: %v`, err)
    }
	defer conn_server.Close()
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		startMockServer(quit, conn_server, URI, chunkSize, maxChunksInACR, fileID1, data1)
		wg.Done()
	}()

    conn_client, err := messages.CreateClientSocket(IP, port)
    if err != nil{
        t.Fatalf(`Creating client failed: %v`, err)
    }
	defer conn_client.Close()

	metadata := new(fileMetadata)
	metadata.timeout = 3*time.Second
	metadata.url = URI
	metadata.packetRate = 10

	err = updateMetadata(conn_client, metadata, &testConfig)
	if err != nil {
		t.Fatalf("updateMetadata failed: %v", err)
	}

	metadata.localFile, err = os.Create(filename)
	if err != nil {
		t.Fatalf("Could not open file: %v", err)
	}
	defer metadata.localFile.Close()

	err = getMissingChunks(conn_client, metadata, &testConfig)
	if err != nil {
		t.Fatalf("getMissingChunks failed: %v", err)
	}

	// File Change
	// Quit previous server
	quit <- true
	wg.Wait()
	// Start new server with different file data
	go startMockServer(quit, conn_server, URI, chunkSize, maxChunksInACR, fileID2, data2)

	for metadata.firstMissing < metadata.fileSize {
		err = getMissingChunks(conn_client, metadata, &testConfig)
		if err != nil {
			t.Fatalf("getMissingChunks failed: %v", err)
		}
	}

	err = checkFileContains(metadata.localFile, data2)
	if err != nil {
		t.Fatalf("received data differs from sent data: %v", err)
	}
}

func TestFileDeleted(t *testing.T) {
	IP := "127.0.0.200"
	port := 6666
	URI := "self-destruct"
	chunkSize := uint16(8)
	maxChunksInACR := uint16(2)
	fileID := uint32(0x2badd00d)
	data := []byte("This message will self destruct in 5...4...3...2...1...0!")
	filename := "/tmp/sanftTest.dat"
	want := regexp.MustCompile(`not found`)
	quit := make(chan bool)

    conn_server, err := messages.CreateServerSocket(IP, port)
    if err != nil{
        t.Fatalf(`Creating server failed: %v`, err)
    }
	defer conn_server.Close()
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		startMockServer(quit, conn_server, URI, chunkSize, maxChunksInACR, fileID, data)
		wg.Done()
	}()

    conn_client, err := messages.CreateClientSocket(IP, port)
    if err != nil{
        t.Fatalf(`Creating client failed: %v`, err)
    }
	defer conn_client.Close()

	metadata := new(fileMetadata)
	metadata.timeout = 3*time.Second
	metadata.url = URI
	metadata.packetRate = 10

	err = updateMetadata(conn_client, metadata, &testConfig)
	if err != nil {
		t.Fatalf("updateMetadata failed: %v", err)
	}

	metadata.localFile, err = os.Create(filename)
	if err != nil {
		t.Fatalf("Could not open file: %v", err)
	}
	defer metadata.localFile.Close()

	err = getMissingChunks(conn_client, metadata, &testConfig)
	if err != nil {
		t.Fatalf("getMissingChunks failed: %v", err)
	}

	// File Deletion
	// Quit previous server
	quit <- true
	wg.Wait()
	// Start new server with no file under required URI
	go startMockServer(quit, conn_server, "", chunkSize, maxChunksInACR, 0x0, []byte{})

	err = getMissingChunks(conn_client, metadata, &testConfig)
	if err == nil {
		t.Fatalf("Expected error from getMissingChunks. Got nil.")
	}
	if !want.MatchString(err.Error()) {
		t.Fatalf("Expected %v in getMissingChunks error. Got %v.", want, err)
	}
}
