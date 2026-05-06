package memory

import (
	"crypto/rand"
	"encoding/hex"
	"strings"
)

func newID(prefix string) (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	if prefix == "" {
		return hex.EncodeToString(b[:]), nil
	}
	return strings.TrimSuffix(prefix, "_") + "_" + hex.EncodeToString(b[:]), nil
}
