package domain

import "testing"

// TestGoalSubjectAndVisibility proves Subject() carries the two ownership facts
// the single access rule consumes: the owner and whether partner read is granted.
func TestGoalSubjectAndVisibility(t *testing.T) {
	cases := []struct {
		name    string
		goal    Goal
		owner   UserID
		granted bool
	}{
		{"couple goal", Goal{OwnerUserID: 0, Visibility: VisibilityPrivate}, 0, false},
		{"personal private", Goal{OwnerUserID: 7, Visibility: VisibilityPrivate}, 7, false},
		{"personal shared", Goal{OwnerUserID: 7, Visibility: VisibilityShared}, 7, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := tc.goal.Subject()
			if s.OwnerID != tc.owner || s.Granted != tc.granted {
				t.Fatalf("Subject = %+v, want owner %d granted %v", s, tc.owner, tc.granted)
			}
		})
	}
}

func TestVisibilityValid(t *testing.T) {
	if !VisibilityPrivate.Valid() || !VisibilityShared.Valid() {
		t.Fatal("known visibilities must be valid")
	}
	if Visibility("").Valid() || Visibility("public").Valid() {
		t.Fatal("unknown visibility must be invalid")
	}
}
