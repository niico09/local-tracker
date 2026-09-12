package domain

import (
	"errors"
	"testing"
)

func TestAuthorizeMatrix(t *testing.T) {
	const owner UserID = 1
	ownerActor := NewActor(owner)
	partner := NewActor(2)
	couple := Subject{OwnerID: 0}
	privateGoal := Subject{OwnerID: owner}
	grantedGoal := Subject{OwnerID: owner, Granted: true}

	cases := []struct {
		name    string
		actor   Actor
		action  Action
		subject Subject
		want    error
	}{
		{"anonymous views couple", Actor{}, ActionView, couple, ErrNoActor},
		{"anonymous modifies couple", Actor{}, ActionModify, couple, ErrNoActor},
		{"owner views own personal", ownerActor, ActionView, privateGoal, nil},
		{"owner modifies own personal", ownerActor, ActionModify, privateGoal, nil},
		{"partner views private", partner, ActionView, privateGoal, ErrForbidden},
		{"partner modifies private", partner, ActionModify, privateGoal, ErrForbidden},
		{"partner views granted", partner, ActionView, grantedGoal, nil},
		{"partner modifies granted", partner, ActionModify, grantedGoal, ErrForbidden},
		{"partner views couple", partner, ActionView, couple, nil},
		{"partner modifies couple", partner, ActionModify, couple, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := Authorize(tc.actor, tc.action, tc.subject); !errors.Is(err, tc.want) {
				t.Fatalf("Authorize = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestActorSemantics(t *testing.T) {
	if err := (Actor{}).Require(); !errors.Is(err, ErrNoActor) {
		t.Errorf("zero actor Require = %v, want ErrNoActor", err)
	}
	if err := NewActor(7).Require(); err != nil {
		t.Errorf("real actor Require = %v, want nil", err)
	}
	if !SystemActor().Valid() {
		t.Error("system actor must be valid")
	}
}
