package messages

import (
	"errors"
	"fmt"
	"github.com/stretchr/testify/assert"
	"math/rand"
	"net"
	"os"
	"testing"
)

// func createStaticToken() *[32]uint8{
//     token := new([32]uint8)
//     for idx := range token{
//         token[idx] = uint8(idx)
//     }
//     return token
// }

func createRandomToken() *[32]uint8 {
	token := make([]uint8, 32)
	rand.Read(token)
	// slice to array
	return (*[32]uint8)(token)
}

func createTestServerAndClient(t *testing.T) (*net.UDPConn, *net.UDPConn, net.Addr) {
	conn_server, err := CreateServerSocket(net.ParseIP("127.0.0.100"), 12345)
	if err != nil {
		t.Fatalf(`Creating server failed: %v`, err)
	}

	conn_client, err := CreateClientSocket(net.ParseIP("127.0.0.100"), 12345)
	if err != nil {
		t.Fatalf(`Creating client failed: %v`, err)
	}
	// send test message to get client ip

	msg := GetMDR(0, createRandomToken(), "some/thing")
	// TODO: I find this msg.send unintuative and would rather prefer something like
	// client.send(msg)
	err = msg.Send(conn_client)
	if err != nil {
		t.Fatalf(`Error while sending message to server: %v`, err)
	}
	addr, data, err := ServerReceive(conn_server, 10000)
	if err != nil {
		t.Fatalf(`Error while receiving on server: %v`, err)
	}
	// parse message
	msgr, err := ParseClient(&data)
	if err != nil {
		t.Fatalf(`Error while parsing clients message: %v`, err)
	}
	// type assertion
	var mdr MDR = msgr.(MDR)
	// sanity check received data
	assert.Equal(t, mdr.Header.Number, msg.Header.Number, "Header number missmatch")
	assert.Equal(t, mdr.Header.Token, msg.Header.Token, "Header token missmatch")
	assert.Equal(t, mdr.Header.Type, msg.Header.Type, "Header type missmatch")
	assert.Equal(t, mdr.Header.Version, msg.Header.Version, "Header version missmatch")
	assert.Equal(t, mdr.URI, msg.URI, "URI missmatch")

	fmt.Printf("Client IP address: %v\n", addr)

	return conn_server, conn_client, addr
}

// test protocol messages

func TestMDR(t *testing.T) {
	conn_server, conn_client, _ := createTestServerAndClient(t)
	defer conn_client.Close()
	defer conn_server.Close()
}

func TestServerError(t *testing.T) {
	conn_server, conn_client, addr := createTestServerAndClient(t)
	defer conn_client.Close()
	defer conn_server.Close()

	// send some error
	msg := ServerHeader{Version: VERS, Type: MDRR_t, Number: 0, Error: FileNotFound}

	err := msg.Send(conn_server, addr)
	if err != nil {
		t.Fatalf(`Error while sending message to client: %v`, err)
	}
	data, err := ClientReceive(conn_client, 10000)
	if err != nil {
		t.Fatalf(`Error while receiving on client: %v`, err)
	}
	// parse message
	msgr, err := ParseServer(&data)
	if err != nil {
		t.Fatalf(`Error while parsing server message: %v`, err)
	}
	// type assertion
	var header ServerHeader = msgr.(ServerHeader)
	// sanity check received data
	assert.Equal(t, header.Number, msg.Number, "Header number missmatch")
	assert.Equal(t, header.Error, msg.Error, "Header error missmatch")
	assert.Equal(t, header.Type, msg.Type, "Header type missmatch")
	assert.Equal(t, header.Version, msg.Version, "Header version missmatch")
}

func TestNTM(t *testing.T) {
	conn_server, conn_client, addr := createTestServerAndClient(t)
	defer conn_client.Close()
	defer conn_server.Close()

	msg := GetNTM(0, NoError, createRandomToken())
	// TODO: I find this msg.send unintuative and would rather prefer something like
	// client.send(msg)
	err := msg.Send(conn_server, addr)
	if err != nil {
		t.Fatalf(`Error while sending message to client: %v`, err)
	}
	data, err := ClientReceive(conn_client, 10000)
	if err != nil {
		t.Fatalf(`Error while receiving on client: %v`, err)
	}
	// parse message
	msgr, err := ParseServer(&data)
	if err != nil {
		t.Fatalf(`Error while parsing server message: %v`, err)
	}
	// type assertion
	var ntm NTM = msgr.(NTM)
	// sanity check received data
	assert.Equal(t, ntm.Header.Number, msg.Header.Number, "Header number missmatch")
	assert.Equal(t, ntm.Header.Error, msg.Header.Error, "Header error missmatch")
	assert.Equal(t, ntm.Header.Type, msg.Header.Type, "Header type missmatch")
	assert.Equal(t, ntm.Header.Version, msg.Header.Version, "Header version missmatch")
	assert.Equal(t, ntm.Token, msg.Token, "Token missmatch")
}

