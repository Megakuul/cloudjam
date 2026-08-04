package challenge

import (
	"codeberg.org/megakuul/cloudjam/pkg/challenge/api"
	"errors"
	"testing"
	"time"
)

type fakeHost struct {
	meta        api.InitInput
	registered  int
	registerErr error

	awards   []award
	awardErr error
	reports  []string
	logs     []string
}

type award struct {
	reason string
	points float64
}

func (f *fakeHost) Register(meta api.InitInput) error {
	f.registered++
	if f.registerErr != nil {
		return f.registerErr
	}
	f.meta = meta
	return nil
}

func (f *fakeHost) Report(message string) error {
	f.reports = append(f.reports, message)
	return nil
}

func (f *fakeHost) Award(reason string, points float64) error {
	if f.awardErr != nil {
		return f.awardErr
	}
	f.awards = append(f.awards, award{reason, points})
	return nil
}

func (f *fakeHost) Log(message string) { f.logs = append(f.logs, message) }

// newChallenge returns a challenge on a virtual clock, so Run finishes instantly
// and the timers are exact. Sleeping advances the clock.
func newChallenge(host Host) *Challenge {
	at := time.Unix(1e9, 0)
	c := New(host, "Test", "")
	c.now = func() time.Time { return at }
	c.sleep = func(d time.Duration) { at = at.Add(d) }
	return c
}

func TestRunRegistersAndFinishesWhenNothingIsLeft(t *testing.T) {
	f := &fakeHost{}
	c := newChallenge(f)

	c.Check("instant").Points(10).Done(func() (bool, error) { return true, nil })
	c.Run()

	if f.registered != 1 {
		t.Errorf("registered %d times, want 1", f.registered)
	}
	if len(f.awards) != 1 || f.awards[0].points != 10 {
		t.Errorf("awards = %v", f.awards)
	}
}

func TestRunProvisionsBeforeChecking(t *testing.T) {
	f := &fakeHost{}
	c := newChallenge(f)

	provisioned := false
	c.Provision("bucket", func() error {
		provisioned = true
		return nil
	})
	c.Check("sees-it").Done(func() (bool, error) { return provisioned, nil })

	c.Run()

	if len(f.awards) != 1 {
		t.Errorf("check ran before provisioning: %v", f.awards)
	}
}

func TestProvisioningRetriesUntilItSucceeds(t *testing.T) {
	f := &fakeHost{}
	c := newChallenge(f)

	attempts := 0
	c.Provision("flaky", func() error {
		attempts++
		if attempts < 3 {
			return errors.New("throttled")
		}
		return nil
	})

	c.Run()

	if attempts != 3 {
		t.Errorf("ran %d times, want 3", attempts)
	}
	if len(f.reports) != 2 {
		t.Errorf("reports = %v, want both failures reported", f.reports)
	}
}

func TestCheckAwardsOnceByDefault(t *testing.T) {
	f := &fakeHost{}
	c := newChallenge(f)

	runs := 0
	c.Check("once").Points(50).Done(func() (bool, error) {
		runs++
		return true, nil
	})
	// A second, never-passing check keeps the loop alive with a timeout.
	c.Check("never").Done(func() (bool, error) { return false, nil })
	c.Timeout(time.Minute)

	c.Run()

	if len(f.awards) != 1 {
		t.Errorf("awarded %d times, want exactly 1: %v", len(f.awards), f.awards)
	}
	if runs != 1 {
		t.Errorf("check ran %d times after passing, want 1 (retired)", runs)
	}
}

func TestRepeatAwardsEveryRound(t *testing.T) {
	f := &fakeHost{}
	c := newChallenge(f)

	c.Check("held").Points(5).Repeat().Done(func() (bool, error) { return true, nil })
	c.Timeout(30 * time.Second) // 10s interval => rounds at 0s, 10s, 20s, 30s

	c.Run()

	if len(f.awards) != 4 {
		t.Errorf("awarded %d times, want one per round: %v", len(f.awards), f.awards)
	}
	for _, a := range f.awards {
		if a.points != 5 {
			t.Errorf("award = %v, want 5 points each", a)
		}
	}
}

func TestPointsFromComputesTheReward(t *testing.T) {
	f := &fakeHost{}
	c := newChallenge(f)

	cleaned := 0
	c.Check("cleanup").
		Reason("Cleaned up").
		PointsFrom(func() (float64, error) { return float64(10 * cleaned), nil }).
		Repeat().
		Done(func() (bool, error) {
			cleaned++
			return true, nil
		})
	c.Timeout(20 * time.Second)

	c.Run()

	want := []float64{10, 20, 30}
	if len(f.awards) != len(want) {
		t.Fatalf("awards = %v", f.awards)
	}
	for i, a := range f.awards {
		if a.points != want[i] {
			t.Errorf("round %d awarded %g, want %g", i, a.points, want[i])
		}
		if a.reason != "Cleaned up" {
			t.Errorf("reason = %q", a.reason)
		}
	}
}

