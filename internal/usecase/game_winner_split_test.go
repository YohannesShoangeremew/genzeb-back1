package usecase

import "testing"

func TestSelectMixedWinners(t *testing.T) {
	all := []winnerCard{{CardID: 10}, {CardID: 20}, {CardID: 30}}
	bots := []winnerCard{{CardID: 10}}

	whenDisabled := selectMixedWinners(all, bots, false)
	if len(whenDisabled) != 3 {
		t.Fatalf("bot_always_win=false returned %d winners, want all 3", len(whenDisabled))
	}
	for i := range all {
		if whenDisabled[i].CardID != all[i].CardID {
			t.Fatalf("winner %d card = %d, want %d", i, whenDisabled[i].CardID, all[i].CardID)
		}
	}

	whenEnabled := selectMixedWinners(all, bots, true)
	if len(whenEnabled) != 1 || whenEnabled[0].CardID != bots[0].CardID {
		t.Fatalf("bot_always_win=true returned %+v, want bot winners", whenEnabled)
	}
}
