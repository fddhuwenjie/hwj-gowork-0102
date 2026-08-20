package platform

import (
	"crypto/rand"
	"encoding/hex"
)

type IDGenerator interface {
	New() string
}

type RandomIDGenerator struct{}

func (RandomIDGenerator) New() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}
