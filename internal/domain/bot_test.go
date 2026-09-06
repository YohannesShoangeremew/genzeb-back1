package domain

import "testing"

func TestEffectiveBiasedDrawModeBackwardsCompatibility(t *testing.T) {
	tests := []struct {
		name string
		cfg  BotConfig
		want BiasedDrawMode
	}{
		{name: "explicit disabled", cfg: BotConfig{BiasedDrawMode: BiasedDrawModeDisabled, BotAlwaysWin: true}, want: BiasedDrawModeDisabled},
		{name: "explicit legacy", cfg: BotConfig{BiasedDrawMode: BiasedDrawModeLegacy}, want: BiasedDrawModeLegacy},
		{name: "explicit protected", cfg: BotConfig{BiasedDrawMode: BiasedDrawModeProtected}, want: BiasedDrawModeProtected},
		{name: "old enabled config", cfg: BotConfig{BotAlwaysWin: true}, want: BiasedDrawModeProtected},
		{name: "old disabled config", cfg: BotConfig{}, want: BiasedDrawModeDisabled},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.cfg.EffectiveBiasedDrawMode(); got != tt.want {
				t.Fatalf("EffectiveBiasedDrawMode() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNormalizeBiasedDrawModeSynchronizesLegacyBoolean(t *testing.T) {
	for _, mode := range []BiasedDrawMode{BiasedDrawModeDisabled, BiasedDrawModeLegacy, BiasedDrawModeProtected} {
		cfg := BotConfig{BiasedDrawMode: mode, BotAlwaysWin: mode == BiasedDrawModeDisabled}
		cfg.NormalizeBiasedDrawMode()
		if got, want := cfg.BotAlwaysWin, mode != BiasedDrawModeDisabled; got != want {
			t.Fatalf("mode %q normalized bot_always_win = %v, want %v", mode, got, want)
		}
	}
}

func TestBiasedDrawModeValidation(t *testing.T) {
	for _, mode := range []BiasedDrawMode{BiasedDrawModeDisabled, BiasedDrawModeLegacy, BiasedDrawModeProtected} {
		if !mode.IsValid() {
			t.Fatalf("expected %q to be valid", mode)
		}
	}
	if BiasedDrawMode("unknown").IsValid() {
		t.Fatal("unexpectedly accepted unknown biased draw mode")
	}
}
