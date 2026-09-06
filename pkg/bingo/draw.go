package bingo

import (
	"crypto/rand"
	"math/big"

	"github.com/bingo/backend/internal/domain"
)

// DrawNumber draws a random number for a given letter
// Returns the number and ensures it hasn't been drawn before
func DrawNumber(letter string, drawnNumbers []int) (int, error) {
	var min, max int

	switch letter {
	case string(domain.BingoLetterB):
		min, max = domain.BingoNumberMinB, domain.BingoNumberMaxB
	case string(domain.BingoLetterI):
		min, max = domain.BingoNumberMinI, domain.BingoNumberMaxI
	case string(domain.BingoLetterN):
		min, max = domain.BingoNumberMinN, domain.BingoNumberMaxN
	case string(domain.BingoLetterG):
		min, max = domain.BingoNumberMinG, domain.BingoNumberMaxG
	case string(domain.BingoLetterO):
		min, max = domain.BingoNumberMinO, domain.BingoNumberMaxO
	default:
		return 0, nil
	}

	// Get available numbers (not yet drawn)
	available := make([]int, 0)
	drawnSet := make(map[int]bool)
	for _, num := range drawnNumbers {
		drawnSet[num] = true
	}

	for num := min; num <= max; num++ {
		if !drawnSet[num] {
			available = append(available, num)
		}
	}

	if len(available) == 0 {
		return 0, nil // All numbers drawn for this letter
	}

	// Select random number from available
	rangeSize := big.NewInt(int64(len(available)))
	randIdx, err := rand.Int(rand.Reader, rangeSize)
	if err != nil {
		return 0, err
	}

	return available[randIdx.Int64()], nil
}

// DrawNextNumber draws the next ball uniformly at random from ALL numbers not
// yet drawn — i.e. one hopper of the remaining 75 balls, exactly like a real
// bingo hall. (Picking a letter first and then a number within it would bias the
// draw toward whichever column is more depleted, since each remaining number's
// odds would be 1/5 × 1/remaining-in-column instead of a flat 1/remaining.)
func DrawNextNumber(drawnNumbers []int) (string, int, error) {
	drawnSet := make(map[int]bool, len(drawnNumbers))
	for _, num := range drawnNumbers {
		drawnSet[num] = true
	}

	// Pool of every undrawn number across the full B..O range (1..75).
	available := make([]int, 0, domain.BingoNumberMaxO-domain.BingoNumberMinB+1)
	for num := domain.BingoNumberMinB; num <= domain.BingoNumberMaxO; num++ {
		if !drawnSet[num] {
			available = append(available, num)
		}
	}

	// All numbers drawn.
	if len(available) == 0 {
		return "", 0, nil
	}

	randIdx, err := rand.Int(rand.Reader, big.NewInt(int64(len(available))))
	if err != nil {
		return "", 0, err
	}

	number := available[randIdx.Int64()]
	return GetLetterForNumber(number), number, nil
}

