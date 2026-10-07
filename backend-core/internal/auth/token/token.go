package token

import "crypto/sha256"

const Bytes = 32

func Hash(value string) []byte {
	sum := sha256.Sum256([]byte(value))
	return sum[:]
}
