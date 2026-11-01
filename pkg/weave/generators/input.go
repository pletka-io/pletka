package generators

import (
	"encoding/json"
	"fmt"
)

// EncodeInput serializes the versioned, detached renderer input. A future Lua
// adapter can decode these tables without access to Go readers or transactions.
// This is a rendering contract, not the lossless project restore format.
func EncodeInput(snapshot *Snapshot) ([]byte, error) {
	if snapshot == nil || snapshot.SchemaVersion != 1 {
		return nil, fmt.Errorf("generator input requires schema version 1")
	}
	return json.Marshal(snapshot)
}

func cloneSnapshot(snapshot *Snapshot) (*Snapshot, error) {
	if snapshot == nil {
		return nil, fmt.Errorf("generator snapshot is nil")
	}
	data, err := json.Marshal(snapshot)
	if err != nil {
		return nil, err
	}
	var copy Snapshot
	if err := json.Unmarshal(data, &copy); err != nil {
		return nil, err
	}
	return &copy, nil
}
