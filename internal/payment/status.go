// Package payment holds the rules of a payment. It talks to nothing:
// no database, no node, no HTTP. That makes the rules easy to test.
package payment

// Status is the stage a payment is in.
type Status string

const (
	Pending   Status = "PENDING"   // accepted and saved; no one has told the node yet
	InFlight  Status = "IN_FLIGHT" // the node was told to pay; outcome not known yet
	Succeeded Status = "SUCCEEDED" // the node gave us the preimage
	Failed    Status = "FAILED"    // the node said it failed; no money moved
)

// allowed lists every move a payment may make. Anything not listed is a bug.
var allowed = map[Status][]Status{
	Pending:   {InFlight, Failed},
	InFlight:  {Succeeded, Failed},
	Succeeded: {}, // final
	Failed:    {}, // final
}

// CanMove reports whether a payment may go from one status to another.
func CanMove(from, to Status) bool {
	for _, next := range allowed[from] {
		if next == to {
			return true
		}
	}
	return false
}

// Final reports whether the status can never change again.
func (s Status) Final() bool {
	return s == Succeeded || s == Failed
}
