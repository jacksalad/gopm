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
	rand.Read(b)
	return fmt.Sprintf("c_%d_%s", time.Now().Unix(), hex.EncodeToString(b))
}
