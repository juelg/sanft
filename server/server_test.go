package server

import (
	"net"
	"testing"
	"github.com/stretchr/testify/assert"
)



func TestToken(t *testing.T){
	s, err := Init("127.0.0.1", 10000, "/", 1024, 1, 0, 0)
    if err != nil{
        t.Fatalf(`Error creating server: %v`, err)
    }

	addr := net.UDPAddr{
		Port: 1000,
		IP:   net.ParseIP("127.100.0.1"),
	}
	token := s.createToken(&addr)
	assert.True(t,s.checkToken(&addr, &token), "Token miss match")

	var false_token [32]uint8
	assert.False(t,s.checkToken(&addr, &false_token), "Token match but should miss match")

	addr2 := net.UDPAddr{
		Port: 1001,
		IP:   net.ParseIP("127.100.0.1"),
	}
	// slighly changed address should also result into chack=false
	assert.False(t,s.checkToken(&addr2, &token), "Token match but should miss match")
}