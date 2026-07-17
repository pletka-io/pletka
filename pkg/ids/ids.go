package ids

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"time"

	"github.com/oklog/ulid/v2"
)

// GenerateULID generates a new ULID as a string
func GenerateULID() string {
	entropy := ulid.Monotonic(rand.Reader, 0)
	return ulid.MustNew(ulid.Timestamp(time.Now()), entropy).String()
}

// GenerateID generates a deterministic ULID if id is not empty, otherwise random
func GenerateID(id string) string {
	// Try to extract Airtable ID from raw data
	if id != "" {
		hash := sha256.Sum256([]byte(id))
		entropy := bytes.NewReader(hash[:])

		// Use a fixed timestamp (e.g., Unix epoch)
		fixedTime := time.Unix(0, 0)
		return ulid.MustNew(ulid.Timestamp(fixedTime), entropy).String()
	}

	// Fallback to random ULID for non-Airtable entities
	return GenerateULID()
}
