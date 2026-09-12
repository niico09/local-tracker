package domain

import "testing"

// TestTiltOwnerForGoal proves Rule 1 (I4): the tilt inside a goal belongs to the
// goal owner, so a couple goal resolves the shared tilt and a personal goal its
// owner's tilt.
func TestTiltOwnerForGoal(t *testing.T) {
	cases := []struct {
		name string
		goal Goal
		want UserID
	}{
		{"couple goal resolves shared tilt", Goal{OwnerUserID: 0}, 0},
		{"personal goal resolves owner tilt", Goal{OwnerUserID: 7}, 7},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := TiltOwnerForGoal(tc.goal); got != tc.want {
				t.Fatalf("TiltOwnerForGoal = %d, want %d", got, tc.want)
			}
		})
	}
}

// TestTiltOwnerForItem proves G5/I5: outside a goal the tilt follows the item's
// own owner, with 0 meaning shared.
func TestTiltOwnerForItem(t *testing.T) {
	if got := TiltOwnerForItem(Item{OwnerUserID: 0}); got != 0 {
		t.Fatalf("shared item tilt owner = %d, want 0", got)
	}
	if got := TiltOwnerForItem(Item{OwnerUserID: 3}); got != 3 {
		t.Fatalf("personal item tilt owner = %d, want 3", got)
	}
}

// TestGoalMemberSubjectUsesEnclosingGoal proves the membership read authorizes
// on the goal, not the item: a personal item owned by A placed in a couple goal
// is still readable by partner B because the goal is shared.
func TestGoalMemberSubjectUsesEnclosingGoal(t *testing.T) {
	item := Item{ID: 9, OwnerUserID: 1}
	couple := Goal{OwnerUserID: 0}
	member := NewGoalMember(item, TiltOwnerForGoal(couple), nil, couple)

	if member.Subject() != (Subject{OwnerID: 0}) {
		t.Fatalf("member subject = %+v, want the enclosing couple goal", member.Subject())
	}
	if err := Authorize(NewActor(2), ActionView, member.Subject()); err != nil {
		t.Fatalf("partner view of couple goal member = %v, want nil", err)
	}
	if member.TiltOwner != 0 {
		t.Fatalf("member tilt owner = %d, want shared 0", member.TiltOwner)
	}
}
