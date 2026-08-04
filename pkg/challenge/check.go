package challenge

import "time"

// Spec is a scored objective under construction. Check returns one; every method
// returns it again so the declaration reads as one sentence:
//
//	challenge.Check("bucket-encrypted").
//		Reason("Enabled default encryption").
//		Points(50).
//		Every(30 * time.Second).
//		Done(encrypted)
//
// The check is live from the moment Check returns, so the methods may be called
// in any order and any of them may be left out.
type Spec struct {
	id     string
	reason string
	repeat bool

	points  float64
	dynamic func() (float64, error)

	done     func() (bool, error)
	interval time.Duration

	last    time.Time
	retired bool
}

// Reason sets the text shown next to the score update. It defaults to the
// check's id.
func (s *Spec) Reason(reason string) *Spec {
	s.reason = reason
	return s
}

// Points sets a fixed reward.
func (s *Spec) Points(points float64) *Spec {
	s.points = points
	s.dynamic = nil
	return s
}

// PointsFrom computes the reward at award time, for a payout that depends on how
// well the player did — how many buckets they encrypted, how much they trimmed
// the bill, how fast they got there:
//
//	challenge.Check("cleanup").
//		PointsFrom(func() (float64, error) {
//			left, err := aws.List[*ec2.Instance]()
//			return float64(10 * (5 - len(left))), err
//		}).
//		Done(anyProgress)
//
// It overrides Points.
func (s *Spec) PointsFrom(points func() (float64, error)) *Spec {
	s.dynamic = points
	return s
}

// Every throttles evaluation: the check is skipped until this much time has
// passed since it last ran. Use it for conditions too expensive to test on every
// round. Zero evaluates every round.
func (s *Spec) Every(interval time.Duration) *Spec {
	s.interval = interval
	return s
}

// Repeat awards the check every round its condition holds, instead of once.
// Use it for a state worth holding — an instance kept healthy, a bill kept under
// budget. Without it a check pays out the first time it passes and is retired.
func (s *Spec) Repeat() *Spec {
	s.repeat = true
	return s
}

// Done sets the condition being scored.
func (s *Spec) Done(condition func() (bool, error)) *Spec {
	s.done = condition
	return s
}

// due reports whether the check may run again at now.
func (s *Spec) due(now time.Time) bool {
	if s.last.IsZero() || s.interval <= 0 {
		return true
	}
	return now.Sub(s.last) >= s.interval
}

// award resolves the reward, fixed or computed.
func (s *Spec) award() (float64, error) {
	if s.dynamic != nil {
		return s.dynamic()
	}
	return s.points, nil
}

// why is the text attached to the score update.
func (s *Spec) why() string {
	if s.reason != "" {
		return s.reason
	}
	return s.id
}
