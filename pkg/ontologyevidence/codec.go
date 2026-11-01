package ontologyevidence

import (
	"crypto/sha256"
	"encoding/json"
	jsonv2 "encoding/json/v2"
	"fmt"
	"strings"
)

// decodeJSON streams a new value in the existing JSON dialect. Legacy error
// reporting prevalidates and buffers the entire input, defeating streaming.
// Disable only that reporting on the success path; on error, use the original
// decoder over the same bytes to preserve its exact acceptance/error behavior.
// UnmarshalRead checks trailing input too. No partial decode is reused.
func decodeJSON[T any](data string) (T, error) {
	var value T
	if err := jsonv2.UnmarshalRead(strings.NewReader(data), &value, json.DefaultOptionsV1(), json.ReportErrorsWithLegacySemantics(false)); err != nil {
		var original T
		err = json.Unmarshal([]byte(data), &original)
		return original, err
	}
	return value, nil
}

// localDigest hashes the same encoding/json representation used for bundle
// consistency, excluding only the embedded sources. MarshalWrite has no trailing
// newline and writes directly to the hash instead of retaining an encoded copy.
// Original artifact bytes and their separate exact-byte digest remain untouched.
func localDigest(d Document) (string, error) {
	d.Sources = nil
	h := sha256.New()
	if err := jsonv2.MarshalWrite(h, d, json.DefaultOptionsV1()); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}
