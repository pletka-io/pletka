// Package csvexport renders per-project CSV downloads of the weave
// data model: fields, models, collections, categories, the three
// override flavours, and linked ontologies. Plus a "Download All"
// zip and a small HTML index page listing the export types.
//
// The slice was previously pkg/service/csvexport; relocating it here
// drops the GORM *repository.Repositories dependency and the legacy
// shared template render path. Everything reads through
// domain.WeaveStore now, and the index page is rendered from a
// self-contained gohtml template embedded in the package.
//
// Routes:
//
//	GET /projects/{projectID}/downloads             — HTML index of export types
//	GET /projects/{projectID}/downloads/all.zip     — every CSV bundled
//	GET /projects/{projectID}/downloads/{type}.csv  — one CSV per type
//
// The CSV export was used during the airtable → weave migration to
// validate the new tables row-for-row against the legacy export.
// Keeping it alive here means later migrations get the same validation
// surface for free.
package csvexport
