package messages

import (
	"fmt"
	"net"
	"os"
	"testing"
	"math/rand"
	"github.com/stretchr/testify/assert"
)

// func createStaticToken() *[256]uint8{
//     token := new([256]uint8)
//     for idx := range token{
//         token[idx] = uint8(idx)
//     }
//     return token
// }

func createRandomToken() *[256]uint8{
    token := make([]uint8, 256)
    rand.Read(token)
    // slice to array
    return (*[256]uint8)(token)
}

func createTestServerAndClient(t *testing.T) (*net.UDPConn, *net.UDPConn, *net.UDPAddr){
    conn_server, err := CreateServerSocket("127.0.0.100", 12345)
    if err != nil{
        t.Fatalf(`Creating server failed: %v`, err)
    }

    conn_client, err := CreateClientSocket("127.0.0.100", 12345)
    if err != nil{
        t.Fatalf(`Creating client failed: %v`, err)
    }
    // send test message to get client ip

    msg := GetMDR(0, createRandomToken(), "some/thing")
    // TODO: I find this msg.send unintuative and would rather prefer something like
    // client.send(msg)
    err = msg.Send(conn_client)
    if err != nil{
        t.Fatalf(`Error while sending message to server: %v`, err)
    }
    addr, data, err := ServerReceive(conn_server)
    if err != nil{
        t.Fatalf(`Error while receiving on server: %v`, err)
    }
    // parse message
    msgr, err := ParseClient(&data)
    if err != nil{
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

func TestMDR(t *testing.T){
    conn_server, conn_client, _ := createTestServerAndClient(t)
    defer conn_client.Close()
    defer conn_server.Close()
}

func TestNTM(t *testing.T){
    conn_server, conn_client, addr := createTestServerAndClient(t)
    defer conn_client.Close()
    defer conn_server.Close()


    msg := GetNTM(0, NoError, createRandomToken())
    // TODO: I find this msg.send unintuative and would rather prefer something like
    // client.send(msg)
    err := msg.Send(conn_server, addr)
    if err != nil{
        t.Fatalf(`Error while sending message to client: %v`, err)
    }
    data, err := ClientReceive(conn_client, 10000)
    if err != nil{
        t.Fatalf(`Error while receiving on client: %v`, err)
    }
    // parse message
    msgr, err := ParseServer(&data)
    if err != nil{
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

func TestMDRR(t *testing.T){
    conn_server, conn_client, addr := createTestServerAndClient(t)
    defer conn_client.Close()
    defer conn_server.Close()

    msg := GetMDRR(0, NoError, 8, 200, 12345, *Int2uint8_6_arr(123), createRandomToken())
    // TODO: I find this msg.send unintuative and would rather prefer something like
    // client.send(msg)
    err := msg.Send(conn_server, addr)
    if err != nil{
        t.Fatalf(`Error while sending message to client: %v`, err)
    }
    data, err := ClientReceive(conn_client, 10000)
    if err != nil{
        t.Fatalf(`Error while receiving on client: %v`, err)
    }
    // parse message
    msgr, err := ParseServer(&data)
    if err != nil{
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

func TestACR(t *testing.T){
    conn_server, conn_client, _ := createTestServerAndClient(t)
    defer conn_client.Close()
    defer conn_server.Close()

	cr1 := CR{ChunkOffset: *Int2uint8_6_arr(32), Length: 5}
	cr2 := CR{ChunkOffset: *Int2uint8_6_arr(42), Length: 6}
    crl := []CR{cr1, cr2}

    msg := GetACR(2, createRandomToken(), 42, 21, &crl)
    err := msg.Send(conn_client)
    if err != nil{
        t.Fatalf(`Error while sending message to server: %v`, err)
    }
    _, data, err := ServerReceive(conn_server)
    if err != nil{
        t.Fatalf(`Error while receiving on server: %v`, err)
    }
    // parse message
    msgr, err := ParseClient(&data)
    if err != nil{
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

func TestCRR(t *testing.T){
    conn_server, conn_client, addr := createTestServerAndClient(t)
    defer conn_client.Close()
    defer conn_server.Close()


    sdata := []uint8("asdf")
    msg := GetCRR(2, 9, *Int2uint8_6_arr(42), &sdata)
    err := msg.Send(conn_server, addr)
    if err != nil{
        t.Fatalf(`Error while sending message to client: %v`, err)
    }
    data, err := ClientReceive(conn_client, 10000)
    if err != nil{
        t.Fatalf(`Error while receiving on client: %v`, err)
    }
    // parse message
    msgr, err := ParseServer(&data)
    if err != nil{
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

func TestTimeout(t *testing.T){
    conn_server, conn_client, _ := createTestServerAndClient(t)
    defer conn_client.Close()
    defer conn_server.Close()

    _, err := ClientReceive(conn_client, 100)

    if !os.IsTimeout(err){
        t.Fatalf(`This receive should actually time out!`)
    }
    if err, ok := err.(net.Error); !ok || !err.Timeout() {
        // this for does the same as above
        t.Fatalf(`This receive should actually time out!`)
    }
    if err == nil{
        t.Fatalf(`This receive should actually time out!`)
    }
}
