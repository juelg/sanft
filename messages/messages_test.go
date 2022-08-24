package messages

import (
	"testing"
)

func TestInt2uint8_6_arr2int(t *testing.T) {
	var a uint64
	a = 0x1baddeadbeef
	b := Int2uint8_6_arr(a)
	c := Uint8_6_arr2int(b)

	if a != c {
		t.Fatalf("Invalid value after uint64->[6]uint8->uint64. Expected %x got %x.", a, c);
	}
}

func TestUint8_6_arr2Int2uint8_6_arr(t *testing.T) {
	a := [6]uint8{0xca, 0xfe, 0xfa, 0xce, 0x12, 0x34}
	b := Uint8_6_arr2int(&a)
	c := Int2uint8_6_arr(b)

	if a != *c {
		t.Fatalf("Invalid value after uint64->[6]uint8->uint64. Expected %x got %x.", a, c);
	}
}
