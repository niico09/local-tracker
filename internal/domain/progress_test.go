package domain

import "testing"

// TestRollup covers the goal denominator rule (G3/I8): target when set, else the
// current member count, with the done count clamped to the denominator.
func TestRollup(t *testing.T) {
	target := func(n int) *int { return &n }
	cases := []struct {
		name                    string
		target                  *int
		memberDone, memberCount int
		wantDone, wantTotal     int
	}{
		{"target set", target(100), 3, 5, 3, 100},
		{"no target uses member count", nil, 2, 4, 2, 4},
		{"zero members with no target", nil, 0, 0, 0, 0},
		{"target larger than members", target(10), 1, 2, 1, 10},
		{"clamp done above target", target(1), 2, 5, 1, 1},
		{"clamp done above member count", nil, 5, 4, 4, 4},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			done, total := Rollup(tc.target, tc.memberDone, tc.memberCount)
			if done != tc.wantDone || total != tc.wantTotal {
				t.Errorf("Rollup = %d/%d, want %d/%d", done, total, tc.wantDone, tc.wantTotal)
			}
		})
	}
}

// TestProgressSubject proves a tilt's ownership facts come from its owner, where
// 0 is the shared couple tilt.
func TestProgressSubject(t *testing.T) {
	if got := (Progress{}).Subject(); got.OwnerID != 0 || got.Granted {
		t.Errorf("shared tilt Subject = %+v, want owner 0 and no grant", got)
	}
	if got := (Progress{OwnerUserID: 2}).Subject(); got.OwnerID != 2 {
		t.Errorf("personal tilt Subject = %+v, want owner 2", got)
	}
}
