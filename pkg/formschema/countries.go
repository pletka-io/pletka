package formschema

import (
	"sort"

	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/i18n"
	"golang.org/x/text/language"
	"golang.org/x/text/language/display"
)

// countryCodes is the ISO 3166-1 alpha-2 list surfaced as the country
// dropdown across the platform. Names are resolved at runtime via
// CLDR (golang.org/x/text/language/display) so en/nl/el/bg pick up
// localized country labels without a hand-maintained translation
// table. Keep the list sorted by code.
var countryCodes = []string{
	"AF", "AL", "DZ", "AD", "AO", "AG", "AR", "AM", "AU", "AT", "AZ", "BS",
	"BH", "BD", "BB", "BY", "BE", "BZ", "BJ", "BT", "BO", "BA", "BW", "BR",
	"BN", "BG", "BF", "BI", "CV", "KH", "CM", "CA", "CF", "TD", "CL", "CN",
	"CO", "KM", "CG", "CD", "CR", "CI", "HR", "CU", "CY", "CZ", "DK", "DJ",
	"DM", "DO", "EC", "EG", "SV", "GQ", "ER", "EE", "SZ", "ET", "FJ", "FI",
	"FR", "GA", "GM", "GE", "DE", "GH", "GR", "GD", "GT", "GN", "GW", "GY",
	"HT", "HN", "HU", "IS", "IN", "ID", "IR", "IQ", "IE", "IL", "IT", "JM",
	"JP", "JO", "KZ", "KE", "KI", "KP", "KR", "KW", "KG", "LA", "LV", "LB",
	"LS", "LR", "LY", "LI", "LT", "LU", "MG", "MW", "MY", "MV", "ML", "MT",
	"MH", "MR", "MU", "MX", "FM", "MD", "MC", "MN", "ME", "MA", "MZ", "MM",
	"NA", "NR", "NP", "NL", "NZ", "NI", "NE", "NG", "MK", "NO", "OM", "PK",
	"PW", "PA", "PG", "PY", "PE", "PH", "PL", "PT", "QA", "RO", "RU", "RW",
	"KN", "LC", "VC", "WS", "SM", "ST", "SA", "SN", "RS", "SC", "SL", "SG",
	"SK", "SI", "SB", "SO", "ZA", "SS", "ES", "LK", "SD", "SR", "SE", "CH",
	"SY", "TW", "TJ", "TZ", "TH", "TL", "TG", "TO", "TT", "TN", "TR", "TM",
	"TV", "UG", "UA", "AE", "GB", "US", "UY", "UZ", "VU", "VA", "VE", "VN",
	"YE", "ZM", "ZW",
}

// supportedCountryLangs are the bundle languages Pletka ships today.
// CountryOptions pre-fills every option's Translations map for these
// languages straight from CLDR so the schema serializes localized
// names without needing matching bundle entries.
var supportedCountryLangs = []string{"en", "nl", "el", "bg"}

// CountryOptions returns the ISO 3166-1 alpha-2 country list as
// SelectOptions, sorted by the localized name for the requested
// language. Each label is a LocalizedText whose Translations map is
// pre-populated from CLDR for every supported language — i18n.L's
// Resolve walker leaves it alone (empty Key skips bundle lookup), and
// MarshalJSON emits the populated map. Bundle overrides are not
// supported here on purpose: country names come from CLDR, full stop.
func CountryOptions(lang string) []SelectOption {
	primary := language.English
	if lang != "" {
		if t, err := language.Parse(lang); err == nil {
			primary = t
		}
	}
	primaryNamer := display.Regions(primary)

	namers := make(map[string]display.Namer, len(supportedCountryLangs))
	for _, code := range supportedCountryLangs {
		t, err := language.Parse(code)
		if err != nil {
			continue
		}
		namers[code] = display.Regions(t)
	}

	out := make([]SelectOption, 0, len(countryCodes))
	for _, code := range countryCodes {
		region, err := language.ParseRegion(code)
		if err != nil {
			continue
		}
		trans := domain.Translations{}
		for langCode, namer := range namers {
			name := namer.Name(region)
			if name != "" {
				trans[langCode] = name
			}
		}
		out = append(out, SelectOption{
			Value: code,
			Label: i18n.LocalizedText{Translations: trans},
		})
	}

	sort.SliceStable(out, func(i, j int) bool {
		ri, _ := language.ParseRegion(out[i].Value)
		rj, _ := language.ParseRegion(out[j].Value)
		return primaryNamer.Name(ri) < primaryNamer.Name(rj)
	})
	return out
}

// CountryLabels returns code → localised name for every CLDR country
// code, in the requested language (falling back to English on parse
// errors). Used by filter dropdowns that lazy-fetch options and need
// pre-resolved labels per code without dragging the full SelectOption
// map through.
func CountryLabels(lang string) map[string]string {
	primary := language.English
	if lang != "" {
		if t, err := language.Parse(lang); err == nil {
			primary = t
		}
	}
	namer := display.Regions(primary)
	out := make(map[string]string, len(countryCodes))
	for _, code := range countryCodes {
		region, err := language.ParseRegion(code)
		if err != nil {
			continue
		}
		out[code] = namer.Name(region)
	}
	return out
}
