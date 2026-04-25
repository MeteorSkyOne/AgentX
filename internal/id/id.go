package id

import (
	"crypto/rand"
	"encoding/base32"
	"strings"
)

func New(prefix string) string {
	return prefix + "_" + randomBase32(16)
}

func NewToken() string {
	return randomBase32(32)
}

func randomBase32(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	enc := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b)
	return strings.ToLower(enc)
}
