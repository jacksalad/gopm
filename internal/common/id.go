package common

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
)

// GenerateConnID generates a unique connection ID in the format c_<timestamp>_<random_hex>.
func GenerateConnID() string {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		// Fallback: use timestamp-only ID if crypto/rand fails
		return fmt.Sprintf("c_%d_err", time.Now().UnixNano())
	}
	return fmt.Sprintf("c_%d_%s", time.Now().Unix(), hex.EncodeToString(b))
}
