package csvexport

import (
	"archive/zip"
	"bytes"
	"fmt"
)

// ZIP renders every CSV from this value and closes the archive before returning
// any bytes. A failed projection/entry never produces a partial download.
func (e *Export) ZIP() ([]byte, error) {
	var out bytes.Buffer
	zw := zip.NewWriter(&out)
	for _, kind := range ExportTypes() {
		data, err := e.CSV(kind)
		if err != nil {
			return nil, err
		}
		entry, err := zw.Create(e.prefix + "-" + kind + ".csv")
		if err != nil {
			return nil, fmt.Errorf("create export entry: %w", err)
		}
		if _, err := entry.Write(data); err != nil {
			return nil, fmt.Errorf("write export entry: %w", err)
		}
	}
	if err := zw.Close(); err != nil {
		return nil, fmt.Errorf("close export archive: %w", err)
	}
	return out.Bytes(), nil
}
