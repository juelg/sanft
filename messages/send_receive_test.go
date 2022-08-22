package messages

import (
	"fmt"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSendReceive(t *testing.T){
    // create server
    conn_server, err := CreateServerSocket("127.0.0.100", 12345)
    if err != nil{
        t.Fatalf(`Creating server failed: %v`, err)
    }
	defer conn_server.Close()

    conn_client, err := CreateClientSocket("127.0.0.100", 12345)
    if err != nil{
        t.Fatalf(`Creating client failed: %v`, err)
    }

	defer conn_client.Close()

    // conn_client.Write()

    // test MDR
	// Token := [256]uint8{1, 2, 3, 4, 5}
	// sh := ServerHeader{Number: 5}
	// m0 := NTM{Header: sh, Token: Token}

	ch := ClientHeader{Number: 6, Type: 1}
	m1 := MDR{Header: ch, URI: "/test/bla/blub"}
    // TODO: this how we get the remote address from which we should send
    // ad := conn_server.RemoteAddr().(*net.UDPAddr)
    // ad := conn_client.RemoteAddr().(*net.UDPAddr)

	ad := net.UDPAddr{
		Port: 12345,
		IP:   net.ParseIP("127.0.0.100"),
	}


	err = m1.Send(conn_client, &ad)
	// err = m0.Send(conn_server, &ad)
	if err != nil {
		t.Fatalf("Error while sending:  %v", err)
	}

    // TODO: match and check address
    // receive
    _, buf, err := ServerReceive(conn_server)
	if err != nil {
		t.Fatalf("Error while receiving:  %v", err)
	}
    fmt.Printf("%b\n", buf)
    m, err := ParseClient(&buf)

	if err != nil {
		t.Fatalf("Error while receiving:  %v", err)
	}

    mdr := m.(MDR)
    assert.Equal(t, m1.URI, mdr.URI, "URI miss match")
    assert.Equal(t, ch, mdr.Header, "Header miss match")

    



}


// // TestHelloName calls greetings.Hello with a name, checking
// // for a valid return value.
// func TestHelloName(t *testing.T) {
//     name := "Gladys"
//     want := regexp.MustCompile(`\b`+name+`\b`)
//     msg, err := Hello("Gladys")
//     if !want.MatchString(msg) || err != nil {
//         t.Fatalf(`Hello("Gladys") = %q, %v, want match for %#q, nil`, msg, err, want)
//     }
// }

// // TestHelloEmpty calls greetings.Hello with an empty string,
// // checking for an error.
// func TestHelloEmpty(t *testing.T) {
//     msg, err := Hello("")
//     if msg != "" || err == nil {
//         t.Fatalf(`Hello("") = %q, %v, want "", error`, msg, err)
//     }
// }