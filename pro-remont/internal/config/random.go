package config

import (
	"crypto/rand"
	"io"
)

func cryptoRead(b []byte) (int, error) {
	return io.ReadFull(rand.Reader, b)
}
