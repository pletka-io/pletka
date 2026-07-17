package threem_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/pletka-io/pletka/pkg/integrations/threem"
)

func newClient(t *testing.T, srv *httptest.Server) *threem.Client {
	t.Helper()
	c, err := threem.NewClient(srv.URL, "alice", "pw", 5*time.Second)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return c
}

func TestLogin_SuccessRedirect(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", "/dashboard")
		http.SetCookie(w, &http.Cookie{Name: "JSESSIONID", Value: "abc123", Path: "/"})
		w.WriteHeader(http.StatusFound)
	}))
	t.Cleanup(srv.Close)
	if err := newClient(t, srv).Login(context.Background()); err != nil {
		t.Fatalf("Login: %v", err)
	}
}

func TestLogin_BadCredsViaLoginRedirect(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", "/login?error")
		w.WriteHeader(http.StatusFound)
	}))
	t.Cleanup(srv.Close)
	err := newClient(t, srv).Login(context.Background())
	if !errors.Is(err, threem.ErrBadCredentials) {
		t.Fatalf("want ErrBadCredentials, got %v", err)
	}
}

func TestLogin_BadCredsViaUnauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	t.Cleanup(srv.Close)
	err := newClient(t, srv).Login(context.Background())
	if !errors.Is(err, threem.ErrBadCredentials) {
		t.Fatalf("want ErrBadCredentials, got %v", err)
	}
}

func TestUploadMapping_Success(t *testing.T) {
	var seenContentType, seenBody string
	var sawCookie bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/login":
			http.SetCookie(w, &http.Cookie{Name: "JSESSIONID", Value: "sess1", Path: "/"})
			w.Header().Set("Location", "/dashboard")
			w.WriteHeader(http.StatusFound)
		case "/upload":
			for _, c := range r.Cookies() {
				if c.Name == "JSESSIONID" {
					sawCookie = true
				}
			}
			seenContentType = r.Header.Get("Content-Type")
			b, _ := io.ReadAll(r.Body)
			seenBody = string(b)
			_, _ = w.Write([]byte("Mapping accepted at http://cidoc_mappings.com/Mapping/Mapping42 done\n"))
		default:
			t.Errorf("unexpected path %q", r.URL.Path)
		}
	}))
	t.Cleanup(srv.Close)

	c := newClient(t, srv)
	if err := c.Login(context.Background()); err != nil {
		t.Fatalf("Login: %v", err)
	}
	uri, err := c.UploadMapping(context.Background(), "LAM.1.b.x3ml.zip", strings.NewReader("ZIP_BYTES"))
	if err != nil {
		t.Fatalf("UploadMapping: %v", err)
	}
	if uri != "http://cidoc_mappings.com/Mapping/Mapping42" {
		t.Fatalf("uri: %q", uri)
	}
	if !sawCookie {
		t.Fatalf("session cookie not sent on upload")
	}
	if !strings.HasPrefix(seenContentType, "multipart/form-data; boundary=") {
		t.Fatalf("upload Content-Type: %q", seenContentType)
	}
	if !strings.Contains(seenBody, "ZIP_BYTES") || !strings.Contains(seenBody, `name="file"`) {
		t.Fatalf("upload body missing file part: %q", seenBody)
	}
	if !strings.Contains(seenBody, "Content-Type: application/zip") {
		t.Fatalf("upload body missing zip part header: %q", seenBody)
	}
}

func TestUploadMapping_RejectedWhenNoMarker(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/login":
			w.Header().Set("Location", "/dashboard")
			w.WriteHeader(http.StatusFound)
		case "/upload":
			_, _ = w.Write([]byte("some other 200 body"))
		}
	}))
	t.Cleanup(srv.Close)

	c := newClient(t, srv)
	_ = c.Login(context.Background())
	_, err := c.UploadMapping(context.Background(), "x.zip", bytes.NewReader([]byte("x")))
	if !errors.Is(err, threem.ErrUploadRejected) {
		t.Fatalf("want ErrUploadRejected, got %v", err)
	}
}

func TestUploadMapping_SessionExpiredOnLoginRedirect(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/login":
			w.Header().Set("Location", "/dashboard")
			w.WriteHeader(http.StatusFound)
		case "/upload":
			w.Header().Set("Location", "/login?session_expired")
			w.WriteHeader(http.StatusFound)
		}
	}))
	t.Cleanup(srv.Close)
	c := newClient(t, srv)
	_ = c.Login(context.Background())
	_, err := c.UploadMapping(context.Background(), "x.zip", bytes.NewReader([]byte("x")))
	if !errors.Is(err, threem.ErrSessionExpired) {
		t.Fatalf("want ErrSessionExpired, got %v", err)
	}
}

func TestUploadMapping_SessionExpiredOn401(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/login":
			w.Header().Set("Location", "/dashboard")
			w.WriteHeader(http.StatusFound)
		case "/upload":
			w.WriteHeader(http.StatusUnauthorized)
		}
	}))
	t.Cleanup(srv.Close)
	c := newClient(t, srv)
	_ = c.Login(context.Background())
	_, err := c.UploadMapping(context.Background(), "x.zip", bytes.NewReader([]byte("x")))
	if !errors.Is(err, threem.ErrSessionExpired) {
		t.Fatalf("want ErrSessionExpired, got %v", err)
	}
}

func TestNewClient_TrimsTrailingSlash(t *testing.T) {
	c, err := threem.NewClient("http://example.test/", "u", "p", 0)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if c.BaseURL != "http://example.test" {
		t.Fatalf("BaseURL: %q", c.BaseURL)
	}
}
