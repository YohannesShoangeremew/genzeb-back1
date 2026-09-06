package bingo

import "testing"

func TestValidateBingo_FourCornersWins(t *testing.T) {
	card := GenerateCard(1)

	marked := []int{
		card.Numbers[0][0],
		card.Numbers[0][4],
		card.Numbers[4][0],
		card.Numbers[4][4],
	}

	if !ValidateBingo(card, marked) {
		t.Fatalf("expected four corners to be a valid winning pattern")
	}
}

func TestValidateBingo_ThreeCornersDoesNotWin(t *testing.T) {
	card := GenerateCard(1)

	marked := []int{
		card.Numbers[0][0],
		card.Numbers[0][4],
		card.Numbers[4][0],
	}

	if ValidateBingo(card, marked) {
		t.Fatalf("expected three corners to be invalid")
	}
}

// autoMarked mirrors the auto-daub marking done in GameUseCase.checkAutoBingo:
// a cell counts as marked when it is the free center or its number was drawn.
func autoMarked(card *BingoCard, drawn map[int]bool) []int {
	marked := make([]int, 0, 25)
	for row := 0; row < 5; row++ {
		for col := 0; col < 5; col++ {
			n := card.Numbers[row][col]
			if n == 0 || drawn[n] {
				marked = append(marked, n)
			}
		}
	}
	return marked
}

// TestAutoBingo_CompletesRowFromDrawnNumbers models automatic detection: once
// every number in a row has been drawn, auto-daubbing yields a winning card
// without any manual claim.
func TestAutoBingo_CompletesRowFromDrawnNumbers(t *testing.T) {
	card := GenerateCard(1)

	// Draw exactly the four non-center numbers of the middle row (col 2 is the
	// free center). The row should then be a valid bingo via auto-daub.
	drawn := map[int]bool{}
	for col := 0; col < 5; col++ {
		if n := card.Numbers[2][col]; n != 0 {
			drawn[n] = true
		}
	}

	if !ValidateBingo(card, autoMarked(card, drawn)) {
		t.Fatalf("expected a fully drawn row to auto-complete bingo")
	}
}

// TestAutoBingo_PartialRowDoesNotWin guards against declaring a winner before a
// pattern is actually complete — a near-miss row must not trigger.
func TestAutoBingo_PartialRowDoesNotWin(t *testing.T) {
	card := GenerateCard(1)

	// Draw the middle row except one cell.
	drawn := map[int]bool{}
	for col := 0; col < 4; col++ {
		if n := card.Numbers[2][col]; n != 0 {
			drawn[n] = true
		}
	}

	if ValidateBingo(card, autoMarked(card, drawn)) {
		t.Fatalf("expected an incomplete row not to win")
	}
}