func TestMDRR(t *testing.T) {
	conn_server, conn_client, addr := createTestServerAndClient(t)
	defer conn_client.Close()
	defer conn_server.Close()

	msg := GetMDRR(0, NoError, 8, 200, 12345, *Int2uint8_6_arr(123), createRandomToken())
	// TODO: I find this msg.send unintuative and would rather prefer something like
	// client.send(msg)
	err := msg.Send(conn_server, addr)
	if err != nil {
		t.Fatalf(`Error while sending message to client: %v`, err)
	}
	data, err := ClientReceive(conn_client, 10000)
	if err != nil {
		t.Fatalf(`Error while receiving on client: %v`, err)
	}
	// parse message
	msgr, err := ParseServer(&data)
	if err != nil {
		t.Fatalf(`Error while parsing server message: %v`, err)
	}
	// type assertion
	var mdrr MDRR = msgr.(MDRR)
	// sanity check received data
	assert.Equal(t, mdrr.Header.Number, msg.Header.Number, "Header number missmatch")
	assert.Equal(t, mdrr.Header.Error, msg.Header.Error, "Header error missmatch")
	assert.Equal(t, mdrr.Header.Type, msg.Header.Type, "Header type missmatch")
	assert.Equal(t, mdrr.Header.Version, msg.Header.Version, "Header version missmatch")

	assert.Equal(t, mdrr.Checksum, msg.Checksum, "checksum missmatch")
	assert.Equal(t, mdrr.ChunkSize, msg.ChunkSize, "chunksize missmatch")
	assert.Equal(t, mdrr.FileID, msg.FileID, "fileid missmatch")
	assert.Equal(t, mdrr.MaxChunksInACR, msg.MaxChunksInACR, "maxchunksinacr missmatch")
	assert.Equal(t, mdrr.FileSize, msg.FileSize, "filesize missmatch")
}

func TestACR(t *testing.T) {
	conn_server, conn_client, _ := createTestServerAndClient(t)
	defer conn_client.Close()
	defer conn_server.Close()

	cr1 := CR{ChunkOffset: *Int2uint8_6_arr(32), Length: 5}
	cr2 := CR{ChunkOffset: *Int2uint8_6_arr(42), Length: 6}
	crl := []CR{cr1, cr2}

	msg := GetACR(2, createRandomToken(), 42, 21, &crl)
	err := msg.Send(conn_client)
	if err != nil {
		t.Fatalf(`Error while sending message to server: %v`, err)
	}
	_, data, err := ServerReceive(conn_server, 10000)
	if err != nil {
		t.Fatalf(`Error while receiving on server: %v`, err)
	}
	// parse message
	msgr, err := ParseClient(&data)
	if err != nil {
		t.Fatalf(`Error while parsing client message: %v`, err)
	}
	// type assertion
	var acr ACR = msgr.(ACR)
	// sanity check received data
	assert.Equal(t, acr.Header.Number, msg.Header.Number, "Header number missmatch")
	assert.Equal(t, acr.Header.Token, msg.Header.Token, "Header token missmatch")
	assert.Equal(t, acr.Header.Type, msg.Header.Type, "Header type missmatch")
	assert.Equal(t, acr.Header.Version, msg.Header.Version, "Header version missmatch")

	assert.Equal(t, acr.FileID, msg.FileID, "fileid missmatch")
	assert.Equal(t, acr.PacketRate, msg.PacketRate, "packetrate missmatch")
	assert.Equal(t, acr.CRs, msg.CRs, "CRs missmatch")
}

