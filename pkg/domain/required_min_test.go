package domain

import "testing"

func TestRequiredMin(t *testing.T) {
	cases := []struct {
		required bool
		min      int
		wantReq  bool
		wantMin  int
	}{
		{false, 0, false, 0},
		{true, 0, true, 1},
		{false, 1, true, 1},
		{true, 3, true, 3},
		{false, 2, true, 2},
	}
	for _, c := range cases {
		req, min := RequiredMin(c.required, c.min)
		if req != c.wantReq || min != c.wantMin {
			t.Errorf("RequiredMin(%v, %d) = %v, %d; want %v, %d", c.required, c.min, req, min, c.wantReq, c.wantMin)
		}
	}
}
