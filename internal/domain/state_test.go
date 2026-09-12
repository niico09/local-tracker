package domain

import (
	"errors"
	"testing"
	"time"
)

func dateRef(y int, m time.Month, d int) *Date {
	v := NewDate(y, m, d)
	return &v
}

// TestDeriveStateTruthTable covers the done-authoritative rule (G2/I7).
func TestDeriveStateTruthTable(t *testing.T) {
	start := dateRef(2024, time.January, 1)
	end := dateRef(2024, time.February, 1)
	cases := []struct {
		name string
		p    Progress
		want State
	}{
		{"done with no dates", Progress{Done: true}, StateCompleted},
		{"done with a start date", Progress{Done: true, StartDate: start}, StateCompleted},
		{"done with both dates", Progress{Done: true, StartDate: start, EndDate: end}, StateCompleted},
		{"open with no dates", Progress{}, StatePending},
		{"started without done", Progress{StartDate: start}, StateInProgress},
		{"end date alone does not start", Progress{EndDate: end}, StatePending},
		{"started with an end date", Progress{StartDate: start, EndDate: end}, StateInProgress},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := DeriveState(tc.p); got != tc.want {
				t.Errorf("DeriveState(%+v) = %q, want %q", tc.p, got, tc.want)
			}
		})
	}
}

// TestValidateDates covers the end >= start rule (I6).
func TestValidateDates(t *testing.T) {
	start := dateRef(2024, time.January, 10)
	same := dateRef(2024, time.January, 10)
	later := dateRef(2024, time.January, 20)
	earlier := dateRef(2024, time.January, 5)

	cases := []struct {
		name    string
		start   *Date
		end     *Date
		wantErr bool
	}{
		{"both nil", nil, nil, false},
		{"start only", start, nil, false},
		{"end only", nil, later, false},
		{"equal dates allowed", start, same, false},
		{"end after start", start, later, false},
		{"end before start", start, earlier, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateDates(tc.start, tc.end)
			if tc.wantErr {
				if !errors.Is(err, ErrValidation) {
					t.Fatalf("ValidateDates = %v, want ErrValidation", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("ValidateDates = %v, want nil", err)
			}
		})
	}
}

func TestKindValid(t *testing.T) {
	for _, k := range []Kind{KindSeries, KindMovie, KindBook, KindCourse} {
		if !k.Valid() {
			t.Errorf("%q.Valid() = false, want true", k)
		}
	}
	if Kind("album").Valid() {
		t.Error("unknown kind reported valid")
	}
}
