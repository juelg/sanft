package messages

import (
	"fmt"
	"net"
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