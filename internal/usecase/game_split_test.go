package usecase

import (
	"math"
	"testing"
)

// sum is a tiny helper for the split tests.
func sum(xs []float64) float64 {
	total := 0.0
	for _, x := range xs {
		total += x
	}
	return total
}

func TestSplitPot_EvenlyDivisible(t *testing.T) {
	shares := splitPot(18.0, 2)
	if len(shares) != 2 {
		t.Fatalf("expected 2 shares, got %d", len(shares))
	}
	if shares[0] != 9.0 || shares[1] != 9.0 {
		t.Fatalf("expected 9/9, got %v", shares)
	}
}

func TestSplitPot_RemainderGoesToFirst(t *testing.T) {
	// 20 / 3 = 6.6667 → 6.67 each rounds to 20.01, so the first winner absorbs
	// the -0.01 to keep the total at exactly 20.
	shares := splitPot(20.0, 3)
	if math.Abs(sum(shares)-20.0) > 1e-9 {
		t.Fatalf("shares must sum to the pool exactly, got %v (sum %.4f)", shares, sum(shares))
	}
	for i, s := range shares {
		if s <= 0 {
			t.Fatalf("share %d should be positive, got %v", i, s)
		}
	}
}

func TestSplitPot_NeverExceedsPool(t *testing.T) {
	// Property check across a range of pools and winner counts: the payout total
	// must never exceed the pool (the house can't lose money to rounding).
	for _, pool := range []float64{10, 20, 50, 33.33, 100, 7} {
		for n := 1; n <= 8; n++ {
			shares := splitPot(pool, n)
			if len(shares) != n {
				t.Fatalf("pool %.2f n %d: expected %d shares, got %d", pool, n, n, len(shares))
			}
			if sum(shares) > pool+1e-9 {
				t.Fatalf("pool %.2f n %d: payout %.4f exceeds pool", pool, n, sum(shares))
			}
			if math.Abs(sum(shares)-pool) > 0.01 {
				t.Fatalf("pool %.2f n %d: payout %.4f drifts from pool", pool, n, sum(shares))
			}
		}
	}
}

func TestSplitPot_SingleWinnerGetsWholePool(t *testing.T) {
	shares := splitPot(50.0, 1)
	if len(shares) != 1 || shares[0] != 50.0 {
		t.Fatalf("single winner should take the whole pool, got %v", shares)
	}
}
