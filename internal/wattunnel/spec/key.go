package spec

import (
	"crypto/rand"
	"math/big"
)

const keyAlphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

// NewKey returns a fresh 40 character tunnel key.
func NewKey() (string, error) {
	b := make([]byte, 40)
	max := big.NewInt(int64(len(keyAlphabet)))
	for i := range b {
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", err
		}
		b[i] = keyAlphabet[n.Int64()]
	}
	return string(b), nil
}
