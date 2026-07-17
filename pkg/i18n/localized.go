package i18n

import (
	"encoding/json"
	"reflect"
	"strings"

	"github.com/pletka-io/pletka/pkg/domain"
)

// LocalizedText is platform copy with a stable translation key plus an
// English fallback. The fallback is baked in at construction so the
// schema always serialises a usable label even when the i18n bundle is
// degraded (missing key, storage error, transient unavailability).
//
// At schema-serialisation time, callers run Manager.Resolve over the
// schema tree before encoding. Resolve looks up Key in every enabled
// language and merges the values into the embedded Translations map
// IN PLACE. Custom MarshalJSON then emits just the Translations map —
// the Key field is metadata for the walker, not for the wire.
//
// Construct with the L or LF helpers:
//
//	Label: i18n.L("forms.cancel", "Cancel")
//	Label: i18n.LF("release.viewing", "Viewing release {version}",
//	    map[string]string{"version": activeVersion})
//
// LF binds runtime arguments to {name} placeholders in the fallback and
// in every bundle translation. Resolve substitutes them in place across
// every language string after the bundle merge, so the wire format
// remains a translations map — no special handling on the frontend.
//
// Schema fields that today hold domain.Translations migrate to
// LocalizedText one file at a time. The walker accepts both types
// (LocalizedText resolves; raw Translations passes through), so
// migration is mechanical with no flag-day requirement.
type LocalizedText struct {
	Key  string
	Args map[string]string
	domain.Translations
}

// MarshalJSON emits the embedded Translations map directly so the wire
// format stays identical to today (frontend `tr(t, lang)` works
// unchanged). The Key and Args fields never reach the wire.
//
// If Resolve hasn't been called, only the fallback ships — degraded but
// readable. Args still get substituted defensively so a missed Resolve
// call doesn't leak {placeholder} tokens to end users; in normal flow
// Resolve has already done this and the Args map is nil here.
func (lt LocalizedText) MarshalJSON() ([]byte, error) {
	if lt.Translations == nil {
		return []byte("null"), nil
	}
	if len(lt.Args) > 0 {
		out := make(domain.Translations, len(lt.Translations))
		for code, value := range lt.Translations {
			out[code] = applyArgs(value, lt.Args)
		}
		return json.Marshal(map[string]string(out))
	}
	return json.Marshal(map[string]string(lt.Translations))
}

// L is the conventional constructor. Inline both pieces, type-safe.
func L(key, en string) LocalizedText {
	return LocalizedText{
		Key:          key,
		Translations: domain.Translations{"en": en},
	}
}

// LF builds a parameterised LocalizedText. The fallback and every bundle
// translation may use {name} placeholders matching keys in args; Resolve
// substitutes the runtime values into each language string after bundle
// lookup. A nil or empty args map degrades cleanly to plain L behaviour
// (placeholders pass through unchanged so missing args are visible
// during development).
func LF(key, en string, args map[string]string) LocalizedText {
	return LocalizedText{
		Key:          key,
		Args:         args,
		Translations: domain.Translations{"en": en},
	}
}

// applyArgs substitutes {name} placeholders in s with values from args.
// Unknown placeholders pass through untouched so callers see the gap.
// Returns s verbatim when args is empty (zero-allocation hot path).
func applyArgs(s string, args map[string]string) string {
	if len(args) == 0 || !strings.Contains(s, "{") {
		return s
	}
	for name, value := range args {
		s = strings.ReplaceAll(s, "{"+name+"}", value)
	}
	return s
}

// localizedTextType caches the reflect.Type for the walker — saves a
// reflect.TypeOf call per LocalizedText hit.
var localizedTextType = reflect.TypeOf(LocalizedText{})

// Resolve walks v recursively and enriches every LocalizedText found
// inside structs, slices, maps, or pointer targets by looking up its
// Key in the bundle and merging the result into its Translations map.
// Mutates in place — pass a pointer to the schema (or a struct value
// containing reference types). After Resolve, the schema can be JSON
// encoded normally; LocalizedText nodes serialise as their Translations
// map.
//
// Pass-through: raw domain.Translations values are not modified — only
// LocalizedText nodes are touched. This lets schemas migrate file by
// file without coordinated flag-days.
//
// Lookup failures (missing key, storage error) keep the embedded
// fallback and emit a slog.Warn so the gap is observable.
func (m *manager) Resolve(v any, lang string) {
	if v == nil {
		return
	}
	rv := reflect.ValueOf(v)
	m.walk(rv, lang)
}

