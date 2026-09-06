package bingo

import "github.com/bingo/backend/internal/domain"

// BingoCard represents a 5x5 bingo card
type BingoCard struct {
	ID      int       `json:"id"`
	Numbers [5][5]int `json:"numbers"` // 5x5 grid: [row][col]
}

var (
	// fixedCards stores all predefined bingo cards
	// Card ID 1-500, each with fixed numbers
	fixedCards map[int]*BingoCard
)

func init() {
	// Initialize fixed cards on package load
	fixedCards = generateFixedCards()
}

// GenerateCard returns a fixed bingo card for the given card ID
// Card ID must be between MinCardID and MaxCardID
// Each card ID always returns the same card (deterministic)
func GenerateCard(cardID int) *BingoCard {
	if cardID < domain.MinCardID || cardID > domain.MaxCardID {
		return nil
	}

	card, exists := fixedCards[cardID]
	if !exists {
		return nil
	}

	// Return a copy to prevent modification
	return &BingoCard{
		ID:      card.ID,
		Numbers: card.Numbers,
	}
}

// GenerateAllCards returns all fixed bingo cards
func GenerateAllCards() []*BingoCard {
	cards := make([]*BingoCard, domain.TotalCards)
	for i := domain.MinCardID; i <= domain.MaxCardID; i++ {
		cards[i-domain.MinCardID] = GenerateCard(i)
	}
	return cards
}

// generateFixedCards creates all fixed bingo cards from hardcoded data
// Each card has unique numbers but the same card ID always has the same numbers
func generateFixedCards() map[int]*BingoCard {
	cards := make(map[int]*BingoCard, domain.TotalCards)

	// Parse card data into BingoCard structures
	for cardID := domain.MinCardID; cardID <= domain.MaxCardID; cardID++ {
		card := &BingoCard{
			ID:      cardID,
			Numbers: cardData500[cardID-domain.MinCardID],
		}

		// Validate card follows bingo rules
		if !validateCard(card) {
			// Fix any violations
			fixCard(card)
		}

		cards[cardID] = card
	}

	return cards
}

// validateCard checks if a card follows bingo rules
func validateCard(card *BingoCard) bool {
	ranges := [5]struct{ min, max int }{
		{domain.BingoNumberMinB, domain.BingoNumberMaxB}, // B
		{domain.BingoNumberMinI, domain.BingoNumberMaxI}, // I
		{domain.BingoNumberMinN, domain.BingoNumberMaxN}, // N
		{domain.BingoNumberMinG, domain.BingoNumberMaxG}, // G
		{domain.BingoNumberMinO, domain.BingoNumberMaxO}, // O
	}

	// Check each column
	for col := 0; col < domain.CardGridSize; col++ {
		used := make(map[int]bool)
		for row := 0; row < domain.CardGridSize; row++ {
			num := card.Numbers[row][col]

			// Center cell should be CardCenterValue
			if col == domain.CardCenterCol && row == domain.CardCenterRow {
				if num != domain.CardCenterValue {
					return false
				}
				continue
			}

			// Check number is in valid range
			if num < ranges[col].min || num > ranges[col].max {
				return false
			}

			// Check for duplicates in column
			if used[num] {
				return false
			}
			used[num] = true
		}
	}

	return true
}

// fixCard fixes any bingo rule violations in a card
func fixCard(card *BingoCard) {
	ranges := [5]struct{ min, max int }{
		{domain.BingoNumberMinB, domain.BingoNumberMaxB}, // B
		{domain.BingoNumberMinI, domain.BingoNumberMaxI}, // I
		{domain.BingoNumberMinN, domain.BingoNumberMaxN}, // N
		{domain.BingoNumberMinG, domain.BingoNumberMaxG}, // G
		{domain.BingoNumberMinO, domain.BingoNumberMaxO}, // O
	}

	// Fix each column
	for col := 0; col < domain.CardGridSize; col++ {
		used := make(map[int]bool)
		available := make([]int, 0)

		// Build available numbers for this column
		for n := ranges[col].min; n <= ranges[col].max; n++ {
			available = append(available, n)
		}

		for row := 0; row < domain.CardGridSize; row++ {
			// Center cell should be CardCenterValue
			if col == domain.CardCenterCol && row == domain.CardCenterRow {
				card.Numbers[row][col] = domain.CardCenterValue
				continue
			}

			num := card.Numbers[row][col]

			// If number is out of range or duplicate, fix it
			if num < ranges[col].min || num > ranges[col].max || used[num] {
				// Find first available number
				for _, availNum := range available {
					if !used[availNum] {
						card.Numbers[row][col] = availNum
						used[availNum] = true
						break
					}
				}
			} else {
				used[num] = true
			}
		}
	}
}