func TestCRR(t *testing.T) {
	conn_server, conn_client, addr := createTestServerAndClient(t)
	defer conn_client.Close()
	defer conn_server.Close()

	sdata := []uint8("asdf")
	msg := GetCRR(2, 0, *Int2uint8_6_arr(42), &sdata)
	err := msg.Send(conn_server, addr)
	if err != nil {
		t.Fatalf(`Error while sending message to client: %v`, err)
	}
	data, err := ClientReceive(conn_client, 10000)
	if err != nil {
		t.Fatalf(`Error while receiving on client: %v`, err)
	}
	// parse message
	msgr, err := ParseServer(&data)
	if err != nil {
		t.Fatalf(`Error while parsing server message: %v`, err)
	}
	// type assertion
	var crr CRR = msgr.(CRR)
	// sanity check received data
	assert.Equal(t, crr.Header.Number, msg.Header.Number, "Header number missmatch")
	assert.Equal(t, crr.Header.Error, msg.Header.Error, "Header error missmatch")
	assert.Equal(t, crr.Header.Type, msg.Header.Type, "Header type missmatch")
	assert.Equal(t, crr.Header.Version, msg.Header.Version, "Header version missmatch")

	assert.Equal(t, crr.ChunkNumber, msg.ChunkNumber, "chunknumber missmatch")
	assert.Equal(t, crr.Data, msg.Data, "data missmatch")
}

// test further stuff

func TestTimeout(t *testing.T) {
	conn_server, conn_client, _ := createTestServerAndClient(t)
	defer conn_client.Close()
	defer conn_server.Close()

	_, err := ClientReceive(conn_client, 100)

	if !os.IsTimeout(err) {
		t.Fatalf(`This receive should actually time out!`)
	}
	if err, ok := err.(net.Error); !ok || !err.Timeout() {
		// this for does the same as above
		t.Fatalf(`This receive should actually time out!`)
	}
	if err == nil {
		t.Fatalf(`This receive should actually time out!`)
	}

	_, _, err = ServerReceive(conn_server, 10)

	if !os.IsTimeout(err) {
		t.Fatalf(`This receive should actually time out: %v`, err)
	}
	if err, ok := err.(net.Error); !ok || !err.Timeout() {
		// this for does the same as above
		t.Fatalf(`This receive should actually time out: %v`, err)
	}
	if err == nil {
		t.Fatalf(`This receive should actually time out!`)
	}
}

// test invalid requests which should lead to parsing errors

func TestPacketHeaderTooSmall(t *testing.T) {
	conn_server, conn_client, addr := createTestServerAndClient(t)
	defer conn_client.Close()
	defer conn_server.Close()

	data := make([]uint8, 2)

	// server header on client side
	_, err := conn_server.WriteTo(data, addr)
	if err != nil {
		t.Fatalf(`Error while sending on server: %v`, err)
	}
	var e *WrongPacketLengthError
	datar, err := ClientReceive(conn_client, 10000)
	if err != nil {
		t.Fatalf(`Error while receiving on client: %v`, err)
	}

	_, err = ParseServer(&datar)
	if !errors.As(err, &e) {
		t.Fatalf(`Error should be WrongPacketLengthError but is: %v`, err)
	}

	// client header on server side
	_, err = conn_client.Write(data)
	if err != nil {
		t.Fatalf(`Error while sending on client: %v`, err)
	}
	_, datar, err = ServerReceive(conn_server, 10000)
	if err != nil {
		t.Fatalf(`Error while receiving on client: %v`, err)
	}

	_, err = ParseClient(&datar)
	if !errors.As(err, &e) {
		t.Fatalf(`Error should be WrongPacketLengthError but is: %v`, err)
	}
}

func TestWrongPacketType(t *testing.T) {
	conn_server, conn_client, addr := createTestServerAndClient(t)
	defer conn_client.Close()
	defer conn_server.Close()

	data := make([]uint8, 4)
	data[1] = 5

	// server header on client side
	_, err := conn_server.WriteTo(data, addr)
	if err != nil {
		t.Fatalf(`Error while sending on server: %v`, err)
	}
	var e *UnsupporedTypeError
	datar, err := ClientReceive(conn_client, 10000)
	if err != nil {
		t.Fatalf(`Error while receiving on client: %v`, err)
	}

	_, err = ParseServer(&datar)
	if !errors.As(err, &e) {
		t.Fatalf(`Error should be WrongPacketLengthError but is: %v`, err)
	}

	// client header on server side
	data = make([]uint8, 35)
	data[1] = 5

	_, err = conn_client.Write(data)
	if err != nil {
		t.Fatalf(`Error while sending on client: %v`, err)
	}
	_, datar, err = ServerReceive(conn_server, 10000)
	if err != nil {
		t.Fatalf(`Error while receiving on client: %v`, err)
	}

	_, err = ParseClient(&datar)
	if !errors.As(err, &e) {
		t.Fatalf(`Error should be WrongPacketLengthError but is: %v`, err)
	}
}

