package domain

import "time"

// StagingRecord holds a raw Airtable record for deferred resolution.
// Domain models never carry raw Airtable data — it lives here.
// ID is DB-generated (bigserial) to prevent silent overwrites on duplicate inserts.
type StagingRecord struct {
	ID          int64     `json:"id"`
	ImportRun   string    `json:"import_run"`
	SourceType  string    `json:"source_type"`
	SourcePath  string    `json:"source_path"`
	ProjectName string    `json:"project_name"`
	SemanticID  string    `json:"semantic_id,omitempty"`
	AirtableRef string    `json:"airtable_ref,omitempty"`
	RawData     []byte    `json:"raw_data"`
	Status      string    `json:"status"`
	ErrorMsg    string    `json:"error_msg,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}