func TestPointsFromErrorIsReportedAndRetried(t *testing.T) {
	f := &fakeHost{}
	c := newChallenge(f)

	c.Check("broken").
		PointsFrom(func() (float64, error) { return 0, errors.New("cannot count") }).
		Done(func() (bool, error) { return true, nil })
	c.Timeout(20 * time.Second)

	c.Run()

	if len(f.awards) != 0 {
		t.Errorf("awarded despite the points error: %v", f.awards)
	}
	if len(f.reports) == 0 {
		t.Error("points error was not reported")
	}
}

func TestEveryThrottlesEvaluation(t *testing.T) {
	f := &fakeHost{}
	c := newChallenge(f)

	runs := 0
	c.Check("expensive").Every(time.Minute).Done(func() (bool, error) {
		runs++
		return false, nil
	})
	c.Timeout(2 * time.Minute) // 13 rounds at 10s, but only ~3 are due

	c.Run()

	if runs != 3 {
		t.Errorf("ran %d times, want 3 (at 0s, 60s, 120s)", runs)
	}
}

func TestReasonDefaultsToID(t *testing.T) {
	f := &fakeHost{}
	c := newChallenge(f)

	c.Check("bucket-encrypted").Points(1).Done(func() (bool, error) { return true, nil })
	c.Run()

	if len(f.awards) != 1 || f.awards[0].reason != "bucket-encrypted" {
		t.Errorf("awards = %v", f.awards)
	}
}

func TestFailedAwardIsRetried(t *testing.T) {
	f := &fakeHost{awardErr: errors.New("score service down")}
	c := newChallenge(f)

	c.Check("x").Points(1).Done(func() (bool, error) { return true, nil })
	c.Timeout(30 * time.Second)

	// First rounds fail; recover partway through.
	rounds := 0
	realSleep := c.sleep
	c.sleep = func(d time.Duration) {
		rounds++
		if rounds == 2 {
			f.awardErr = nil
		}
		realSleep(d)
	}

	c.Run()

	if len(f.awards) != 1 {
		t.Errorf("awards = %v, want the retry to succeed exactly once", f.awards)
	}
	if len(f.reports) < 2 {
		t.Errorf("reports = %v, want the failures recorded", f.reports)
	}
}

func TestCheckErrorIsReportedAndDoesNotAward(t *testing.T) {
	f := &fakeHost{}
	c := newChallenge(f)

	c.Check("flaky").Done(func() (bool, error) { return false, errors.New("read failed") })
	c.Timeout(20 * time.Second)

	c.Run()

	if len(f.awards) != 0 {
		t.Errorf("awarded despite the error: %v", f.awards)
	}
	if len(f.reports) != 3 {
		t.Errorf("reports = %v, want one per round", f.reports)
	}
}

func TestCheckWithoutConditionIsReported(t *testing.T) {
	f := &fakeHost{}
	c := newChallenge(f)

	c.Check("forgot-done").Points(10)
	c.Run()

	if len(f.reports) != 1 {
		t.Errorf("reports = %v, want the missing condition reported", f.reports)
	}
	if len(f.awards) != 0 {
		t.Errorf("awarded a check with no condition: %v", f.awards)
	}
}

func TestRegistrationFailureDoesNotStopTheChallenge(t *testing.T) {
	f := &fakeHost{registerErr: errors.New("host down")}
	c := newChallenge(f)

	c.Check("still-runs").Points(1).Done(func() (bool, error) { return true, nil })
	c.Run()

	if len(f.reports) != 1 {
		t.Errorf("reports = %v", f.reports)
	}
	if len(f.awards) != 1 {
		t.Errorf("challenge stopped after a registration failure: %v", f.awards)
	}
}

func TestTimeoutStopsAnEndlessChallenge(t *testing.T) {
	f := &fakeHost{}
	c := newChallenge(f)

	c.Check("never").Done(func() (bool, error) { return false, nil })
	c.Timeout(time.Minute)

	c.Run() // would loop forever without the timeout

	if len(f.awards) != 0 {
		t.Errorf("awards = %v", f.awards)
	}
}

func TestBuilderOrderDoesNotMatter(t *testing.T) {
	f := &fakeHost{}
	c := newChallenge(f)

	c.Check("a").Done(func() (bool, error) { return true, nil }).Points(7).Reason("late reason")
	c.Run()

	if len(f.awards) != 1 || f.awards[0].points != 7 || f.awards[0].reason != "late reason" {
		t.Errorf("awards = %v", f.awards)
	}
}
