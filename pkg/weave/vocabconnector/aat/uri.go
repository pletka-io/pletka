package aat

import (
	"net/url"
	"strings"
)

func NormalizeURI(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return value
	}
	if strings.EqualFold(parsed.Host, "vocab.getty.edu") && strings.HasPrefix(parsed.Path, "/aat/") {
		parsed.Scheme = "https"
	}
	if strings.EqualFold(parsed.Host, "vocab.getty.edu") && strings.HasPrefix(parsed.Path, "/page/aat/") {
		parsed.Scheme = "https"
		parsed.Path = "/aat/" + strings.TrimPrefix(parsed.Path, "/page/aat/")
	}
	return parsed.String()
}

func sparqlURI(value string) string {
	value = NormalizeURI(value)
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return value
	}
	// Getty's SPARQL data uses http IRIs even though public links are often
	// accessed over https. Keep storage normalized to https, but query Getty
	// with the canonical RDF IRI so parent filters and BIND lookups match.
	if strings.EqualFold(parsed.Host, "vocab.getty.edu") && strings.HasPrefix(parsed.Path, "/aat/") {
		parsed.Scheme = "http"
	}
	return parsed.String()
}
