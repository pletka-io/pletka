package model

import (
	"testing"

	"github.com/pletka-io/pletka/pkg/domain"
)

func TestOwnedActionFlags(t *testing.T) {
	tests := []struct {
		name                             string
		modelProject                     string
		deprecated                       bool
		viewerProject                    string
		wantOwned, wantDeprec, wantActiv bool
	}{
		{"own, active", "LA", false, "LA", true, true, false},
		{"own, deprecated", "LA", true, "LA", true, false, true},
		{"foreign (reference-adopted)", "SRD", false, "LA", false, false, false},
		{"foreign, deprecated", "SRD", true, "LA", false, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &domain.Model{Entity: domain.Entity{ProjectID: tt.modelProject, Deprecated: tt.deprecated}}
			owned, canDeprec, canActiv := ownedActionFlags(m, tt.viewerProject)
			if owned != tt.wantOwned || canDeprec != tt.wantDeprec || canActiv != tt.wantActiv {
				t.Errorf("got (owned=%v,deprec=%v,activ=%v) want (%v,%v,%v)",
					owned, canDeprec, canActiv, tt.wantOwned, tt.wantDeprec, tt.wantActiv)
			}
		})
	}
}
