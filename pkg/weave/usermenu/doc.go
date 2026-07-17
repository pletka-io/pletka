// Package usermenu builds the role-aware navigation panel that opens
// from the header trigger for logged-in users. The shape is deliberately
// flat data — no domain knowledge — so the frontend renders a generic
// dropdown without reasoning about permissions. This package is a shared infrastructure
// slice: it owns no store and is imported directly by pkg/weave/templates
// to embed the menu payload in the island page shell.
//
// Adding a new menu item is one line in Build; the schema-driven island
// picks it up. Future slots already reserved on the types: AvatarURL
// (when profile photos ship), Match (active-highlight via current
// path), Badge (notification counts).
package usermenu
