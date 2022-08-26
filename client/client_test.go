package client

import (
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"os"
	"testing"
	"time"

	"gitlab.lrz.de/protocol-design-sose-2022-team-0/sanft/messages"
)


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
	metadata.fileSize = 25;
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
	for _,cr := range acr.CRs {
		offset := messages.Uint8_6_arr2int(&cr.ChunkOffset)
		length := cr.Length
		if length == 0 {
			t.Fatalf("Invalid length (0) for chunk request in %v", acr)
		}
		for i:=uint64(0); i<uint64(length); i++ {
			in_acr[offset + i] = true
			if offset+i > metadata.fileSize {
				t.Fatalf("Request for an out of bound chunk %d in ACR %v", offset+i, acr)
			}
			if metadata.chunkMap[offset+i] {
				t.Fatalf("Unexpected chunk request for already received chunk %d in ACR %v", offset+i, acr)
			}
			n_requests ++
		}
	}
	if n_requests > int(metadata.maxChunksInACR) {
		t.Fatalf("Too many chunks in chunk request : %d (max %d)", n_requests, metadata.maxChunksInACR)
	}

	// Check that requested matches the id of the chunks requested in the ACR:
	for _,r := range requested {
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

	for i:=0; i<n_expected; i++ {
		timeMap[i] = t0.Add(time.Duration(i*int(time.Second)/int(prevRate)))
	}

	newRate, err := computePacketRate(timeMap, n_expected, prevRate);
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if newRate == 0 {
		t.Fatalf("packetRate cannot be zero")
	}

	if newRate != prevRate {
		t.Fatalf("New packetRate (%d) differ from previous packetRate (%d)", newRate, prevRate)
	}
}

func TestComputePacketRateMissing(t *testing.T) {
	n_expected := 324
	timeMap := make(map[int]time.Time)
	prevRate := uint32(2000)
	t0 := time.Now()

	for _, i := range []int{1,4,23,24,45,46} {
		timeMap[i] = t0.Add(time.Duration(i*int(time.Second)/int(prevRate)))
	}

	newRate, err := computePacketRate(timeMap, n_expected, prevRate);
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

	for i:=0; i<n_expected; i++ {
		timeMap[i] = t0.Add(time.Duration(i*int(time.Second)/int(prevRate)))
	}

	timeMap[0] = t0.Add(3*time.Second)

	newRate, err := computePacketRate(timeMap, n_expected, prevRate);
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
