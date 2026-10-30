// Package snapshotrow maps explicit lossless SQL projections to project values.
package snapshotrow

// Nullable decodes the empty sentinel used only by nullable date/numeric/time
// projections. None of those PostgreSQL types can contain an empty string.
func Nullable[T ~string](text string) *T {
	if text == "" {
		return nil
	}
	value := T(text)
	return &value
}
