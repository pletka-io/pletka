package generators

import (
	"strings"
	"unicode"
)

// Slug normalizes an identifier for use in relative resource paths.
func Slug(parts ...string) string {
	for _, part := range parts {
		if s := slugOne(part); s != "" {
			return s
		}
	}
	return "resource"
}

func slugOne(in string) string {
	in = strings.TrimSpace(in)
	if in == "" {
		return ""
	}

	var b strings.Builder
	lastDash := false
	for _, r := range strings.ToLower(in) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		case unicode.IsLetter(r), unicode.IsDigit(r):
			b.WriteRune(r)
			lastDash = false
		default:
			if !lastDash && b.Len() > 0 {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}

type slugAllocator struct {
	seen map[string]int
}

func newSlugAllocator() *slugAllocator {
	return &slugAllocator{seen: make(map[string]int)}
}

func (a *slugAllocator) Next(parts ...string) string {
	base := Slug(parts...)
	n := a.seen[base]
	a.seen[base] = n + 1
	if n == 0 {
		return base
	}
	return base + "-" + strconvInt(n+1)
}

func strconvInt(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