func TestUnsupporedVersion(t *testing.T) {
	conn_server, conn_client, addr := createTestServerAndClient(t)
	defer conn_client.Close()
	defer conn_server.Close()

	data := make([]uint8, 4)
	data[0] = 1

	// server header on client side
	_, err := conn_server.WriteTo(data, addr)
	if err != nil {
		t.Fatalf(`Error while sending on server: %v`, err)
	}
	var e *UnsupporedVersionError
	datar, err := ClientReceive(conn_client, 10000)
	if err != nil {
		t.Fatalf(`Error while receiving on client: %v`, err)
	}

	_, err = ParseServer(&datar)
	if !errors.As(err, &e) {
		t.Fatalf(`Error should be WrongPacketLengthError but is: %v`, err)
	}

	// client header on server side
	data = make([]uint8, 35)
	data[0] = 1

	_, err = conn_client.Write(data)
	if err != nil {
		t.Fatalf(`Error while sending on client: %v`, err)
	}
	_, datar, err = ServerReceive(conn_server, 10000)
	if err != nil {
		t.Fatalf(`Error while receiving on client: %v`, err)
	}

	_, err = ParseClient(&datar)
	if !errors.As(err, &e) {
		t.Fatalf(`Error should be WrongPacketLengthError but is: %v`, err)
	}
}

func TestClientSpecificPacketLength(t *testing.T) {
	conn_server, conn_client, _ := createTestServerAndClient(t)
	defer conn_client.Close()
	defer conn_server.Close()

	data := make([]uint8, 35)
	data[1] = MDR_t

	_, err := conn_client.Write(data)
	if err != nil {
		t.Fatalf(`Error while sending on client: %v`, err)
	}
	_, datar, err := ServerReceive(conn_server, 10000)
	if err != nil {
		t.Fatalf(`Error while receiving on server: %v`, err)
	}

	var e *WrongPacketLengthError
	_, err = ParseClient(&datar)
	if !errors.As(err, &e) {
		t.Fatalf(`Error should be WrongPacketLengthError but is: %v`, err)
	}
	data[1] = ACR_t

	_, err = conn_client.Write(data)
	if err != nil {
		t.Fatalf(`Error while sending on client: %v`, err)
	}
	_, datar, err = ServerReceive(conn_server, 10000)
	if err != nil {
		t.Fatalf(`Error while receiving on server: %v`, err)
	}

	_, err = ParseClient(&datar)
	if !errors.As(err, &e) {
		t.Fatalf(`Error should be WrongPacketLengthError but is: %v`, err)
	}
}
func TestServerSpecificPacketLength(t *testing.T) {
	conn_server, conn_client, addr := createTestServerAndClient(t)
	defer conn_client.Close()
	defer conn_server.Close()

	data := make([]uint8, 4)
	data[1] = NTM_t

	_, err := conn_server.WriteTo(data, addr)
	if err != nil {
		t.Fatalf(`Error while sending on server: %v`, err)
	}
	datar, err := ClientReceive(conn_client, 10000)
	if err != nil {
		t.Fatalf(`Error while receiving on client: %v`, err)
	}

	var e *WrongPacketLengthError
	_, err = ParseServer(&datar)
	if !errors.As(err, &e) {
		t.Fatalf(`Error should be WrongPacketLengthError but is: %v`, err)
	}
	data[1] = MDRR_t

	_, err = conn_server.WriteTo(data, addr)
	if err != nil {
		t.Fatalf(`Error while sending on server: %v`, err)
	}
	datar, err = ClientReceive(conn_client, 10000)
	if err != nil {
		t.Fatalf(`Error while receiving on client: %v`, err)
	}

	_, err = ParseServer(&datar)
	if !errors.As(err, &e) {
		t.Fatalf(`Error should be WrongPacketLengthError but is: %v`, err)
	}
	data[1] = CRR_t

	_, err = conn_server.WriteTo(data, addr)
	if err != nil {
		t.Fatalf(`Error while sending on server: %v`, err)
	}
	datar, err = ClientReceive(conn_client, 10000)
	if err != nil {
		t.Fatalf(`Error while receiving on client: %v`, err)
	}

	_, err = ParseServer(&datar)
	if !errors.As(err, &e) {
		t.Fatalf(`Error should be WrongPacketLengthError but is: %v`, err)
	}
}
