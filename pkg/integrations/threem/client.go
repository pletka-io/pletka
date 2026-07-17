// Package threem is the FORTH 3M integration. It speaks the legacy 3M
// HTTP surface — form login + multipart mapping upload + sticky session
// cookie — and exposes it as a pkg/integrations/registry.Integration so
// the integrations hub can offer per-project "Upload to 3M" actions.
//
// The integration is built around a small Client that knows nothing
// about Pletka or the hub. RunAction wires the user-supplied config
// into a Client and dispatches an UploadMapping with the in-memory
// X3ML zip the hub's artifact provider produced.
package threem

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// Sentinel errors callers (the integration's RunAction, the hub
// service, tests) match against.
var (
	// ErrBadCredentials is returned by Login when the server's 302
	// redirect points back at the login page, the conventional
	// signal that username/password are wrong in 3M's Spring
	// Security stack.
	ErrBadCredentials = errors.New("3m: bad credentials")

	// ErrSessionExpired is returned by UploadMapping when the
	// server redirects to the login page mid-upload (the cookie
	// expired between Login and UploadMapping). Callers should
	// call Login again and retry once.
	ErrSessionExpired = errors.New("3m: session expired")

	// ErrUploadRejected is returned by UploadMapping when the
	// server returns 2xx but the response body does not contain
	// the success marker or the mapping URI pattern.
	ErrUploadRejected = errors.New("3m: upload rejected")
)

// Client is a thin HTTP wrapper around the 3M endpoints. It is
// stateful in one way only: the cookie jar that Login populates is
// read back by UploadMapping. Construct via NewClient.
type Client struct {
	BaseURL  string
	Username string
	Password string
	HTTP     *http.Client

	// LoginPath, UploadPath override the conventional 3M paths.
	// Empty means defaults: "/login" and "/upload".
	LoginPath  string
	UploadPath string
}

// NewClient returns a Client with its own cookie jar so concurrent
// callers cannot accidentally share session state. Timeout is the
// caller's responsibility; supply via Client.HTTP.Timeout if needed.
func NewClient(baseURL, username, password string, timeout time.Duration) (*Client, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, fmt.Errorf("new cookie jar: %w", err)
	}
	httpClient := &http.Client{
		Jar:     jar,
		Timeout: timeout,
		// Halt on the first 3xx so we can inspect the login Location
		// header for the "back to login" signal Spring Security uses
		// for bad credentials. The upload path uses the same
		// CheckRedirect to spot session expiry.
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	return &Client{
		BaseURL:  strings.TrimRight(baseURL, "/"),
		Username: username,
		Password: password,
		HTTP:     httpClient,
	}, nil
}

func (c *Client) loginURL() string {
	if c.LoginPath != "" {
		return c.BaseURL + c.LoginPath
	}
	return c.BaseURL + "/login"
}

func (c *Client) uploadURL() string {
	if c.UploadPath != "" {
		return c.BaseURL + c.UploadPath
	}
	return c.BaseURL + "/upload"
}

// Login posts username + password to the 3M form login endpoint and
// inspects the resulting redirect. A 302 whose Location contains the
// substring "login" is Spring Security's conventional "back to the
// login form" response and is treated as ErrBadCredentials.
func (c *Client) Login(ctx context.Context) error {
	form := url.Values{}
	form.Set("username", c.Username)
	form.Set("password", c.Password)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.loginURL(), strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("build login request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("login request: %w", err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusFound, http.StatusSeeOther, http.StatusMovedPermanently, http.StatusTemporaryRedirect, http.StatusPermanentRedirect:
		loc := resp.Header.Get("Location")
		if loc == "" {
			return fmt.Errorf("login: empty redirect Location")
		}
		if strings.Contains(strings.ToLower(loc), "login") {
			return ErrBadCredentials
		}
		return nil
	case http.StatusOK:
		// Some 3M deployments respond 200 with a body marker on
		// success rather than redirecting. Treat 200 as success and
		// rely on UploadMapping to surface auth failure if the
		// cookie wasn't actually issued.
		return nil
	case http.StatusUnauthorized, http.StatusForbidden:
		return ErrBadCredentials
	default:
		return fmt.Errorf("login: unexpected status %d", resp.StatusCode)
	}
}

// mappingURIPattern matches the upload response marker — a fully
// qualified URI pointing at the newly created mapping. The literal
// host "cidoc_mappings.com" mirrors the value 3M emits in
// production responses. Capture group 1 is the bare mapping ID
// (e.g. "Mapping646") which callers use to build editor URLs.
var mappingURIPattern = regexp.MustCompile(`https?://cidoc_mappings\.com/Mapping/(Mapping\d+)`)

// ExtractMappingID parses a mapping URI returned by UploadMapping and
// returns the bare ID (e.g. "Mapping646") or "" when no ID is present.
// Exposed as a package function so the integration's RunAction can
// build editor URLs without re-running the upload regex.
func ExtractMappingID(uri string) string {
	m := mappingURIPattern.FindStringSubmatch(uri)
	if len(m) < 2 {
		return ""
	}
	return m[1]
}

// UploadMapping POSTs a multipart form with field "file" carrying the
// zipped X3ML mapping. The server responds 200 with a body that
// contains the new mapping URI; UploadMapping extracts and returns it.
//
// Cookies from Login persist via the shared jar. If the server
// redirects mid-upload, the request is interpreted as session expiry.
func (c *Client) UploadMapping(ctx context.Context, filename string, body io.Reader) (string, error) {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	if err := writeZipPart(mw, "file", filename, body); err != nil {
		return "", err
	}
	if err := mw.Close(); err != nil {
		return "", fmt.Errorf("close multipart writer: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.uploadURL(), &buf)
	if err != nil {
		return "", fmt.Errorf("build upload request: %w", err)
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", fmt.Errorf("upload request: %w", err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		// Read the body and look for the mapping URI marker.
		bodyBytes, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			return "", fmt.Errorf("read upload response: %w", readErr)
		}
		m := mappingURIPattern.FindStringSubmatch(string(bodyBytes))
		if len(m) < 1 || m[0] == "" {
			return "", ErrUploadRejected
		}
		return m[0], nil
	case http.StatusFound, http.StatusSeeOther:
		loc := resp.Header.Get("Location")
		if strings.Contains(strings.ToLower(loc), "login") {
			return "", ErrSessionExpired
		}
		return "", fmt.Errorf("upload: unexpected redirect to %q", loc)
	case http.StatusUnauthorized:
		return "", ErrSessionExpired
	default:
		return "", fmt.Errorf("upload: unexpected status %d", resp.StatusCode)
	}
}

// writeZipPart writes a single form file part with Content-Type
// application/zip — 3M requires the part header (not the request
// header) to advertise the zip type or it returns a generic 400.
func writeZipPart(mw *multipart.Writer, field, filename string, body io.Reader) error {
	header := make(map[string][]string)
	header["Content-Disposition"] = []string{
		fmt.Sprintf(`form-data; name=%q; filename=%q`, field, filename),
	}
	header["Content-Type"] = []string{"application/zip"}
	part, err := mw.CreatePart(header)
	if err != nil {
		return fmt.Errorf("create multipart part: %w", err)
	}
	if _, err := io.Copy(part, body); err != nil {
		return fmt.Errorf("copy multipart body: %w", err)
	}
	return nil
}
