package server

import (
	"encoding/hex"
	"net"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"gitlab.lrz.de/protocol-design-sose-2022-team-0/sanft/messages"
)

func TestToken(t *testing.T) {
	s, err := Init(net.ParseIP("127.0.0.1"), 10000, "/", 1024, 1, 0, 0, 0)
	if err != nil {
		t.Fatalf(`Error creating server: %v`, err)
	}

	addr := net.UDPAddr{
		Port: 1000,
		IP:   net.ParseIP("127.100.0.1"),
	}
	token := s.createToken(&addr)
	assert.True(t, s.checkToken(&addr, &token), "Token miss match")

	var false_token [32]uint8
	assert.False(t, s.checkToken(&addr, &false_token), "Token match but should miss match")

	addr2 := net.UDPAddr{
		Port: 1001,
		IP:   net.ParseIP("127.100.0.1"),
	}
	// slighly changed address should also result into check=false
	assert.False(t, s.checkToken(&addr2, &token), "Token match but should miss match")
}

func TestMDR(t *testing.T) {
	s, err := Init(net.ParseIP("127.0.0.100"), 12345, "./", 20, 10, 0, 0, 0)
	if err != nil {
		t.Fatalf(`Error creating server: %v`, err)
	}
	defer s.Conn.Close()

	c, err := messages.CreateClientSocket(net.ParseIP("127.0.0.100"), 12345)
	if err != nil {
		t.Fatalf(`Creating client failed: %v`, err)
	}
	defer c.Close()

	close := make(chan bool)
	go s.Listen(close)
	defer s.StopListening(close)

	msg := messages.GetMDR(0, messages.EmptyToken(), "test.txt")
	msg.Send(c)

	msgr, err := messages.ClientReceive(c, 10000)
	if err != nil {
		t.Fatalf(`Client Receive failed: %v`, err)
	}
	parsed, err := messages.ParseServer(&msgr)
	if err != nil {
		t.Fatalf(`parse failed: %v`, err)
	}
	// should be NTM message
	var ntm messages.NTM = parsed.(messages.NTM)
	token := ntm.Token

	assert.Equal(t, ntm.Header.Number, msg.Header.Number, "Header number should match")
	assert.Equal(t, ntm.Header.Version, messages.VERS, "Returned wrong version")
	assert.Equal(t, ntm.Header.Error, messages.NoError, "There should be no error type set")

	// ask for the file again
	msg = messages.GetMDR(1, &token, "test.txt")
	msg.Send(c)

	msgr, err = messages.ClientReceive(c, 10000)
	if err != nil {
		t.Fatalf(`Client Receive failed: %v`, err)
	}
	parsed, err = messages.ParseServer(&msgr)
	if err != nil {
		t.Fatalf(`parse failed: %v`, err)
	}
	// should be MDRR message
	var mdrr messages.MDRR = parsed.(messages.MDRR)
	assert.Equal(t, mdrr.Header.Number, msg.Header.Number, "Header number should match")
	assert.Equal(t, mdrr.Header.Version, messages.VERS, "Returned wrong version")
	assert.Equal(t, mdrr.Header.Error, messages.NoError, "There should be no error type set")

	assert.Equal(t, mdrr.ChunkSize, s.ChunkSize, "Wrong chunksize")
	assert.Equal(t, mdrr.MaxChunksInACR, s.MaxChunksInACR, "Wrong maxchunksinacr")

	fileid := mdrr.FileID

	assert.Equal(t, int64(messages.Uint8_6_arr2Int(mdrr.FileSize)), Ceil(676, int64(s.ChunkSize)), "wrong size")

	ch, _ := hex.DecodeString("3c61b3311004a65a70fd313afb943c94ac8dfaae8a00000efe85db25d9e288f1")

	ch2 := (*[32]uint8)(ch)

	assert.Equal(t, mdrr.Checksum, *ch2, "wrong checksum")

	// test possbile errors: file not exists, id already taken, map full
	msg = messages.GetMDR(1, &token, "does_not_exists.txt")
	msg.Send(c)

	msgr, err = messages.ClientReceive(c, 10000)
	if err != nil {
		t.Fatalf(`Client Receive failed: %v`, err)
	}
	parsed, err = messages.ParseServer(&msgr)
	if err != nil {
		t.Fatalf(`parse failed: %v`, err)
	}
	// should be header message
	var header messages.ServerHeader = parsed.(messages.ServerHeader)
	assert.Equal(t, header.Number, msg.Header.Number, "Header number should match")
	assert.Equal(t, header.Version, messages.VERS, "Returned wrong version")
	assert.Equal(t, header.Error, messages.FileNotFound, "Should be file not found error")
	assert.Equal(t, header.Type, messages.MDRR_t, "Should be mdrr messages")

	// id already taken
	s.FileIDMap[fileid] = FileM{}
	msg = messages.GetMDR(1, &token, "test.txt")
	msg.Send(c)
	msgr, err = messages.ClientReceive(c, 10000)
	if err != nil {
		t.Fatalf(`Client Receive failed: %v`, err)
	}
	parsed, err = messages.ParseServer(&msgr)
	if err != nil {
		t.Fatalf(`parse failed: %v`, err)
	}
	// should be MDRR message
	mdrr = parsed.(messages.MDRR)
	assert.Equal(t, int64(messages.Uint8_6_arr2Int(mdrr.FileSize)), Ceil(676, int64(s.ChunkSize)), "wrong size")
	ch, _ = hex.DecodeString("3c61b3311004a65a70fd313afb943c94ac8dfaae8a00000efe85db25d9e288f1")
	ch2 = (*[32]uint8)(ch)
	assert.Equal(t, mdrr.Checksum, *ch2, "wrong checksum")
	assert.True(t, s.FileIDMap[mdrr.FileID].Try >= 1, "try should be at least 1")

	// map completely full -> out of ram :(
	// for i := 0; i < 2<<32; i++ {
	// 	s.FileIDMap[uint32(i)] = FileM{}
	// }

	// simulate by filling all "try" slots
	mypath := s.FileIDMap[mdrr.FileID].Path
	mytime := s.FileIDMap[mdrr.FileID].T
	for i := 0; i < 10; i++ {
		fid, _ := GetFileID(mypath, mytime, i)
		s.FileIDMap[fid] = FileM{}
	}

	msg = messages.GetMDR(1, &token, "test.txt")
	msg.Send(c)
	msgr, err = messages.ClientReceive(c, 10000)
	if err != nil {
		t.Fatalf(`Client Receive failed: %v`, err)
	}
	parsed, err = messages.ParseServer(&msgr)
	if err != nil {
		t.Fatalf(`parse failed: %v`, err)
	}
	// should be MDRR message
	mdrr = parsed.(messages.MDRR)
	assert.Equal(t, int64(messages.Uint8_6_arr2Int(mdrr.FileSize)), Ceil(676, int64(s.ChunkSize)), "wrong size")
	ch, _ = hex.DecodeString("3c61b3311004a65a70fd313afb943c94ac8dfaae8a00000efe85db25d9e288f1")
	ch2 = (*[32]uint8)(ch)
	assert.Equal(t, mdrr.Checksum, *ch2, "wrong checksum")
	assert.True(t, s.FileIDMap[mdrr.FileID].Try >= 9, "try should be at least 1")
	_, ok := s.FileIDMap[0]
	assert.False(t, ok, "map should be emptied -> zero id should not be in the map any more")

}

