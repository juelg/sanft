package client

import (
	"testing"
	"crypto/rand"
	"crypto/sha256"
	"os"
	"fmt"
)

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
