// Package templates renders the minimal weave island page shell (layout,
// header, footer, and error pages) from embedded gohtml templates, folding
// in the usermenu payload and resolving i18n.LocalizedText nodes before
// render. This package is a shared infrastructure slice: it owns no store
// and is imported directly by handler-only modules and full slices alike
// whenever they need to render a page shell.
package templates
