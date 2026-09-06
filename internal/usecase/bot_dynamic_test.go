package usecase

import "testing"

func TestDesiredBotCount(t *testing.T) {
	tests := []struct {
		name               string
		realPlayers        int
		minimum            int
		hasSpendableBonus  bool
		savedPolicyEnabled bool
		want               int
	}{
		{name: "empty room fills to minimum", realPlayers: 0, minimum: 20, want: 20},
		{name: "three real players need seventeen bots", realPlayers: 3, minimum: 20, want: 17},
		{name: "below minimum uses exact gap", realPlayers: 19, minimum: 20, hasSpendableBonus: true, savedPolicyEnabled: true, want: 1},
		{name: "minimum reached without bonus needs no bots", realPlayers: 20, minimum: 20, savedPolicyEnabled: true, want: 0},
		{name: "minimum reached with bonus keeps two guards", realPlayers: 20, minimum: 20, hasSpendableBonus: true, savedPolicyEnabled: true, want: 2},
		{name: "above minimum with mixed funding keeps two guards", realPlayers: 22, minimum: 20, hasSpendableBonus: true, savedPolicyEnabled: true, want: 2},
		{name: "disabled saved policy does not retain guards", realPlayers: 22, minimum: 20, hasSpendableBonus: true, savedPolicyEnabled: false, want: 0},
		{name: "invalid zero minimum disables staffing", realPlayers: 0, minimum: 0, hasSpendableBonus: true, savedPolicyEnabled: true, want: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := desiredBotCount(tt.realPlayers, tt.minimum, tt.hasSpendableBonus, tt.savedPolicyEnabled); got != tt.want {
				t.Fatalf("desiredBotCount() = %d, want %d", got, tt.want)
			}
		})
	}
}