func TestACR(t *testing.T) {
	s, err := Init(net.ParseIP("127.0.0.100"), 12345, "./", 20, 4, 0, 0, 0)
	if err != nil {
		t.Fatalf(`Error creating server: %v`, err)
	}
	defer s.Conn.Close()

	c, err := messages.CreateClientSocket(net.ParseIP("127.0.0.100"), 12345)
	if err != nil {
		t.Fatalf(`Creating client failed: %v`, err)
	}
	defer c.Close()

	close := make(chan bool)
	go s.Listen(close)
	defer s.StopListening(close)

	msg := messages.GetMDR(0, messages.EmptyToken(), "test.txt")
	msg.Send(c)

	msgr, err := messages.ClientReceive(c, 10000)
	if err != nil {
		t.Fatalf(`Client Receive failed: %v`, err)
	}
	parsed, err := messages.ParseServer(&msgr)
	if err != nil {
		t.Fatalf(`parse failed: %v`, err)
	}
	// should be NTM message
	var ntm messages.NTM = parsed.(messages.NTM)
	token := ntm.Token

	assert.Equal(t, ntm.Header.Number, msg.Header.Number, "Header number should match")
	assert.Equal(t, ntm.Header.Version, messages.VERS, "Returned wrong version")
	assert.Equal(t, ntm.Header.Error, messages.NoError, "There should be no error type set")

	// ask for the file again
	msg = messages.GetMDR(1, &token, "test.txt")
	msg.Send(c)

	msgr, err = messages.ClientReceive(c, 10000)
	if err != nil {
		t.Fatalf(`Client Receive failed: %v`, err)
	}
	parsed, err = messages.ParseServer(&msgr)
	if err != nil {
		t.Fatalf(`parse failed: %v`, err)
	}
	// should be MDRR message
	var mdrr messages.MDRR = parsed.(messages.MDRR)
	assert.Equal(t, mdrr.Header.Number, msg.Header.Number, "Header number should match")
	assert.Equal(t, mdrr.Header.Version, messages.VERS, "Returned wrong version")
	assert.Equal(t, mdrr.Header.Error, messages.NoError, "There should be no error type set")

	assert.Equal(t, mdrr.ChunkSize, s.ChunkSize, "Wrong chunksize")
	assert.Equal(t, mdrr.MaxChunksInACR, s.MaxChunksInACR, "Wrong maxchunksinacr")

	fileid := mdrr.FileID

	assert.Equal(t, int64(messages.Uint8_6_arr2Int(mdrr.FileSize)), Ceil(676, int64(s.ChunkSize)), "wrong size")

	ch, _ := hex.DecodeString("3c61b3311004a65a70fd313afb943c94ac8dfaae8a00000efe85db25d9e288f1")

	ch2 := (*[32]uint8)(ch)

	assert.Equal(t, mdrr.Checksum, *ch2, "wrong checksum")

	crlist := make([]messages.CR, 2)
	crlist[0] = messages.CR{ChunkOffset: *messages.Int2uint8_6_arr(0), Length: 2}
	crlist[1] = messages.CR{ChunkOffset: *messages.Int2uint8_6_arr(2), Length: 1}

	msgacr := messages.GetACR(1, &token, fileid, 1, &crlist)
	msgacr.Send(c)

	parsed_arr := make([]messages.ServerMessage, 3)
	times := make([]time.Time, 3)

	for i := 0; i < 3; i++ {
		msgr, err = messages.ClientReceive(c, 10000)
		times[i] = time.Now()

		if err != nil {
			t.Fatalf(`Client Receive failed: %v`, err)
		}
		parsed_arr[i], err = messages.ParseServer(&msgr)
		if err != nil {
			t.Fatalf(`parse failed: %v`, err)
		}
	}
	// server should send no further packet
	_, err = messages.ClientReceive(c, 200)
	if !os.IsTimeout(err) {
		t.Fatalf(`Server should send no further packets`)
	}
	// check that durations match the expected sending rate
	for i := 0; i < 2; i++ {
		dur := times[i+1].Sub(times[i])
		assert.True(t, dur < 1100*time.Millisecond, "sending rate too low")
		assert.True(t, dur > 900*time.Millisecond, "sending rate too high")
	}
	crrs := make([]messages.CRR, 3)
	for i := 0; i < 3; i++ {
		// all messages should be crrs
		crrs[i] = parsed_arr[i].(messages.CRR)
		assert.Equal(t, crrs[i].Header.Number, msgacr.Header.Number, "Header number should match")
		assert.Equal(t, crrs[i].Header.Version, messages.VERS, "Returned wrong version")
		assert.Equal(t, crrs[i].Header.Error, messages.NoError, "there should be no error")
	}

	// check chunk content
	assert.Equal(t, messages.Uint8_6_arr2Int(crrs[0].ChunkNumber), uint64(0), "wrong chunk number for chunk 0")
	assert.Equal(t, string(crrs[0].Data), "Lorem ipsum dolor si", "wrong chunk content for chunk 0")

	assert.Equal(t, messages.Uint8_6_arr2Int(crrs[1].ChunkNumber), uint64(1), "wrong chunk number for chunk 1")
	assert.Equal(t, string(crrs[1].Data), "t amet, consectetur ", "wrong chunk content for chunk 1")

	assert.Equal(t, messages.Uint8_6_arr2Int(crrs[2].ChunkNumber), uint64(2), "wrong chunk number for chunk 2")
	assert.Equal(t, string(crrs[2].Data), "adipiscing elit, sed", "wrong chunk content for chunk 2")

	// wrong token

	crlist = make([]messages.CR, 1)
	crlist[0] = messages.CR{ChunkOffset: *messages.Int2uint8_6_arr(0), Length: 1}

	msgacr = messages.GetACR(1, messages.EmptyToken(), fileid, 1, &crlist)
	msgacr.Send(c)

	msgr, err = messages.ClientReceive(c, 10000)
	if err != nil {
		t.Fatalf(`Client Receive failed: %v`, err)
	}
	parsed, err = messages.ParseServer(&msgr)
	if err != nil {
		t.Fatalf(`parse failed: %v`, err)
	}
	// should be NTM message
	ntm = parsed.(messages.NTM)
	assert.Equal(t, ntm.Header.Number, msgacr.Header.Number, "Header number should match")
	assert.Equal(t, ntm.Header.Version, messages.VERS, "Returned wrong version")
	assert.Equal(t, ntm.Header.Error, messages.NoError, "There should be no error type set")
	assert.Equal(t, ntm.Token, token, "token should match the previous")

	// wrong file id
	msgacr = messages.GetACR(1, &token, 0, 1, &crlist)
	msgacr.Send(c)

	msgr, err = messages.ClientReceive(c, 10000)
	if err != nil {
		t.Fatalf(`Client Receive failed: %v`, err)
	}
	parsed, err = messages.ParseServer(&msgr)
	if err != nil {
		t.Fatalf(`parse failed: %v`, err)
	}
	// should be Server header message
	header := parsed.(messages.ServerHeader)

	assert.Equal(t, header.Number, msgacr.Header.Number, "Header number should match")
	assert.Equal(t, header.Version, messages.VERS, "Returned wrong version")
	assert.Equal(t, header.Error, messages.InvalidFileID, "error should be invalid file id")

	// too many chunks requested in one CR
	crlist = make([]messages.CR, 1)
	crlist[0] = messages.CR{ChunkOffset: *messages.Int2uint8_6_arr(0), Length: 5}
	msgacr = messages.GetACR(1, &token, fileid, 1, &crlist)
	msgacr.Send(c)

	// first 4 messages should work
	for i := 0; i < 4; i++ {
		messages.ClientReceive(c, 10000)
	}

	// 5th should given an error
	msgr, err = messages.ClientReceive(c, 10000)
	if err != nil {
		t.Fatalf(`Client Receive failed: %v`, err)
	}
	parsed, err = messages.ParseServer(&msgr)
	if err != nil {
		t.Fatalf(`parse failed: %v`, err)
	}
	// should be Server header message
	header = parsed.(messages.ServerHeader)

	assert.Equal(t, header.Number, msgacr.Header.Number, "Header number should match")
	assert.Equal(t, header.Version, messages.VERS, "Returned wrong version")
	assert.Equal(t, header.Error, messages.TooManyChunks, "error should be too many chunks")

	// too many chunks requested in single CRs
	crlist = make([]messages.CR, 5)
	for i := 0; i < 5; i++ {
		crlist[i] = messages.CR{ChunkOffset: *messages.Int2uint8_6_arr(uint64(i)), Length: 1}
	}
	msgacr = messages.GetACR(1, &token, fileid, 1, &crlist)
	msgacr.Send(c)

	// first 4 messages should work
	for i := 0; i < 4; i++ {
		messages.ClientReceive(c, 10000)
	}

	// 5th should given an error
	msgr, err = messages.ClientReceive(c, 10000)
	if err != nil {
		t.Fatalf(`Client Receive failed: %v`, err)
	}
	parsed, err = messages.ParseServer(&msgr)
	if err != nil {
		t.Fatalf(`parse failed: %v`, err)
	}
	// should be Server header message
	header = parsed.(messages.ServerHeader)

	assert.Equal(t, header.Number, msgacr.Header.Number, "Header number should match")
	assert.Equal(t, header.Version, messages.VERS, "Returned wrong version")
	assert.Equal(t, header.Error, messages.TooManyChunks, "error should be too many chunks")

	// zero length
	crlist = make([]messages.CR, 1)
	crlist[0] = messages.CR{ChunkOffset: *messages.Int2uint8_6_arr(0), Length: 0}
	msgacr = messages.GetACR(1, &token, fileid, 1, &crlist)
	msgacr.Send(c)

	msgr, err = messages.ClientReceive(c, 10000)
	if err != nil {
		t.Fatalf(`Client Receive failed: %v`, err)
	}
	parsed, err = messages.ParseServer(&msgr)
	if err != nil {
		t.Fatalf(`parse failed: %v`, err)
	}
	// should be Server header message
	header = parsed.(messages.ServerHeader)

	assert.Equal(t, header.Number, msgacr.Header.Number, "Header number should match")
	assert.Equal(t, header.Version, messages.VERS, "Returned wrong version")
	assert.Equal(t, header.Error, messages.ZeroLengthCR, "error should be zero length")

	// chunk out of bounds
	crlist = make([]messages.CR, 1)
	crlist[0] = messages.CR{ChunkOffset: *messages.Int2uint8_6_arr(100), Length: 1}
	msgacr = messages.GetACR(1, &token, fileid, 1, &crlist)
	msgacr.Send(c)

	msgr, err = messages.ClientReceive(c, 10000)
	if err != nil {
		t.Fatalf(`Client Receive failed: %v`, err)
	}

	// fails as chunkoutofbounds means that crs are somehow included
	parsed, err = messages.ParseServer(&msgr)
	if err != nil {
		t.Fatalf(`parse failed: %v`, err)
	}
	// should be Server header message
	crr := parsed.(messages.CRR)

	assert.Equal(t, crr.Header.Number, msgacr.Header.Number, "Header number should match")
	assert.Equal(t, crr.Header.Version, messages.VERS, "Returned wrong version")
	assert.Equal(t, crr.Header.Error, messages.ChunkOutOfBounds, "error should be too many chunks")
	cn := messages.Int2uint8_6_arr(100)
	assert.Equal(t, crr.ChunkNumber, *cn, "chunk number should be")
	assert.Equal(t, string(crr.Data), "", "chunk number should be")

	// correct file id but file modified
	// modify file
	dat, _ := os.ReadFile("test.txt")
	f, _ := os.Create("test.txt")
	f.Write(dat)
	f.Close()

	// fileid should still exist in server
	_, ok := s.FileIDMap[fileid]
	assert.True(t, ok, "file id should still be in server map")

	msgacr = messages.GetACR(1, &token, fileid, 1, &crlist)
	msgacr.Send(c)

	msgr, err = messages.ClientReceive(c, 10000)
	if err != nil {
		t.Fatalf(`Client Receive failed: %v`, err)
	}
	parsed, err = messages.ParseServer(&msgr)
	if err != nil {
		t.Fatalf(`parse failed: %v`, err)
	}
	// should be Server header message
	header = parsed.(messages.ServerHeader)
	assert.Equal(t, header.Number, msgacr.Header.Number, "Header number should match")
	assert.Equal(t, header.Version, messages.VERS, "Returned wrong version")
	assert.Equal(t, header.Error, messages.InvalidFileID, "error should be invalid file id")

	_, ok = s.FileIDMap[fileid]
	assert.False(t, ok, "file id should now be deleted from server map")

}
