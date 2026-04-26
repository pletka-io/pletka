// Package relations holds link-emission helpers used to attach hypermedia
// "rel" links to schema responses. Authorization decisions are taken here:
// a link appears in the response only if the current user is permitted to
// follow it.
//
// One file per link relation. Each relation has a Markdown spec under
// schema/relations/ documenting its semantics, HTTP method, and request /
// response shape.
package relations
