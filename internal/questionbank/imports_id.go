package questionbank

import (
	"crypto/sha1"
	"encoding/hex"
)

func importGeneratedID(prefix, s string) string {
	sum := sha1.Sum([]byte(s))
	return prefix + "-" + hex.EncodeToString(sum[:])[:12]
}