// walk does the recursive descent. Three addressability quirks shape
// the structure:
//
//  1. Interface values: rv.Elem() returns a *copy*, never addressable.
//     For LocalizedText buried in an `any`-typed field, we read the
//     concrete value out, enrich a stack copy, and Set the original
//     interface field to the enriched copy. This is the common case
//     for schema fields like `FieldDef.Label any`.
//  2. Pointer values: rv.Elem() IS addressable when the pointer was
//     non-nil. Recurse normally.
//  3. Map values: Go map values are never addressable. Clone, walk
//     the clone, SetMapIndex back.
//
// Direct LocalizedText struct fields off a pointer-rooted schema are
// addressable through the chain and get mutated in place — fastest
// path, no copy.
func (m *manager) walk(rv reflect.Value, lang string) {
	if !rv.IsValid() {
		return
	}

	switch rv.Kind() {
	case reflect.Interface:
		if rv.IsNil() {
			return
		}
		inner := rv.Elem()
		if inner.Type() == localizedTextType {
			// Pull the concrete LocalizedText out, enrich a copy on the
			// stack, write the enriched value back into the interface
			// field. Original field MUST be settable (i.e. came from
			// a pointer-rooted struct field or slice index).
			lt := inner.Interface().(LocalizedText)
			m.enrichLocalized(&lt, lang)
			if rv.CanSet() {
				rv.Set(reflect.ValueOf(lt))
			}
			return
		}
		// Other concrete types in interface fields — walk an
		// addressable copy and write it back.
		copy := reflect.New(inner.Type()).Elem()
		copy.Set(inner)
		m.walk(copy, lang)
		if rv.CanSet() {
			rv.Set(copy)
		}
		return

	case reflect.Pointer:
		if rv.IsNil() {
			return
		}
		m.walk(rv.Elem(), lang)
		return
	}

	// LocalizedText hit (direct struct field, slice index, etc.).
	if rv.Type() == localizedTextType {
		if !rv.CanAddr() {
			// Caller passed a struct by value somewhere upstream. Can't
			// mutate; skip silently. Pointer-rooted schemas avoid this.
			return
		}
		lt := rv.Addr().Interface().(*LocalizedText)
		m.enrichLocalized(lt, lang)
		return
	}

	switch rv.Kind() {
	case reflect.Struct:
		for i := 0; i < rv.NumField(); i++ {
			f := rv.Field(i)
			// Skip unexported fields — reflect can read but not set them.
			if !f.CanSet() && !f.CanAddr() {
				continue
			}
			m.walk(f, lang)
		}
	case reflect.Slice, reflect.Array:
		for i := 0; i < rv.Len(); i++ {
			m.walk(rv.Index(i), lang)
		}
	case reflect.Map:
		// Map values aren't addressable. Clone each value into an
		// addressable holder, walk it, set it back.
		for _, key := range rv.MapKeys() {
			mv := rv.MapIndex(key)
			holder := reflect.New(mv.Type()).Elem()
			holder.Set(mv)
			m.walk(holder, lang)
			rv.SetMapIndex(key, holder)
		}
	}
}

// enrichLocalized fills lt.Translations with bundle values for every
// enabled language. The English fallback baked in by L stays as a
// floor — bundle values overwrite when present, missing values keep
// whatever was there. A single missing-key Warn fires when the
// requested language can't be resolved at all (key absent in every
// language); per-language misses don't spam.
//
// Final pass interpolates lt.Args into every language string so {name}
// placeholders in fallback or bundle entries resolve to runtime values
// before serialisation. Args is then nil'd out to avoid leaking
// metadata through any future generic encoding path.
func (m *manager) enrichLocalized(lt *LocalizedText, lang string) {
	if lt.Key != "" {
		if lt.Translations == nil {
			lt.Translations = domain.Translations{}
		}

		for _, l := range m.Languages() {
			if !l.Enabled {
				continue
			}
			trans, err := m.storage.Get(lt.Key, l.Code)
			if err != nil || trans == nil || trans.Value == "" {
				continue
			}
			lt.Translations[l.Code] = trans.Value
		}

		if _, ok := lt.Translations[lang]; !ok {
			// Last-ditch fallback to fallbackLang via the resolver chain
			// (which logs cache hits and miscellaneous metadata via the
			// existing Get method). If even that misses, log once.
			if trans, err := m.Get(lt.Key, lang); err == nil && trans != nil && trans.Value != "" {
				lt.Translations[lang] = trans.Value
			} else {
				m.logger.Warn("i18n key missing for requested language",
					"key", lt.Key,
					"lang", lang,
				)
			}
		}
	}

	if len(lt.Args) > 0 && lt.Translations != nil {
		for code, value := range lt.Translations {
			lt.Translations[code] = applyArgs(value, lt.Args)
		}
		lt.Args = nil
	}
}
