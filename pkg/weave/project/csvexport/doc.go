// Package csvexport renders member-only verification CSVs for one explicitly
// selected draft or release: owned fields, models, collections, categories,
// three raw override contexts, and directly linked ontology metadata.
//
// Source captures one complete projectsnapshot value through the shared read
// boundary. Build and CSV/ZIP serialization are pure. Current project access
// is separate from all selected content, names, references and ontology evidence.
// Downloads are fully prepared before response headers; missing release content
// returns the canonical error instead of substituting current rows.
//
// Routes under /projects/{projectID}/exports:
//
// GET /          — HTML index with selected project metadata
// GET /all.zip   — all eight CSVs from one capture
// GET /{type}.csv — one selected file
//
// Omitted version deliberately selects draft; ?version selects a concrete
// release. The existing CSV columns remain a verification format, not lossless
// project transport. Authored paths and raw overrides are not merged semantic
// views. Unknown historical active-ontology policy is blank, never read live.
package csvexport
