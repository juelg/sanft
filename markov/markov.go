package markov

import (
	"math/rand"
	"net"
	"time"
)

type MarkovConn struct {
	UDPConn *net.UDPConn
	P       float64
	Q       float64

	lastDropped bool
}

// Implement the interface for net.PacketConn
func (mc *MarkovConn) ReadFrom(p []byte) (n int, addr net.Addr, err error) {
	return mc.UDPConn.ReadFrom(p)
}

func (mc *MarkovConn) WriteTo(p []byte, addr net.Addr) (n int, err error) {
	if mc.lastDropped {
		if rand.Float64() < mc.Q {
			// Drop
			mc.lastDropped = true
			return len(p), nil
		} else {
			mc.lastDropped = false
			return mc.UDPConn.WriteTo(p, addr)
		}
	} else {
		if rand.Float64() < mc.P {
			// Drop
			mc.lastDropped = true
			return len(p), nil
		} else {
			mc.lastDropped = false
			return mc.UDPConn.WriteTo(p, addr)
		}
	}
}

// Implement the interface for net.Conn
func (mc *MarkovConn) Read(p []byte) (n int, err error) {
	return mc.UDPConn.Read(p)
}

func (mc *MarkovConn) Write(p []byte) (n int, err error) {
	if mc.lastDropped {
		if rand.Float64() < mc.Q {
			// Drop
			mc.lastDropped = true
			return len(p), nil
		} else {
			mc.lastDropped = false
			return mc.UDPConn.Write(p)
		}
	} else {
		if rand.Float64() < mc.P {
			// Drop
			mc.lastDropped = true
			return len(p), nil
		} else {
			mc.lastDropped = false
			return mc.UDPConn.Write(p)
		}
	}
}

func (mc *MarkovConn) RemoteAddr() net.Addr {
	return mc.UDPConn.RemoteAddr()
}

// Implement the interface for both net.Conn and net.PacketConn
func (mc *MarkovConn) Close() error {
	return mc.UDPConn.Close()
}

func (mc *MarkovConn) LocalAddr() net.Addr {
	return mc.UDPConn.LocalAddr()
}

func (mc *MarkovConn) SetDeadline(t time.Time) error {
	return mc.UDPConn.SetDeadline(t)
}

func (mc *MarkovConn) SetReadDeadline(t time.Time) error {
	return mc.UDPConn.SetReadDeadline(t)
}

func (mc *MarkovConn) SetWriteDeadline(t time.Time) error {
	return mc.UDPConn.SetWriteDeadline(t)
}
