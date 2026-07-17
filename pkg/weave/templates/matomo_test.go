package templates

import (
	"strings"
	"testing"
)

func TestMatomoSnippet(t *testing.T) {
	base := "https://analytics.pletka.io"

	// disabled / incomplete -> empty
	for _, cfg := range []AnalyticsConfig{
		{},
		{Enabled: true, MatomoBaseURL: base}, // no site id
		{Enabled: true, SiteID: "1"},         // no base url
		{Enabled: false, MatomoBaseURL: base, SiteID: "1"}, // not enabled
	} {
		if s := matomoSnippet(cfg); s != "" {
			t.Errorf("expected empty snippet for %+v, got %q", cfg, s)
		}
	}

	// fully configured -> the tracker script for the right site/base
	s := string(matomoSnippet(AnalyticsConfig{Enabled: true, MatomoBaseURL: base, SiteID: "1"}))
	for _, want := range []string{"setSiteId", `"1"`, "analytics.pletka.io", "matomo.js", "setCustomUrl"} {
		if !strings.Contains(s, want) {
			t.Errorf("snippet missing %q:\n%s", want, s)
		}
	}

	// track_query_string=true -> no setCustomUrl (full URL tracked)
	s2 := string(matomoSnippet(AnalyticsConfig{Enabled: true, MatomoBaseURL: base, SiteID: "4", TrackQueryString: true}))
	if strings.Contains(s2, "setCustomUrl") {
		t.Errorf("with TrackQueryString the snippet should not strip the query:\n%s", s2)
	}
	if !strings.Contains(s2, `"4"`) {
		t.Errorf("snippet should carry site id 4:\n%s", s2)
	}
}
