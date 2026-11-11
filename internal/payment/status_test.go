package payment

import "testing"

func TestCanMove(t *testing.T) {
	cases := []struct {
		from, to Status
		want     bool
	}{
		{Pending, InFlight, true},
		{Pending, Failed, true},
		{InFlight, Succeeded, true},
		{InFlight, Failed, true},
		{Pending, Succeeded, false}, // cannot succeed without asking the node
		{Succeeded, Failed, false},  // final stays final
		{Failed, InFlight, false},   // a retry is a new row, not a revived one
		{InFlight, Pending, false},  // never backwards
	}
	for _, c := range cases {
		if got := CanMove(c.from, c.to); got != c.want {
			t.Errorf("CanMove(%s, %s) = %v, want %v", c.from, c.to, got, c.want)
		}
	}
}
