package vocabservice

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
)

// maxErrorBodyBytes caps how much of a non-2xx body get will read before
// giving up on decoding it as the v2 error envelope. An error body is not
// guaranteed to be small, or JSON at all — a proxy or a captive portal can
// hand back an HTML page — so this bounds the read regardless of what the
// service itself would ever send.
const maxErrorBodyBytes = 64 * 1024

// errCodeUnknownVocabulary is the v2 token naming a vocabulary the service
// does not mount. Named as a constant because it is compared against in more
// than one place (permanentTokens here, and the tests that assert against
// it), where a repeated string literal would drift.
const errCodeUnknownVocabulary = "unknown_vocabulary"

// permanentTokens are the v2 error tokens that mean a row's configuration
// will not work no matter how many times it is retried: the vocabulary
// named in the row is not mounted, or the row's configured lang is not a
// usable tag at all (bad_lang no longer means "no index for this language" —
// the service now falls back to one it does have; it is left only for a
// malformed tag, which is as permanent a fault as naming an unmounted
// vocabulary). Every other token, including vocab_unavailable, is
// transient: the mount exists but could not be read, which is the
// service's problem and may well fix itself.
var permanentTokens = map[string]bool{
	errCodeUnknownVocabulary: true,
	"bad_lang":               true,
}

// ErrMisconfigured marks a failure that will not fix itself: the row names a
// vocabulary the service does not serve, or a language tag the service
// rejects as malformed. It travels alongside ErrDegraded rather than instead
// of it — autocomplete still degrades to an empty list, but the log line can
// say the row is wrong rather than implying the service is down.
var ErrMisconfigured = errors.New("vocabulary service: configuration does not match the service")

// ServiceError is a v2 error body: {"error": "<token>", "message": "<sentence>"}
// plus whatever extra keys that token carries — today just Vocab, which
// unknown_vocabulary sends. Switch on Token; Message is prose for a log and
// may be reworded between releases without notice.
type ServiceError struct {
	Status  int
	Token   string `json:"error"`
	Message string `json:"message"`
	Vocab   string `json:"vocab,omitempty"`
}

func (e *ServiceError) Error() string {
	if e.Vocab != "" {
		return fmt.Sprintf("vocabulary service: %s (status %d, vocab %q): %s", e.Token, e.Status, e.Vocab, e.Message)
	}
	return fmt.Sprintf("vocabulary service: %s (status %d): %s", e.Token, e.Status, e.Message)
}

// Unwrap lets errors.Is(err, ErrMisconfigured) find a permanent fault without
// every caller needing to switch on Token itself.
func (e *ServiceError) Unwrap() error {
	if permanentTokens[e.Token] {
		return ErrMisconfigured
	}
	return nil
}

// decodeServiceError reads a non-2xx response body defensively — it is not
// guaranteed to be JSON at all (a proxy or a captive portal can hand back
// HTML), and it is not guaranteed to be small — and turns it into a
// *ServiceError when a token is present. A body with no recognizable token
// still produces an error naming the status: never a silent success.
func decodeServiceError(endpoint string, resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBodyBytes))
	svcErr := &ServiceError{Status: resp.StatusCode}
	if err := json.Unmarshal(body, svcErr); err != nil || svcErr.Token == "" {
		return fmt.Errorf("call %s: status %d", endpoint, resp.StatusCode)
	}
	return fmt.Errorf("call %s: %w", endpoint, svcErr)
}
