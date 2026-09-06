package bingo

import (
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/bingo/backend/internal/domain"
)

// numSetKey is the order-independent signature of a card's 24 numbers.
func numSetKey(c [5][5]int) string {
	vals := make([]int, 0, 24)
	for r := 0; r < 5; r++ {
		for col := 0; col < 5; col++ {
			if c[r][col] != 0 {
				vals = append(vals, c[r][col])
			}
		}
	}
	sort.Ints(vals)
	var b strings.Builder
	for _, v := range vals {
		fmt.Fprintf(&b, "%d,", v)
	}
	return b.String()
}

// The generated card table must be self-consistent so the Go backend and the TS
// frontend (which mirrors cardData500) can never disagree on a card:
//   - exactly TotalCards entries,
//   - every card already valid (so fixCard is a no-op — if a fix ran, the Go
//     table would silently differ from the raw TS mirror),
//   - all cards distinct.
func TestGeneratedCardsValidAndDistinct(t *testing.T) {
	if len(cardData500) != domain.TotalCards {
		t.Fatalf("cardData500 has %d cards, want %d", len(cardData500), domain.TotalCards)
	}
	seen := make(map[[5][5]int]int, len(cardData500))
	numSets := make(map[string]int, len(cardData500))
	for id := domain.MinCardID; id <= domain.MaxCardID; id++ {
		raw := cardData500[id-1]
		if !validateCard(&BingoCard{ID: id, Numbers: raw}) {
			t.Fatalf("card %d is invalid — fixCard would alter it and diverge from the TS table", id)
		}
		if got := GenerateCard(id); got == nil || got.Numbers != raw {
			t.Fatalf("card %d: GenerateCard does not match the raw table (a fix ran)", id)
		}
		if prev, dup := seen[raw]; dup {
			t.Fatalf("card %d is a duplicate layout of card %d", id, prev)
		}
		seen[raw] = id
		// Stronger guarantee: no two cards share the same set of 24 numbers.
		ns := numSetKey(raw)
		if prev, dup := numSets[ns]; dup {
			t.Fatalf("card %d shares the same 24 numbers as card %d", id, prev)
		}
		numSets[ns] = id
	}
}
