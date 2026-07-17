package errortracking

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
)

// sensitiveHeaders are dropped wholesale from the headers snapshot.
var sensitiveHeaders = map[string]struct{}{
	"authorization": {},
	"cookie":        {},
	"set-cookie":    {},
	"x-csrf-token":  {},
}

// sensitiveBodyFields are JSON keys whose values are masked (case-
// insensitive, at any nesting depth).
var sensitiveBodyFields = map[string]struct{}{
	"password":     {},
	"passwd":       {},
	"secret":       {},
	"token":        {},
	"api_key":      {},
	"apikey":       {},
	"access_token": {},
	"refresh_token": {},
}

// maxPayloadBytes caps the serialised request body stored per row.
const maxPayloadBytes = 8 * 1024

// snapshotHeaders returns a header map with sensitive entries dropped.
// Headers are flattened to first-value-only since storing arrays
// adds noise without aiding debugging.
func snapshotHeaders(h http.Header) map[string]string {
	out := make(map[string]string, len(h))
	for k, v := range h {
		if _, sensitive := sensitiveHeaders[strings.ToLower(k)]; sensitive {
			continue
		}
		if len(v) == 0 {
			continue
		}
		out[k] = v[0]
	}
	return out
}

// redactJSONPayload returns a redacted copy of body capped at
// maxPayloadBytes. If body isn't valid JSON it returns the raw bytes
// truncated and a marker indicating the content was not parsed.
func redactJSONPayload(body []byte) json.RawMessage {
	if len(body) == 0 {
		return nil
	}
	if len(body) > maxPayloadBytes {
		body = body[:maxPayloadBytes]
	}
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		raw, _ := json.Marshal(map[string]any{
			"_truncated_raw": string(body),
		})
		return raw
	}
	redactInPlace(v)
	out, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	if len(out) > maxPayloadBytes {
		out = out[:maxPayloadBytes]
	}
	return out
}

// redactInPlace walks a decoded JSON tree and masks values whose key
// matches sensitiveBodyFields.
func redactInPlace(v any) {
	switch t := v.(type) {
	case map[string]any:
		for k, child := range t {
			if _, sensitive := sensitiveBodyFields[strings.ToLower(k)]; sensitive {
				t[k] = "[redacted]"
				continue
			}
			redactInPlace(child)
		}
	case []any:
		for _, child := range t {
			redactInPlace(child)
		}
	}
}
