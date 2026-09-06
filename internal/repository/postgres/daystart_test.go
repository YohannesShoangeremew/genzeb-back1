package postgres

import (
	"testing"
	"time"
)

// wallClock models what the driver actually stores in a `timestamp without
// time zone` column: the time.Time rendered in ITS OWN location, with the
// offset then discarded.
//
// Using the value's own location is the whole point. Two time.Time values can
// name the same instant while carrying different locations — midnightEAT.UTC()
// and midnightEAT.In(EAT) do — and they serialize to DIFFERENT wall clocks,
// three hours apart. A helper that re-converted into a common location here
// would normalize that away and make these tests pass against the very bug
// they exist to catch.
func wallClock(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), t.Second(), t.Nanosecond(), time.UTC)
}

// The property that actually matters: a withdrawal counts toward today's cap
// if and only if it happened on or after the current Ethiopian midnight —
// whatever timezone the app process runs in. The old implementation ended in
// .UTC() and only held while the process was UTC; under EAT the window opened
// at 21:00 the previous evening, so yesterday's late withdrawals counted
// against today's cap and blocked players early.
func TestEthiopianDayStartIsCorrectInAnyProcessTimezone(t *testing.T) {
	locals := map[string]*time.Location{
		"UTC":       time.UTC,
		"EAT":       eatZone,
		"UTC-5":     time.FixedZone("WEIRD", -5*60*60),
		"UTC+13":    time.FixedZone("FAR", 13*60*60),
		"half-hour": time.FixedZone("IST", 5*60*60+30*60),
	}

	// "Now" is 10:00 EAT on 18 July 2026.
	now := time.Date(2026, 7, 18, 10, 0, 0, 0, eatZone)

	cases := []struct {
		name    string
		instant time.Time
		counts  bool
	}{
		{"just after EAT midnight today", time.Date(2026, 7, 18, 0, 0, 1, 0, eatZone), true},
		{"this morning", time.Date(2026, 7, 18, 9, 59, 0, 0, eatZone), true},
		{"exactly EAT midnight today", time.Date(2026, 7, 18, 0, 0, 0, 0, eatZone), true},
		{"one second before EAT midnight", time.Date(2026, 7, 17, 23, 59, 59, 0, eatZone), false},
		{"yesterday 23:00 EAT", time.Date(2026, 7, 17, 23, 0, 0, 0, eatZone), false},
		{"yesterday 21:30 EAT (the old bug's window)", time.Date(2026, 7, 17, 21, 30, 0, 0, eatZone), false},
		{"yesterday midday", time.Date(2026, 7, 17, 12, 0, 0, 0, eatZone), false},
	}

	for localName, local := range locals {
		dayStart := wallClock(ethiopianDayStart(now, local))
		for _, tc := range cases {
			stored := wallClock(tc.instant.In(local)) // as transactionRepository.Create writes it
			counted := !stored.Before(dayStart)
			if counted != tc.counts {
				t.Errorf("process TZ %s: %q counted=%v, want %v (stored %v, dayStart %v)",
					localName, tc.name, counted, tc.counts, stored, dayStart)
			}
		}
	}
}

// The boundary must track the Ethiopian day even when the process timezone
// puts "now" on a different calendar date.
func TestEthiopianDayStartAcrossDateLineDisagreement(t *testing.T) {
	// 01:00 EAT on 18 July is still 22:00 UTC on 17 July — the process and the
	// Ethiopian calendar disagree about what day it is.
	now := time.Date(2026, 7, 18, 1, 0, 0, 0, eatZone)
	if got := now.UTC().Day(); got != 17 {
		t.Fatalf("precondition: expected UTC to still be the 17th, got %d", got)
	}

	for _, local := range []*time.Location{time.UTC, eatZone} {
		start := ethiopianDayStart(now, local)
		inEAT := start.In(eatZone)
		if inEAT.Day() != 18 || inEAT.Hour() != 0 || inEAT.Minute() != 0 {
			t.Errorf("day start in EAT terms = %v, want 2026-07-18 00:00 EAT", inEAT)
		}
	}
}

// Guards the specific regression: under a UTC process the result must still be
// the previous calendar day at 21:00, which is what makes the existing
// production behaviour correct.
func TestEthiopianDayStartUTCProcessMatchesPreviousBehaviour(t *testing.T) {
	now := time.Date(2026, 7, 18, 10, 0, 0, 0, eatZone)
	got := ethiopianDayStart(now, time.UTC)
	want := time.Date(2026, 7, 17, 21, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("UTC process: day start = %v, want %v", got, want)
	}
}