// ValidateBingo checks if the marked numbers form a valid bingo
// A valid bingo can be:
// - Any row (5 numbers)
// - Any column (5 numbers)
// - Main diagonal (top-left to bottom-right)
// - Anti-diagonal (top-right to bottom-left)
// - Four corners (top-left, top-right, bottom-left, bottom-right)
func ValidateBingo(card *BingoCard, markedNumbers []int) bool {
	if len(markedNumbers) < 4 {
		return false
	}

	// Convert marked numbers to a set for quick lookup
	marked := make(map[int]bool)
	for _, num := range markedNumbers {
		marked[num] = true
	}

	// Check rows
	for row := 0; row < 5; row++ {
		count := 0
		for col := 0; col < 5; col++ {
			num := card.Numbers[row][col]
			// Center cell is always considered marked
			if num == 0 || marked[num] {
				count++
			}
		}
		if count == 5 {
			return true
		}
	}

	// Check columns
	for col := 0; col < 5; col++ {
		count := 0
		for row := 0; row < 5; row++ {
			num := card.Numbers[row][col]
			// Center cell is always considered marked
			if num == 0 || marked[num] {
				count++
			}
		}
		if count == 5 {
			return true
		}
	}

	// Check main diagonal (top-left to bottom-right)
	count := 0
	for i := 0; i < 5; i++ {
		num := card.Numbers[i][i]
		if num == 0 || marked[num] {
			count++
		}
	}
	if count == 5 {
		return true
	}

	// Check anti-diagonal (top-right to bottom-left)
	count = 0
	for i := 0; i < 5; i++ {
		num := card.Numbers[i][4-i]
		if num == 0 || marked[num] {
			count++
		}
	}
	if count == 5 {
		return true
	}

	// Check four corners
	corners := [][2]int{
		{0, 0}, // top-left
		{0, 4}, // top-right
		{4, 0}, // bottom-left
		{4, 4}, // bottom-right
	}

	count = 0
	for _, pos := range corners {
		num := card.Numbers[pos[0]][pos[1]]
		if num == 0 || marked[num] {
			count++
		}
	}
	if count == 4 {
		return true
	}

	return false
}

// GetLetterForNumber returns the BINGO letter for a given number
func GetLetterForNumber(num int) string {
	switch {
	case num >= 1 && num <= 15:
		return "B"
	case num >= 16 && num <= 30:
		return "I"
	case num >= 31 && num <= 45:
		return "N"
	case num >= 46 && num <= 60:
		return "G"
	case num >= 61 && num <= 75:
		return "O"
	default:
		return ""
	}
}

// CompletingBingoNumbers returns all undrawn numbers that would complete at
// least one valid bingo pattern (row, column, or diagonal) on the card. The
// center cell (value 0) is treated as already marked.
func CompletingBingoNumbers(card *BingoCard, drawnSet map[int]bool) []int {
	marked := make(map[int]bool, len(drawnSet)+1)
	for n := range drawnSet {
		marked[n] = true
	}
	marked[0] = true

	completing := make([]int, 0)

	rows := func() {
		for row := 0; row < 5; row++ {
			unmarked := make([]int, 0)
			for col := 0; col < 5; col++ {
				n := card.Numbers[row][col]
				if !marked[n] {
					unmarked = append(unmarked, n)
				}
			}
			if len(unmarked) == 1 {
				completing = append(completing, unmarked[0])
			}
		}
	}
	cols := func() {
		for col := 0; col < 5; col++ {
			unmarked := make([]int, 0)
			for row := 0; row < 5; row++ {
				n := card.Numbers[row][col]
				if !marked[n] {
					unmarked = append(unmarked, n)
				}
			}
			if len(unmarked) == 1 {
				completing = append(completing, unmarked[0])
			}
		}
	}
	diags := func() {
		unmarked := make([]int, 0)
		for i := 0; i < 5; i++ {
			n := card.Numbers[i][i]
			if !marked[n] {
				unmarked = append(unmarked, n)
			}
		}
		if len(unmarked) == 1 {
			completing = append(completing, unmarked[0])
		}
		unmarked = make([]int, 0)
		for i := 0; i < 5; i++ {
			n := card.Numbers[i][4-i]
			if !marked[n] {
				unmarked = append(unmarked, n)
			}
		}
		if len(unmarked) == 1 {
			completing = append(completing, unmarked[0])
		}
	}

	rows()
	cols()
	diags()

	seen := make(map[int]bool)
	uniq := make([]int, 0, len(completing))
	for _, n := range completing {
		if !seen[n] {
			seen[n] = true
			uniq = append(uniq, n)
		}
	}
	return uniq
}
