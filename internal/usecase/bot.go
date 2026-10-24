// // // package usecase

// // // import (
// // // 	"context"
// // // 	"database/sql"
// // // 	"errors"
// // // 	"fmt"
// // // 	"math/rand"
// // // 	"sort"
// // // 	"time"

// // // 	"github.com/bingo/backend/internal/domain"
// // // 	redisGame "github.com/bingo/backend/pkg/redis"
// // // 	"github.com/bingo/backend/pkg/referral"
// // // 	utils "github.com/bingo/backend/pkg/utils"
// // // 	"github.com/google/uuid"
// // // )

// // // // botTelegramIDBase is the offset for synthetic bot Telegram IDs. Real Telegram
// // // // IDs are positive, so large negative IDs can never collide with a real user.
// // // const (
// // // 	botTelegramIDBase           = -1_000_000_000
// // // 	highPopulationGuardBotCount = 2
// // // 	maxCardsPerBot              = 2 // Allow each bot to hold up to 5 cards per game room
// // // )

// // // // botDisplayNames are Telegram-style display names used for house-controlled
// // // // filler bots, matching your clean list.
// // // var botDisplayNames = []string{
// // // 	"Natiye", "beki", "B", "bekaaaaaaaaa", "Solitude",
// // // 	"Godsproperty", "maki", "Beka", "Kiya", "Babi",
// // // 	"Yadi", "TemesgenYa", "N", "natiiii", "Eclipse",
// // // 	"YaAllah", "Moges", "Micky", "Mack", "Abela",
// // // 	"Fikir", "yabs", "Elsa", "edu", "Peace",
// // // 	"His", "B", "Abet", "Noah", "Kikiyeee",
// // // 	"Abi", "makgreat", "J", "hanicho", "Chaos",
// // // 	"Grace", "Ghosttt", "Tew", "Beka", "Mimi",
// // // 	"habib", "eduta", "K", "yabsss", "Loner",
// // // 	"Amen", "Edu", "Min", "Sami", "Nani",
// // // 	"Mela", "hani", "D", "sammyy", "Vibes",
// // // 	"Chosen", "Kira", "Awo", "Nati", "Tuti",
// // // 	"Tseda", "sami", "M", "makkk", "Aura",
// // // 	"Gospel", "yabs", "Ishi", "Hani", "Jiji",
// // // 	"Beki", "beti", "H", "abiii", "Moonchild",
// // // 	"Jesus", "miki", "Kiya", "Edu", "Fifi",
// // // 	"Robi", "chala", "Y", "fikirrr", "Soul",
// // // 	"Faith", "hani", "Ere", "Yoni", "Chuchu",
// // // 	"Daveeeeee", "muni", "elias", "lilii", "Broken",
// // // 	"Marys", "beti", "Gida", "Dani", "Didi",
// // // 	"Yoni", "nami", "A", "danii", "Faded",
// // // 	"Alhamdulillah", "jo", "Ebak", "Fikir", "Bobo",
// // // 	"Kaleb", "rozi", "T", "robb", "Ghost",
// // // 	"Sabr", "solo", "Tik", "Selam", "Mamo",
// // // 	"Fema", "meron", "R", "jerryy", "Silence",
// // // 	"Tawakkul", "ezi", "Fen", "Tsega", "Jojo",
// // // 	"ezra", "yerus", "L", "tutuu", "Empty",
// // // 	"Blessed", "dan", "Lemen", "Geta", "Nono",
// // // 	"Bruk", "mina", "P", "eziii", "Void",
// // // 	"Redeemed", "rob", "Man", "Rahmet", "Pipi",
// // // 	"Safi", "tsed", "C", "fiooo", "Nothing",
// // // 	"Saved", "fik", "Hul", "Juma", "Toto",
// // // 	"Elu", "yofe", "V", "soll", "Shadow",
// // // 	"Holy", "lid", "Chil", "Hiwot", "Lolo",
// // // 	"Jeri", "abi", "Z", "babb", "Bliss",
// // // 	"Pray", "na", "Wey", "Tesfa", "Yoyo",
// // // 	"Redi", "meze", "G", "garii", "Serenity",
// // // 	"Lord", "kal", "Ged", "Birhan", "Gogo",
// // // 	"Bogi", "zed", "F", "noahh", "Lost",
// // // 	"Christ", "bru", "Zim", "Kal", "Shushu",
// // // 	"Miki", "jappy", "T", "sidd", "Tired",
// // // 	"Saint", "sel", "Besm", "Emnet", "Kuku",
// // // 	"Seli", "lela", "W", "yemii", "Enigma",
// // // 	"Peace", "mel", "Kef", "Tsehay", "Maca",
// // // 	"Lina", "eba", "Q", "farr", "Abyss",
// // // 	"Mercy", "Fitse", "Are", "Chereka", "Papa",
// // // 	"Miky", "tofik", "X", "AB", "Mirage",
// // // 	"Heaven", "Abenezer", "Dagmawi", "Kiyaye", "Daniel",
// // // 	"dagi", "jossi", "Kirubel", "kiraa", "Echo",
// // // 	"Angel", "Father", "Ende", "Semay", "Coco",
// // // 	"Fasilo", "Yichalal", "Demelash", "lderu", "Chill",
// // // 	"Prophet", "Dereje", "Dere", "Midir", "Zizu",
// // // 	"Hena", "hiwi", "User", "titi", "Dawn",
// // // 	"Nour", "Bomb", "Tel", "Zimta", "Ririfikir",
// // // 	"Amu", "mack", "U", "Timiro", "Dusk",
// // // 	"Deen", "Bella", "ErmiyasAsa", "Ewnet", "Davinchi",
// // // 	"YonasG", "yordi", "K", "mariiii", "Luna",
// // // 	"Glory", "Goytom", "Abrish", "Sewmalet", "Meseret",
// // // 	"Abdulselam", "Abdi", "Kedir", "babyyyy", "Starboy",
// // // 	"MyLove", "Yabsera", "Faya", "Nuro", "ElaBest",
// // // }

// // // // BotSettings holds the operator-tunable knobs supplied from config/env.
// // // type BotSettings struct {
// // // 	PoolSize        int           // how many bot accounts to seed
// // // 	WalletFloat     float64       // balance each bot wallet is topped up to
// // // 	MaxJoinsPerTick int           // bots added per game per sweep (spaces out joins)
// // // 	JoinDelay       time.Duration // hold bots back this long after the first real player joins
// // // 	CheckInterval   time.Duration // how often the auto-filler sweeps
// // // 	WinRate         float64       // probability (0-1) that bots win when they have a valid bingo alongside humans
// // // }

// // // // BotUseCase seeds a pool of house-owned filler bots and joins them into games
// // // type BotUseCase struct {
// // // 	botRepo         domain.BotRepository
// // // 	userRepo        domain.UserRepository
// // // 	walletRepo      domain.WalletRepository
// // // 	transactionRepo domain.TransactionRepository
// // // 	gameRepo        domain.GameRepository
// // // 	gameUC          *GameUseCase
// // // 	gameState       *redisGame.GameStateService
// // // 	redisService    *redisGame.GameStateService
// // // 	db              *sql.DB
// // // 	settings        BotSettings
// // // }

// // // // NewBotUseCase wires the bot use case.
// // // func NewBotUseCase(
// // // 	botRepo domain.BotRepository,
// // // 	userRepo domain.UserRepository,
// // // 	walletRepo domain.WalletRepository,
// // // 	transactionRepo domain.TransactionRepository,
// // // 	gameRepo domain.GameRepository,
// // // 	gameUC *GameUseCase,
// // // 	gameState *redisGame.GameStateService,
// // // 	db *sql.DB,
// // // 	settings BotSettings,
// // // ) *BotUseCase {
// // // 	return &BotUseCase{
// // // 		botRepo:         botRepo,
// // // 		userRepo:        userRepo,
// // // 		walletRepo:      walletRepo,
// // // 		transactionRepo: transactionRepo,
// // // 		gameRepo:        gameRepo,
// // // 		gameUC:          gameUC,
// // // 		gameState:       gameState,
// // // 		redisService:    gameState,
// // // 		db:              db,
// // // 		settings:        settings,
// // // 	}
// // // }

// // // // ---- Config passthrough (admin dashboard) ----

// // // func (uc *BotUseCase) GetConfig(ctx context.Context) (*domain.BotConfig, error) {
// // // 	return uc.botRepo.GetConfig(ctx)
// // // }

// // // func (uc *BotUseCase) UpdateConfig(ctx context.Context, req domain.UpdateBotConfigRequest) (*domain.BotConfig, error) {
// // // 	cfg, err := uc.botRepo.GetConfig(ctx)
// // // 	if err != nil {
// // // 		return nil, err
// // // 	}
// // // 	if req.Enabled != nil {
// // // 		cfg.Enabled = *req.Enabled
// // // 	}
// // // 	if req.MinRealPlayers != nil {
// // // 		if *req.MinRealPlayers < 0 {
// // // 			return nil, fmt.Errorf("min_real_players cannot be negative")
// // // 		}
// // // 		cfg.MinRealPlayers = *req.MinRealPlayers
// // // 	}
// // // 	if req.TargetBots != nil {
// // // 		if *req.TargetBots < 0 {
// // // 			return nil, fmt.Errorf("target_bots cannot be negative")
// // // 		}
// // // 		cfg.TargetBots = *req.TargetBots
// // // 	}
// // // 	if req.MinimumRoomPlayers != nil {
// // // 		if *req.MinimumRoomPlayers < domain.MinPlayers || *req.MinimumRoomPlayers > domain.MaxCardID {
// // // 			return nil, fmt.Errorf("minimum_room_players must be between %d and %d", domain.MinPlayers, domain.MaxCardID)
// // // 		}
// // // 		cfg.MinimumRoomPlayers = *req.MinimumRoomPlayers
// // // 	}
// // // 	if req.Tiers != nil {
// // // 		cfg.Tiers = *req.Tiers
// // // 	}
// // // 	if req.WinRate != nil {
// // // 		if *req.WinRate < 0 || *req.WinRate > 1 {
// // // 			return nil, fmt.Errorf("win_rate must be between 0 and 1")
// // // 		}
// // // 		cfg.WinRate = *req.WinRate
// // // 	}
// // // 	if req.BiasedDrawMode != nil {
// // // 		if !req.BiasedDrawMode.IsValid() {
// // // 			return nil, fmt.Errorf("biased_draw_mode must be disabled, legacy, or protected")
// // // 		}
// // // 		cfg.BiasedDrawMode = *req.BiasedDrawMode
// // // 		cfg.BotAlwaysWin = cfg.BiasedDrawMode != domain.BiasedDrawModeDisabled
// // // 	} else if req.BotAlwaysWin != nil {
// // // 		cfg.BotAlwaysWin = *req.BotAlwaysWin
// // // 		if cfg.BotAlwaysWin {
// // // 			cfg.BiasedDrawMode = domain.BiasedDrawModeProtected
// // // 		} else {
// // // 			cfg.BiasedDrawMode = domain.BiasedDrawModeDisabled
// // // 		}
// // // 	}
// // // 	if err := uc.botRepo.UpdateConfig(ctx, cfg); err != nil {
// // // 		return nil, err
// // // 	}
// // // 	return cfg, nil
// // // }

// // // // ---- Seeding ----

// // // func (uc *BotUseCase) SeedPool(ctx context.Context, count int) error {
// // // 	if count <= 0 {
// // // 		count = uc.settings.PoolSize
// // // 	}
// // // 	return uc.EnsureBotPool(ctx, count)
// // // }

// // // func (uc *BotUseCase) EnsureBotPool(ctx context.Context, size int) error {
// // // 	for i := 1; i <= size; i++ {
// // // 		telegramID := int64(botTelegramIDBase - i)

// // // 		existing, err := uc.userRepo.FindByTelegramID(ctx, telegramID)
// // // 		if err == nil && existing != nil {
// // // 			if ferr := uc.fundBotWallet(ctx, existing.ID); ferr != nil {
// // // 				return fmt.Errorf("fund existing bot %d: %w", i, ferr)
// // // 			}
// // // 			continue
// // // 		}

// // // 		if err := uc.createBot(ctx, i, telegramID); err != nil {
// // // 			return fmt.Errorf("create bot %d: %w", i, err)
// // // 		}
// // // 	}
// // // 	return nil
// // // }

// // // func (uc *BotUseCase) createBot(ctx context.Context, index int, telegramID int64) error {
// // // 	code, err := uc.uniqueReferralCode(ctx)
// // // 	if err != nil {
// // // 		return err
// // // 	}

// // // 	name := botDisplayNames[(index-1)%len(botDisplayNames)]
// // // 	phone := fmt.Sprintf("BOT-%08d", index)

// // // 	tx, err := uc.db.BeginTx(ctx, nil)
// // // 	if err != nil {
// // // 		return err
// // // 	}
// // // 	defer tx.Rollback()

// // // 	user := &domain.User{
// // // 		ID:          uuid.New(),
// // // 		TelegramID:  telegramID,
// // // 		FirstName:   name,
// // // 		PhoneNumber: phone,
// // // 		ReferalCode: code,
// // // 		Role:        "user",
// // // 		IsBot:       true,
// // // 	}
// // // 	if err := uc.userRepo.Create(ctx, tx, user); err != nil {
// // // 		return err
// // // 	}

// // // 	wallet := &domain.Wallet{UserID: user.ID, Balance: uc.settings.WalletFloat}
// // // 	if err := uc.walletRepo.Create(ctx, tx, wallet); err != nil {
// // // 		return err
// // // 	}

// // // 	if err := uc.recordBotFunding(ctx, tx, user.ID, uc.settings.WalletFloat); err != nil {
// // // 		return err
// // // 	}

// // // 	return tx.Commit()
// // // }

// // // func (uc *BotUseCase) uniqueReferralCode(ctx context.Context) (string, error) {
// // // 	for i := 0; i < domain.MaxReferralCodeGenerationAttempts; i++ {
// // // 		code, err := referral.GenerateReferralCode()
// // // 		if err != nil {
// // // 			return "", fmt.Errorf("failed to generate referral code: %w", err)
// // // 		}
// // // 		if _, err := uc.userRepo.FindByReferralCode(ctx, code); err != nil {
// // // 			return code, nil
// // // 		}
// // // 	}
// // // 	return "", fmt.Errorf("failed to generate unique referral code")
// // // }

// // // func (uc *BotUseCase) fundBotWallet(ctx context.Context, userID uuid.UUID) error {
// // // 	tx, err := uc.db.BeginTx(ctx, nil)
// // // 	if err != nil {
// // // 		return err
// // // 	}
// // // 	defer tx.Rollback()

// // // 	wallet, err := uc.walletRepo.LockForUpdate(ctx, tx, userID)
// // // 	if err != nil {
// // // 		return err
// // // 	}
// // // 	topUp := uc.settings.WalletFloat - wallet.Balance
// // // 	if topUp <= 0 {
// // // 		return tx.Commit()
// // // 	}
// // // 	if err := uc.walletRepo.UpdateBalance(ctx, tx, userID, topUp); err != nil {
// // // 		return err
// // // 	}
// // // 	if err := uc.recordBotFunding(ctx, tx, userID, topUp); err != nil {
// // // 		return err
// // // 	}
// // // 	return tx.Commit()
// // // }

// // // func (uc *BotUseCase) recordBotFunding(ctx context.Context, tx *sql.Tx, userID uuid.UUID, amount float64) error {
// // // 	ref := "BOT_FUNDING"
// // // 	return uc.transactionRepo.Create(ctx, tx, &domain.Transaction{
// // // 		UserID:    userID,
// // // 		Type:      domain.TransactionTypeDeposit,
// // // 		Category:  domain.TransactionCategoryBotFunding,
// // // 		Amount:    amount,
// // // 		Status:    domain.TransactionStatusCompleted,
// // // 		Reference: &ref,
// // // 	})
// // // }

// // // // ---- Filling (Optimized Bulk Join with Admin Control & Pacing) ----

// // // func (uc *BotUseCase) FillGame(ctx context.Context, gameID uuid.UUID, requested int) (*domain.BotFillResult, error) {
// // // 	// Manual admin fill: bypass MaxJoinsPerTick throttling so admin requests fulfill immediately
// // // 	return uc.fill(ctx, gameID, requested, false, false)
// // // }

// // // func (uc *BotUseCase) fill(ctx context.Context, gameID uuid.UUID, requested int, allowEmpty bool, applyPacing bool) (*domain.BotFillResult, error) {
// // // 	game, err := uc.gameRepo.FindByID(ctx, gameID)
// // // 	if err != nil {
// // // 		return nil, fmt.Errorf("game not found: %w", err)
// // // 	}
// // // 	if game.State != domain.GameStateWaiting && game.State != domain.GameStateCountdown {
// // // 		return nil, fmt.Errorf("game is not accepting new players")
// // // 	}

// // // 	realPlayers, err := uc.botRepo.CountRealPlayersInGame(ctx, gameID)
// // // 	if err != nil {
// // // 		return nil, err
// // // 	}
// // // 	botPlayers, err := uc.botRepo.CountBotsInGame(ctx, gameID)
// // // 	if err != nil {
// // // 		return nil, err
// // // 	}

// // // 	result := &domain.BotFillResult{GameID: gameID, Requested: requested, RealPlayers: realPlayers, BotPlayers: botPlayers}

// // // 	if !allowEmpty && realPlayers < 1 {
// // // 		return result, nil
// // // 	}

// // // 	botCardCounts, err := uc.botCardCountsInGame(ctx, gameID)
// // // 	if err != nil {
// // // 		return nil, err
// // // 	}
// // // 	freeCards, err := uc.freeCards(ctx, gameID)
// // // 	if err != nil {
// // // 		return nil, err
// // // 	}

// // // 	bots, err := uc.botRepo.ListBots(ctx, uc.settings.PoolSize*2+requested+10)
// // // 	if err != nil {
// // // 		return nil, err
// // // 	}

// // // 	type cardAssignment struct {
// // // 		userID uuid.UUID
// // // 		cardID int
// // // 	}
// // // 	var assignments []cardAssignment

// // // 	added := 0
// // // 	for _, bot := range bots {
// // // 		if added >= requested || len(freeCards) == 0 {
// // // 			break
// // // 		}

// // // 		for botCardCounts[bot.ID] < maxCardsPerBot && added < requested && len(freeCards) > 0 {
// // // 			if err := uc.ensureAffordable(ctx, bot.ID, game.BetAmount); err != nil {
// // // 				break
// // // 			}

// // // 			cardID := freeCards[len(freeCards)-1]
// // // 			freeCards = freeCards[:len(freeCards)-1]

// // // 			assignments = append(assignments, cardAssignment{userID: bot.ID, cardID: cardID})
// // // 			botCardCounts[bot.ID]++
// // // 			added++
// // // 		}
// // // 	}

// // // 	if len(assignments) == 0 {
// // // 		result.Added = 0
// // // 		result.BotPlayers = botPlayers
// // // 		return result, nil
// // // 	}

// // // 	// Apply MaxJoinsPerTick pacing ONLY when requested by the automatic background sweeper
// // // 	if applyPacing && uc.settings.MaxJoinsPerTick > 0 && len(assignments) > uc.settings.MaxJoinsPerTick {
// // // 		assignments = assignments[:uc.settings.MaxJoinsPerTick]
// // // 	}

// // // 	// ---- BULK TRANSACTION: Insert this batch atomically ----
// // // 	tx, err := uc.db.BeginTx(ctx, nil)
// // // 	if err != nil {
// // // 		return nil, fmt.Errorf("failed to begin bulk join tx: %w", err)
// // // 	}
// // // 	defer tx.Rollback()

// // // 	game, err = uc.gameRepo.LockForUpdate(ctx, tx, gameID)
// // // 	if err != nil {
// // // 		return nil, fmt.Errorf("failed to lock game for bulk join: %w", err)
// // // 	}
// // // 	if game.State != domain.GameStateWaiting && game.State != domain.GameStateCountdown {
// // // 		return nil, errors.New("game is no longer accepting new players")
// // // 	}

// // // 	prevPlayerCount := game.PlayerCount

// // // 	for _, asm := range assignments {
// // // 		player := &domain.GamePlayer{
// // // 			ID:           uuid.New(),
// // // 			GameID:       gameID,
// // // 			UserID:       asm.userID,
// // // 			CardID:       asm.cardID,
// // // 			Paid:         false,
// // // 			IsEliminated: false,
// // // 		}
// // // 		if err := uc.gameRepo.AddPlayer(ctx, tx, player); err != nil {
// // // 			continue
// // // 		}
// // // 	}

// // // 	active, err := uc.gameRepo.GetActivePlayersTx(ctx, tx, gameID)
// // // 	if err != nil {
// // // 		return nil, fmt.Errorf("failed to read active players: %w", err)
// // // 	}
// // // 	distinct := make(map[uuid.UUID]bool, len(active))
// // // 	for _, p := range active {
// // // 		distinct[p.UserID] = true
// // // 	}
// // // 	game.PlayerCount = len(distinct)
// // // 	game.PrizePool = float64(len(active)) * game.BetAmount * (1 - game.HouseCut)

// // // 	if err := uc.gameRepo.UpdateTx(ctx, tx, game); err != nil {
// // // 		return nil, fmt.Errorf("failed to update game counters: %w", err)
// // // 	}

// // // 	if err := tx.Commit(); err != nil {
// // // 		return nil, fmt.Errorf("failed to commit bulk join tx: %w", err)
// // // 	}

// // // 	for _, asm := range assignments {
// // // 		uc.redisService.AddPlayer(ctx, gameID, asm.userID)
// // // 		uc.redisService.AddTakenCard(ctx, gameID, asm.cardID)
// // // 	}
// // // 	uc.redisService.SaveGameState(ctx, game)

// // // 	// STANDARD COUNTDOWN TRIGGER: Start countdown as soon as the room hits MinPlayers (2 distinct players/bots)
// // // 	if game.State == domain.GameStateWaiting && prevPlayerCount < domain.MinPlayers && game.PlayerCount >= domain.MinPlayers {
// // // 		utils.GoSafe("startCountdown", func() { uc.gameUC.startCountdown(context.Background(), gameID) })
// // // 	}

// // // 	uc.redisService.PublishEvent(ctx, gameID, domain.WebSocketEventPlayerJoined, map[string]interface{}{
// // // 		"prize_pool":   game.PrizePool,
// // // 		"player_count": game.PlayerCount,
// // // 	})

// // // 	result.Added = len(assignments)
// // // 	result.BotPlayers = botPlayers + len(assignments)
// // // 	return result, nil
// // // }

// // // func (uc *BotUseCase) ensureAffordable(ctx context.Context, userID uuid.UUID, bet float64) error {
// // // 	wallet, err := uc.walletRepo.FindByUserID(ctx, userID)
// // // 	if err == nil && wallet.Balance >= bet {
// // // 		return nil
// // // 	}
// // // 	return uc.fundBotWallet(ctx, userID)
// // // }

// // // func (uc *BotUseCase) botCardCountsInGame(ctx context.Context, gameID uuid.UUID) (map[uuid.UUID]int, error) {
// // // 	players, err := uc.gameRepo.GetPlayers(ctx, gameID)
// // // 	if err != nil {
// // // 		return nil, err
// // // 	}
// // // 	counts := make(map[uuid.UUID]int)
// // // 	for _, p := range players {
// // // 		if p != nil && p.IsBot && !p.IsEliminated && p.LeftAt == nil {
// // // 			counts[p.UserID]++
// // // 		}
// // // 	}
// // // 	return counts, nil
// // // }

// // // func (uc *BotUseCase) freeCards(ctx context.Context, gameID uuid.UUID) ([]int, error) {
// // // 	taken, err := uc.gameRepo.GetTakenCards(ctx, gameID)
// // // 	if err != nil {
// // // 		return nil, err
// // // 	}
// // // 	takenSet := make(map[int]bool, len(taken))
// // // 	for _, c := range taken {
// // // 		takenSet[c] = true
// // // 	}
// // // 	free := make([]int, 0, domain.MaxCardID)
// // // 	for c := domain.MinCardID; c <= domain.MaxCardID; c++ {
// // // 		if !takenSet[c] {
// // // 			free = append(free, c)
// // // 		}
// // // 	}
// // // 	rand.Shuffle(len(free), func(i, j int) { free[i], free[j] = free[j], free[i] })
// // // 	return free, nil
// // // }

// // // // ---- Auto-fill sweeper ----

// // // func (uc *BotUseCase) joinDelayFor(gameID uuid.UUID) time.Duration {
// // // 	base := uc.settings.JoinDelay
// // // 	if base <= 0 {
// // // 		return 0
// // // 	}
// // // 	spread := float64(gameID[0]) / 256.0
// // // 	return base + time.Duration(float64(base)*0.5*spread)
// // // }

// // // func desiredBotCount(realPlayers, minimumRoomPlayers int, hasSpendableBonus, savedPolicyEnabled bool) int {
// // // 	if minimumRoomPlayers <= 0 {
// // // 		return 0
// // // 	}
// // // 	if realPlayers < minimumRoomPlayers {
// // // 		return minimumRoomPlayers - realPlayers
// // // 	}
// // // 	if hasSpendableBonus && savedPolicyEnabled {
// // // 		return highPopulationGuardBotCount
// // // 	}
// // // 	return 0
// // // }

// // // func (uc *BotUseCase) trimBots(ctx context.Context, gameID uuid.UUID, requested int) int {
// // // 	if requested <= 0 {
// // // 		return 0
// // // 	}
// // // 	players, err := uc.gameRepo.GetPlayers(ctx, gameID)
// // // 	if err != nil {
// // // 		return 0
// // // 	}

// // // 	latestByBot := make(map[uuid.UUID]*domain.GamePlayer)
// // // 	for _, p := range players {
// // // 		if p == nil || !p.IsBot || p.IsEliminated || p.LeftAt != nil {
// // // 			continue
// // // 		}
// // // 		current := latestByBot[p.UserID]
// // // 		if current == nil || p.JoinedAt.After(current.JoinedAt) {
// // // 			latestByBot[p.UserID] = p
// // // 		}
// // // 	}

// // // 	bots := make([]*domain.GamePlayer, 0, len(latestByBot))
// // // 	for _, p := range latestByBot {
// // // 		bots = append(bots, p)
// // // 	}
// // // 	sort.Slice(bots, func(i, j int) bool {
// // // 		if bots[i].JoinedAt.Equal(bots[j].JoinedAt) {
// // // 			return bots[i].UserID.String() > bots[j].UserID.String()
// // // 		}
// // // 		return bots[i].JoinedAt.After(bots[j].JoinedAt)
// // // 	})

// // // 	removed := 0
// // // 	for _, bot := range bots {
// // // 		if removed >= requested || ctx.Err() != nil {
// // // 			break
// // // 		}
// // // 		if err := uc.gameUC.LeaveGame(ctx, gameID, domain.LeaveGameRequest{UserID: bot.UserID}); err == nil {
// // // 			removed++
// // // 		}
// // // 	}
// // // 	return removed
// // // }

// // // func (uc *BotUseCase) Run(ctx context.Context) {
// // // 	ticker := time.NewTicker(uc.settings.CheckInterval)
// // // 	defer ticker.Stop()

// // // 	for {
// // // 		select {
// // // 		case <-ctx.Done():
// // // 			return
// // // 		case <-ticker.C:
// // // 			uc.sweep(ctx)
// // // 		}
// // // 	}
// // // }

// // // func (uc *BotUseCase) sweep(ctx context.Context) {
// // // 	cfg, err := uc.botRepo.GetConfig(ctx)
// // // 	if err != nil || !cfg.Enabled || cfg.MinimumRoomPlayers < domain.MinPlayers {
// // // 		return
// // // 	}

// // // 	savedPolicyEnabled := cfg.EffectiveBiasedDrawMode() != domain.BiasedDrawModeDisabled
// // // 	for _, tier := range cfg.TierList() {
// // // 		t := tier
// // // 		games, err := uc.gameRepo.FindAvailable(ctx, &t, domain.MaxAvailableGamesLimit)
// // // 		if err != nil {
// // // 			continue
// // // 		}
// // // 		for _, game := range games {
// // // 			realPlayers, err := uc.botRepo.CountRealPlayersInGame(ctx, game.ID)
// // // 			if err != nil {
// // // 				continue
// // // 			}

// // // 			hasSpendableBonus := false
// // // 			if realPlayers >= cfg.MinimumRoomPlayers && savedPolicyEnabled {
// // // 				hasSpendableBonus, err = uc.botRepo.HasSpendableBonusPlayerInGame(ctx, game.ID, game.BetAmount)
// // // 				if err != nil {
// // // 					continue
// // // 				}
// // // 			}

// // // 			desired := desiredBotCount(realPlayers, cfg.MinimumRoomPlayers, hasSpendableBonus, savedPolicyEnabled)
// // // 			botPlayers, err := uc.botRepo.CountBotsInGame(ctx, game.ID)
// // // 			if err != nil {
// // // 				continue
// // // 			}

// // // 			if botPlayers > desired {
// // // 				uc.trimBots(ctx, game.ID, botPlayers-desired)
// // // 				continue
// // // 			}
// // // 			if botPlayers == desired {
// // // 				continue
// // // 			}

// // // 			if realPlayers > 0 {
// // // 				if delay := uc.joinDelayFor(game.ID); delay > 0 {
// // // 					age, hasReal, err := uc.botRepo.SecondsSinceFirstRealPlayer(ctx, game.ID)
// // // 					if err != nil || !hasReal || time.Duration(age*float64(time.Second)) < delay {
// // // 						continue
// // // 					}
// // // 				}
// // // 			}

// // // 			need := desired - botPlayers
// // // 			// Sweeper fill applies pacing
// // // 			if _, err := uc.fill(ctx, game.ID, need, realPlayers == 0, true); err != nil {
// // // 				continue
// // // 			}
// // // 		}
// // // 	}
// // // }

// // package usecase

// // import (
// // 	"context"
// // 	"database/sql"
// // 	"errors"
// // 	"fmt"
// // 	"math/rand"
// // 	"sort"
// // 	"time"

// // 	"github.com/bingo/backend/internal/domain"
// // 	redisGame "github.com/bingo/backend/pkg/redis"
// // 	"github.com/bingo/backend/pkg/referral"
// // 	utils "github.com/bingo/backend/pkg/utils"
// // 	"github.com/google/uuid"
// // )

// // // botTelegramIDBase is the offset for synthetic bot Telegram IDs. Real Telegram
// // // IDs are positive, so large negative IDs can never collide with a real user.
// // const (
// // 	botTelegramIDBase           = -1_000_000_000
// // 	highPopulationGuardBotCount = 2
// // 	maxCardsPerBot              = 2 // Allow each bot to hold up to 2 cards per game room
// // )

// // // botDisplayNames are Telegram-style display names used for house-controlled
// // // filler bots, matching your clean list.
// // var botDisplayNames = []string{
// // 	"Natiye", "beki", "B", "bekaaaaaaaaa", "Solitude",
// // 	"Godsproperty", "maki", "Beka", "Kiya", "Babi",
// // 	"Yadi", "TemesgenYa", "N", "natiiii", "Eclipse",
// // 	"YaAllah", "Moges", "Micky", "Mack", "Abela",
// // 	"Fikir", "yabs", "Elsa", "edu", "Peace",
// // 	"His", "B", "Abet", "Noah", "Kikiyeee",
// // 	"Abi", "makgreat", "J", "hanicho", "Chaos",
// // 	"Grace", "Ghosttt", "Tew", "Beka", "Mimi",
// // 	"habib", "eduta", "K", "yabsss", "Loner",
// // 	"Amen", "Edu", "Min", "Sami", "Nani",
// // 	"Mela", "hani", "D", "sammyy", "Vibes",
// // 	"Chosen", "Kira", "Awo", "Nati", "Tuti",
// // 	"Tseda", "sami", "M", "makkk", "Aura",
// // 	"Gospel", "yabs", "Ishi", "Hani", "Jiji",
// // 	"Beki", "beti", "H", "abiii", "Moonchild",
// // 	"Jesus", "miki", "Kiya", "Edu", "Fifi",
// // 	"Robi", "chala", "Y", "fikirrr", "Soul",
// // 	"Faith", "hani", "Ere", "Yoni", "Chuchu",
// // 	"Daveeeeee", "muni", "elias", "lilii", "Broken",
// // 	"Marys", "beti", "Gida", "Dani", "Didi",
// // 	"Yoni", "nami", "A", "danii", "Faded",
// // 	"Alhamdulillah", "jo", "Ebak", "Fikir", "Bobo",
// // 	"Kaleb", "rozi", "T", "robb", "Ghost",
// // 	"Sabr", "solo", "Tik", "Selam", "Mamo",
// // 	"Fema", "meron", "R", "jerryy", "Silence",
// // 	"Tawakkul", "ezi", "Fen", "Tsega", "Jojo",
// // 	"ezra", "yerus", "L", "tutuu", "Empty",
// // 	"Blessed", "dan", "Lemen", "Geta", "Nono",
// // 	"Bruk", "mina", "P", "eziii", "Void",
// // 	"Redeemed", "rob", "Man", "Rahmet", "Pipi",
// // 	"Safi", "tsed", "C", "fiooo", "Nothing",
// // 	"Saved", "fik", "Hul", "Juma", "Toto",
// // 	"Elu", "yofe", "V", "soll", "Shadow",
// // 	"Holy", "lid", "Chil", "Hiwot", "Lolo",
// // 	"Jeri", "abi", "Z", "babb", "Bliss",
// // 	"Pray", "na", "Wey", "Tesfa", "Yoyo",
// // 	"Redi", "meze", "G", "garii", "Serenity",
// // 	"Lord", "kal", "Ged", "Birhan", "Gogo",
// // 	"Bogi", "zed", "F", "noahh", "Lost",
// // 	"Christ", "bru", "Zim", "Kal", "Shushu",
// // 	"Miki", "jappy", "T", "sidd", "Tired",
// // 	"Saint", "sel", "Besm", "Emnet", "Kuku",
// // 	"Seli", "lela", "W", "yemii", "Enigma",
// // 	"Peace", "mel", "Kef", "Tsehay", "Maca",
// // 	"Lina", "eba", "Q", "farr", "Abyss",
// // 	"Mercy", "Fitse", "Are", "Chereka", "Papa",
// // 	"Miky", "tofik", "X", "AB", "Mirage",
// // 	"Heaven", "Abenezer", "Dagmawi", "Kiyaye", "Daniel",
// // 	"dagi", "jossi", "Kirubel", "kiraa", "Echo",
// // 	"Angel", "Father", "Ende", "Semay", "Coco",
// // 	"Fasilo", "Yichalal", "Demelash", "lderu", "Chill",
// // 	"Prophet", "Dereje", "Dere", "Midir", "Zizu",
// // 	"Hena", "hiwi", "User", "titi", "Dawn",
// // 	"Nour", "Bomb", "Tel", "Zimta", "Ririfikir",
// // 	"Amu", "mack", "U", "Timiro", "Dusk",
// // 	"Deen", "Bella", "ErmiyasAsa", "Ewnet", "Davinchi",
// // 	"YonasG", "yordi", "K", "mariiii", "Luna",
// // 	"Glory", "Goytom", "Abrish", "Sewmalet", "Meseret",
// // 	"Abdulselam", "Abdi", "Kedir", "babyyyy", "Starboy",
// // 	"MyLove", "Yabsera", "Faya", "Nuro", "ElaBest",
// // }

// // // BotSettings holds the operator-tunable knobs supplied from config/env.
// // type BotSettings struct {
// // 	PoolSize        int           // how many bot accounts to seed
// // 	WalletFloat     float64       // balance each bot wallet is topped up to
// // 	MaxJoinsPerTick int           // bots added per game per sweep (spaces out joins)
// // 	JoinDelay       time.Duration // hold bots back this long after the first real player joins
// // 	CheckInterval   time.Duration // how often the auto-filler sweeps
// // 	WinRate         float64       // probability (0-1) that bots win when they have a valid bingo alongside humans
// // }

// // // BotUseCase seeds a pool of house-owned filler bots and joins them into games
// // type BotUseCase struct {
// // 	botRepo         domain.BotRepository
// // 	userRepo        domain.UserRepository
// // 	walletRepo      domain.WalletRepository
// // 	transactionRepo domain.TransactionRepository
// // 	gameRepo        domain.GameRepository
// // 	gameUC          *GameUseCase
// // 	gameState       *redisGame.GameStateService
// // 	redisService    *redisGame.GameStateService
// // 	db              *sql.DB
// // 	settings        BotSettings
// // }

// // // NewBotUseCase wires the bot use case.
// // func NewBotUseCase(
// // 	botRepo domain.BotRepository,
// // 	userRepo domain.UserRepository,
// // 	walletRepo domain.WalletRepository,
// // 	transactionRepo domain.TransactionRepository,
// // 	gameRepo domain.GameRepository,
// // 	gameUC *GameUseCase,
// // 	gameState *redisGame.GameStateService,
// // 	db *sql.DB,
// // 	settings BotSettings,
// // ) *BotUseCase {
// // 	return &BotUseCase{
// // 		botRepo:         botRepo,
// // 		userRepo:        userRepo,
// // 		walletRepo:      walletRepo,
// // 		transactionRepo: transactionRepo,
// // 		gameRepo:        gameRepo,
// // 		gameUC:          gameUC,
// // 		gameState:       gameState,
// // 		redisService:    gameState,
// // 		db:              db,
// // 		settings:        settings,
// // 	}
// // }

// // // ---- Config passthrough (admin dashboard) ----

// // func (uc *BotUseCase) GetConfig(ctx context.Context) (*domain.BotConfig, error) {
// // 	return uc.botRepo.GetConfig(ctx)
// // }

// // func (uc *BotUseCase) UpdateConfig(ctx context.Context, req domain.UpdateBotConfigRequest) (*domain.BotConfig, error) {
// // 	cfg, err := uc.botRepo.GetConfig(ctx)
// // 	if err != nil {
// // 		return nil, err
// // 	}
// // 	if req.Enabled != nil {
// // 		cfg.Enabled = *req.Enabled
// // 	}
// // 	if req.MinRealPlayers != nil {
// // 		if *req.MinRealPlayers < 0 {
// // 			return nil, fmt.Errorf("min_real_players cannot be negative")
// // 		}
// // 		cfg.MinRealPlayers = *req.MinRealPlayers
// // 	}
// // 	if req.TargetBots != nil {
// // 		if *req.TargetBots < 0 {
// // 			return nil, fmt.Errorf("target_bots cannot be negative")
// // 		}
// // 		cfg.TargetBots = *req.TargetBots
// // 	}
// // 	if req.MinimumRoomPlayers != nil {
// // 		if *req.MinimumRoomPlayers < domain.MinPlayers || *req.MinimumRoomPlayers > domain.MaxCardID {
// // 			return nil, fmt.Errorf("minimum_room_players must be between %d and %d", domain.MinPlayers, domain.MaxCardID)
// // 		}
// // 		cfg.MinimumRoomPlayers = *req.MinimumRoomPlayers
// // 	}
// // 	if req.Tiers != nil {
// // 		cfg.Tiers = *req.Tiers
// // 	}
// // 	if req.WinRate != nil {
// // 		if *req.WinRate < 0 || *req.WinRate > 1 {
// // 			return nil, fmt.Errorf("win_rate must be between 0 and 1")
// // 		}
// // 		cfg.WinRate = *req.WinRate
// // 	}
// // 	if req.BiasedDrawMode != nil {
// // 		if !req.BiasedDrawMode.IsValid() {
// // 			return nil, fmt.Errorf("biased_draw_mode must be disabled, legacy, or protected")
// // 		}
// // 		cfg.BiasedDrawMode = *req.BiasedDrawMode
// // 		cfg.BotAlwaysWin = cfg.BiasedDrawMode != domain.BiasedDrawModeDisabled
// // 	} else if req.BotAlwaysWin != nil {
// // 		cfg.BotAlwaysWin = *req.BotAlwaysWin
// // 		if cfg.BotAlwaysWin {
// // 			cfg.BiasedDrawMode = domain.BiasedDrawModeProtected
// // 		} else {
// // 			cfg.BiasedDrawMode = domain.BiasedDrawModeDisabled
// // 		}
// // 	}
// // 	if err := uc.botRepo.UpdateConfig(ctx, cfg); err != nil {
// // 		return nil, err
// // 	}
// // 	return cfg, nil
// // }

// // // ---- Seeding ----

// // func (uc *BotUseCase) SeedPool(ctx context.Context, count int) error {
// // 	if count <= 0 {
// // 		count = uc.settings.PoolSize
// // 	}
// // 	return uc.EnsureBotPool(ctx, count)
// // }

// // func (uc *BotUseCase) EnsureBotPool(ctx context.Context, size int) error {
// // 	for i := 1; i <= size; i++ {
// // 		telegramID := int64(botTelegramIDBase - i)

// // 		existing, err := uc.userRepo.FindByTelegramID(ctx, telegramID)
// // 		if err == nil && existing != nil {
// // 			if ferr := uc.fundBotWallet(ctx, existing.ID); ferr != nil {
// // 				return fmt.Errorf("fund existing bot %d: %w", i, ferr)
// // 			}
// // 			continue
// // 		}

// // 		if err := uc.createBot(ctx, i, telegramID); err != nil {
// // 			return fmt.Errorf("create bot %d: %w", i, err)
// // 		}
// // 	}
// // 	return nil
// // }

// // func (uc *BotUseCase) createBot(ctx context.Context, index int, telegramID int64) error {
// // 	code, err := uc.uniqueReferralCode(ctx)
// // 	if err != nil {
// // 		return err
// // 	}

// // 	name := botDisplayNames[(index-1)%len(botDisplayNames)]
// // 	phone := fmt.Sprintf("BOT-%08d", index)

// // 	tx, err := uc.db.BeginTx(ctx, nil)
// // 	if err != nil {
// // 		return err
// // 	}
// // 	defer tx.Rollback()

// // 	user := &domain.User{
// // 		ID:          uuid.New(),
// // 		TelegramID:  telegramID,
// // 		FirstName:   name,
// // 		PhoneNumber: phone,
// // 		ReferalCode: code,
// // 		Role:        "user",
// // 		IsBot:       true,
// // 	}
// // 	if err := uc.userRepo.Create(ctx, tx, user); err != nil {
// // 		return err
// // 	}

// // 	wallet := &domain.Wallet{UserID: user.ID, Balance: uc.settings.WalletFloat}
// // 	if err := uc.walletRepo.Create(ctx, tx, wallet); err != nil {
// // 		return err
// // 	}

// // 	if err := uc.recordBotFunding(ctx, tx, user.ID, uc.settings.WalletFloat); err != nil {
// // 		return err
// // 	}

// // 	return tx.Commit()
// // }

// // func (uc *BotUseCase) uniqueReferralCode(ctx context.Context) (string, error) {
// // 	for i := 0; i < domain.MaxReferralCodeGenerationAttempts; i++ {
// // 		code, err := referral.GenerateReferralCode()
// // 		if err != nil {
// // 			return "", fmt.Errorf("failed to generate referral code: %w", err)
// // 		}
// // 		if _, err := uc.userRepo.FindByReferralCode(ctx, code); err != nil {
// // 			return code, nil
// // 		}
// // 	}
// // 	return "", fmt.Errorf("failed to generate unique referral code")
// // }

// // func (uc *BotUseCase) fundBotWallet(ctx context.Context, userID uuid.UUID) error {
// // 	tx, err := uc.db.BeginTx(ctx, nil)
// // 	if err != nil {
// // 		return err
// // 	}
// // 	defer tx.Rollback()

// // 	wallet, err := uc.walletRepo.LockForUpdate(ctx, tx, userID)
// // 	if err != nil {
// // 		return err
// // 	}
// // 	topUp := uc.settings.WalletFloat - wallet.Balance
// // 	if topUp <= 0 {
// // 		return tx.Commit()
// // 	}
// // 	if err := uc.walletRepo.UpdateBalance(ctx, tx, userID, topUp); err != nil {
// // 		return err
// // 	}
// // 	if err := uc.recordBotFunding(ctx, tx, userID, topUp); err != nil {
// // 		return err
// // 	}
// // 	return tx.Commit()
// // }

// // func (uc *BotUseCase) recordBotFunding(ctx context.Context, tx *sql.Tx, userID uuid.UUID, amount float64) error {
// // 	ref := "BOT_FUNDING"
// // 	return uc.transactionRepo.Create(ctx, tx, &domain.Transaction{
// // 		UserID:    userID,
// // 		Type:      domain.TransactionTypeDeposit,
// // 		Category:  domain.TransactionCategoryBotFunding,
// // 		Amount:    amount,
// // 		Status:    domain.TransactionStatusCompleted,
// // 		Reference: &ref,
// // 	})
// // }

// // // ---- Filling (Optimized Bulk Join with Admin Control & Pacing) ----

// // func (uc *BotUseCase) FillGame(ctx context.Context, gameID uuid.UUID, requested int) (*domain.BotFillResult, error) {
// // 	// Manual admin fill: bypass MaxJoinsPerTick throttling so admin requests fulfill immediately
// // 	return uc.fill(ctx, gameID, requested, false, false)
// // }

// // func (uc *BotUseCase) fill(ctx context.Context, gameID uuid.UUID, requested int, allowEmpty bool, applyPacing bool) (*domain.BotFillResult, error) {
// // 	game, err := uc.gameRepo.FindByID(ctx, gameID)
// // 	if err != nil {
// // 		return nil, fmt.Errorf("game not found: %w", err)
// // 	}
// // 	if game.State != domain.GameStateWaiting && game.State != domain.GameStateCountdown {
// // 		return nil, fmt.Errorf("game is not accepting new players")
// // 	}

// // 	realPlayers, err := uc.botRepo.CountRealPlayersInGame(ctx, gameID)
// // 	if err != nil {
// // 		return nil, err
// // 	}
// // 	botPlayers, err := uc.botRepo.CountBotsInGame(ctx, gameID)
// // 	if err != nil {
// // 		return nil, err
// // 	}

// // 	result := &domain.BotFillResult{GameID: gameID, Requested: requested, RealPlayers: realPlayers, BotPlayers: botPlayers}

// // 	if !allowEmpty && realPlayers < 1 {
// // 		return result, nil
// // 	}

// // 	botCardCounts, err := uc.botCardCountsInGame(ctx, gameID)
// // 	if err != nil {
// // 		return nil, err
// // 	}
// // 	freeCards, err := uc.freeCards(ctx, gameID)
// // 	if err != nil {
// // 		return nil, err
// // 	}

// // 	bots, err := uc.botRepo.ListBots(ctx, uc.settings.PoolSize*2+requested+10)
// // 	if err != nil {
// // 		return nil, err
// // 	}

// // 	type cardAssignment struct {
// // 		userID uuid.UUID
// // 		cardID int
// // 	}
// // 	var assignments []cardAssignment

// // 	added := 0
// // 	for _, bot := range bots {
// // 		if added >= requested || len(freeCards) == 0 {
// // 			break
// // 		}

// // 		for botCardCounts[bot.ID] < maxCardsPerBot && added < requested && len(freeCards) > 0 {
// // 			if err := uc.ensureAffordable(ctx, bot.ID, game.BetAmount); err != nil {
// // 				break
// // 			}

// // 			cardID := freeCards[len(freeCards)-1]
// // 			freeCards = freeCards[:len(freeCards)-1]

// // 			assignments = append(assignments, cardAssignment{userID: bot.ID, cardID: cardID})
// // 			botCardCounts[bot.ID]++
// // 			added++
// // 		}
// // 	}

// // 	if len(assignments) == 0 {
// // 		result.Added = 0
// // 		result.BotPlayers = botPlayers
// // 		return result, nil
// // 	}

// // 	// Apply MaxJoinsPerTick pacing ONLY when requested by the automatic background sweeper
// // 	if applyPacing && uc.settings.MaxJoinsPerTick > 0 && len(assignments) > uc.settings.MaxJoinsPerTick {
// // 		assignments = assignments[:uc.settings.MaxJoinsPerTick]
// // 	}

// // 	// ---- BULK TRANSACTION: Insert this batch atomically ----
// // 	tx, err := uc.db.BeginTx(ctx, nil)
// // 	if err != nil {
// // 		return nil, fmt.Errorf("failed to begin bulk join tx: %w", err)
// // 	}
// // 	defer tx.Rollback()

// // 	game, err = uc.gameRepo.LockForUpdate(ctx, tx, gameID)
// // 	if err != nil {
// // 		return nil, fmt.Errorf("failed to lock game for bulk join: %w", err)
// // 	}
// // 	if game.State != domain.GameStateWaiting && game.State != domain.GameStateCountdown {
// // 		return nil, errors.New("game is no longer accepting new players")
// // 	}

// // 	prevPlayerCount := game.PlayerCount

// // 	for _, asm := range assignments {
// // 		player := &domain.GamePlayer{
// // 			ID:           uuid.New(),
// // 			GameID:       gameID,
// // 			UserID:       asm.userID,
// // 			CardID:       asm.cardID,
// // 			Paid:         false,
// // 			IsEliminated: false,
// // 		}
// // 		if err := uc.gameRepo.AddPlayer(ctx, tx, player); err != nil {
// // 			continue
// // 		}
// // 	}

// // 	active, err := uc.gameRepo.GetActivePlayersTx(ctx, tx, gameID)
// // 	if err != nil {
// // 		return nil, fmt.Errorf("failed to read active players: %w", err)
// // 	}
// // 	distinct := make(map[uuid.UUID]bool, len(active))
// // 	for _, p := range active {
// // 		distinct[p.UserID] = true
// // 	}
// // 	game.PlayerCount = len(distinct)
// // 	game.PrizePool = float64(len(active)) * game.BetAmount * (1 - game.HouseCut)

// // 	if err := uc.gameRepo.UpdateTx(ctx, tx, game); err != nil {
// // 		return nil, fmt.Errorf("failed to update game counters: %w", err)
// // 	}

// // 	if err := tx.Commit(); err != nil {
// // 		return nil, fmt.Errorf("failed to commit bulk join tx: %w", err)
// // 	}

// // 	for _, asm := range assignments {
// // 		uc.redisService.AddPlayer(ctx, gameID, asm.userID)
// // 		uc.redisService.AddTakenCard(ctx, gameID, asm.cardID)
// // 	}
// // 	uc.redisService.SaveGameState(ctx, game)

// // 	// STANDARD COUNTDOWN TRIGGER: Start countdown as soon as the room hits MinPlayers (2 distinct players/bots)
// // 	if game.State == domain.GameStateWaiting && prevPlayerCount < domain.MinPlayers && game.PlayerCount >= domain.MinPlayers {
// // 		utils.GoSafe("startCountdown", func() { uc.gameUC.startCountdown(context.Background(), gameID) })
// // 	}

// // 	uc.redisService.PublishEvent(ctx, gameID, domain.WebSocketEventPlayerJoined, map[string]interface{}{
// // 		"prize_pool":   game.PrizePool,
// // 		"player_count": game.PlayerCount,
// // 	})

// // 	result.Added = len(assignments)
// // 	result.BotPlayers = botPlayers + len(assignments)
// // 	return result, nil
// // }

// // func (uc *BotUseCase) ensureAffordable(ctx context.Context, userID uuid.UUID, bet float64) error {
// // 	wallet, err := uc.walletRepo.FindByUserID(ctx, userID)
// // 	if err == nil && wallet.Balance >= bet {
// // 		return nil
// // 	}
// // 	return uc.fundBotWallet(ctx, userID)
// // }

// // func (uc *BotUseCase) botCardCountsInGame(ctx context.Context, gameID uuid.UUID) (map[uuid.UUID]int, error) {
// // 	players, err := uc.gameRepo.GetPlayers(ctx, gameID)
// // 	if err != nil {
// // 		return nil, err
// // 	}
// // 	counts := make(map[uuid.UUID]int)
// // 	for _, p := range players {
// // 		if p != nil && p.IsBot && !p.IsEliminated && p.LeftAt == nil {
// // 			counts[p.UserID]++
// // 		}
// // 	}
// // 	return counts, nil
// // }

// // func (uc *BotUseCase) freeCards(ctx context.Context, gameID uuid.UUID) ([]int, error) {
// // 	taken, err := uc.gameRepo.GetTakenCards(ctx, gameID)
// // 	if err != nil {
// // 		return nil, err
// // 	}
// // 	takenSet := make(map[int]bool, len(taken))
// // 	for _, c := range taken {
// // 		takenSet[c] = true
// // 	}
// // 	free := make([]int, 0, domain.MaxCardID)
// // 	for c := domain.MinCardID; c <= domain.MaxCardID; c++ {
// // 		if !takenSet[c] {
// // 			free = append(free, c)
// // 		}
// // 	}
// // 	rand.Shuffle(len(free), func(i, j int) { free[i], free[j] = free[j], free[i] })
// // 	return free, nil
// // }

// // // ---- Auto-fill sweeper ----

// // func (uc *BotUseCase) joinDelayFor(gameID uuid.UUID) time.Duration {
// // 	base := uc.settings.JoinDelay
// // 	if base <= 0 {
// // 		return 0
// // 	}
// // 	spread := float64(gameID[0]) / 256.0
// // 	return base + time.Duration(float64(base)*0.5*spread)
// // }

// // // getShiftMultiplier calculates the bot volume multiplier based on 15 equal shifts (96 minutes each)
// // // across the international clock (00:00 to 23:59 UTC).
// // // - Shifts 1 & 2: Good night players
// // // - Shifts 3 & 4: Drop very low (deep sleep)
// // // - Shifts 5 & 6: Early morning recovery / some drop
// // // - Shifts 7 & 8: Average
// // // - Shift 9: High
// // // - Shifts 10 to 15: Very high (peak evening & night)
// // func getShiftMultiplier() float64 {
// // 	now := time.Now().UTC()
// // 	totalMinutes := now.Hour()*60 + now.Minute()

// // 	// 24 hours / 15 shifts = 96 minutes per shift
// // 	shiftDuration := 96
// // 	shiftIndex := totalMinutes / shiftDuration
// // 	if shiftIndex >= 15 {
// // 		shiftIndex = 14
// // 	}

// // 	multipliers := []float64{
// // 		0.7,  // Shift 1:  00:00 - 01:36 (Good night players)
// // 		0.6,  // Shift 2:  01:36 - 03:12 (Good night players)
// // 		0.2,  // Shift 3:  03:12 - 04:48 (Drop very low)
// // 		0.2,  // Shift 4:  04:48 - 06:24 (Drop very low)
// // 		0.4,  // Shift 5:  06:24 - 08:00 (Early morning recovery / some drop)
// // 		0.5,  // Shift 6:  08:00 - 09:36 (Morning start)
// // 		0.7,  // Shift 7:  09:36 - 11:12 (Average)
// // 		0.7,  // Shift 8:  11:12 - 12:48 (Average)
// // 		0.85, // Shift 9:  12:48 - 14:24 (High)
// // 		1.0,  // Shift 10: 14:24 - 16:00 (Very high)
// // 		1.0,  // Shift 11: 16:00 - 17:36 (Very high)
// // 		1.0,  // Shift 12: 17:36 - 19:12 (Very high)
// // 		1.0,  // Shift 13: 19:12 - 20:48 (Very high)
// // 		1.0,  // Shift 14: 20:48 - 22:24 (Very high)
// // 		1.0,  // Shift 15: 22:24 - 23:59 (Very high)
// // 	}

// // 	return multipliers[shiftIndex]
// // }

// // // desiredBotCount computes target bots scaled by the 15 international clock shifts.
// // func desiredBotCount(realPlayers, minimumRoomPlayers int, hasSpendableBonus, savedPolicyEnabled bool) int {
// // 	if minimumRoomPlayers <= 0 {
// // 		return 0
// // 	}

// // 	multiplier := getShiftMultiplier()
// // 	effectiveMinPlayers := int(float64(minimumRoomPlayers) * multiplier)
// // 	if effectiveMinPlayers < domain.MinPlayers {
// // 		effectiveMinPlayers = domain.MinPlayers
// // 	}

// // 	if realPlayers < effectiveMinPlayers {
// // 		return effectiveMinPlayers - realPlayers
// // 	}
// // 	if hasSpendableBonus && savedPolicyEnabled {
// // 		return highPopulationGuardBotCount
// // 	}
// // 	return 0
// // }

// // func (uc *BotUseCase) trimBots(ctx context.Context, gameID uuid.UUID, requested int) int {
// // 	if requested <= 0 {
// // 		return 0
// // 	}
// // 	players, err := uc.gameRepo.GetPlayers(ctx, gameID)
// // 	if err != nil {
// // 		return 0
// // 	}

// // 	latestByBot := make(map[uuid.UUID]*domain.GamePlayer)
// // 	for _, p := range players {
// // 		if p == nil || !p.IsBot || p.IsEliminated || p.LeftAt != nil {
// // 			continue
// // 		}
// // 		current := latestByBot[p.UserID]
// // 		if current == nil || p.JoinedAt.After(current.JoinedAt) {
// // 			latestByBot[p.UserID] = p
// // 		}
// // 	}

// // 	bots := make([]*domain.GamePlayer, 0, len(latestByBot))
// // 	for _, p := range latestByBot {
// // 		bots = append(bots, p)
// // 	}
// // 	sort.Slice(bots, func(i, j int) bool {
// // 		if bots[i].JoinedAt.Equal(bots[j].JoinedAt) {
// // 			return bots[i].UserID.String() > bots[j].UserID.String()
// // 		}
// // 		return bots[i].JoinedAt.After(bots[j].JoinedAt)
// // 	})

// // 	removed := 0
// // 	for _, bot := range bots {
// // 		if removed >= requested || ctx.Err() != nil {
// // 			break
// // 		}
// // 		if err := uc.gameUC.LeaveGame(ctx, gameID, domain.LeaveGameRequest{UserID: bot.UserID}); err == nil {
// // 			removed++
// // 		}
// // 	}
// // 	return removed
// // }

// // func (uc *BotUseCase) Run(ctx context.Context) {
// // 	ticker := time.NewTicker(uc.settings.CheckInterval)
// // 	defer ticker.Stop()

// // 	for {
// // 		select {
// // 		case <-ctx.Done():
// // 			return
// // 		case <-ticker.C:
// // 			uc.sweep(ctx)
// // 		}
// // 	}
// // }

// // func (uc *BotUseCase) sweep(ctx context.Context) {
// // 	cfg, err := uc.botRepo.GetConfig(ctx)
// // 	if err != nil || !cfg.Enabled || cfg.MinimumRoomPlayers < domain.MinPlayers {
// // 		return
// // 	}

// // 	savedPolicyEnabled := cfg.EffectiveBiasedDrawMode() != domain.BiasedDrawModeDisabled
// // 	for _, tier := range cfg.TierList() {
// // 		t := tier
// // 		games, err := uc.gameRepo.FindAvailable(ctx, &t, domain.MaxAvailableGamesLimit)
// // 		if err != nil {
// // 			continue
// // 		}
// // 		for _, game := range games {
// // 			realPlayers, err := uc.botRepo.CountRealPlayersInGame(ctx, game.ID)
// // 			if err != nil {
// // 				continue
// // 			}

// // 			hasSpendableBonus := false
// // 			if realPlayers >= cfg.MinimumRoomPlayers && savedPolicyEnabled {
// // 				hasSpendableBonus, err = uc.botRepo.HasSpendableBonusPlayerInGame(ctx, game.ID, game.BetAmount)
// // 				if err != nil {
// // 					continue
// // 				}
// // 			}

// // 			desired := desiredBotCount(realPlayers, cfg.MinimumRoomPlayers, hasSpendableBonus, savedPolicyEnabled)
// // 			botPlayers, err := uc.botRepo.CountBotsInGame(ctx, game.ID)
// // 			if err != nil {
// // 				continue
// // 			}

// // 			if botPlayers > desired {
// // 				uc.trimBots(ctx, game.ID, botPlayers-desired)
// // 				continue
// // 			}
// // 			if botPlayers == desired {
// // 				continue
// // 			}

// // 			if realPlayers > 0 {
// // 				if delay := uc.joinDelayFor(game.ID); delay > 0 {
// // 					age, hasReal, err := uc.botRepo.SecondsSinceFirstRealPlayer(ctx, game.ID)
// // 					if err != nil || !hasReal || time.Duration(age*float64(time.Second)) < delay {
// // 						continue
// // 					}
// // 				}
// // 			}

// // 			need := desired - botPlayers
// // 			// Sweeper fill applies pacing
// // 			if _, err := uc.fill(ctx, game.ID, need, realPlayers == 0, true); err != nil {
// // 				continue
// // 			}
// // 		}
// // 	}
// // }


// package usecase

// import (
// 	"context"
// 	"database/sql"
// 	"errors"
// 	"fmt"
// 	"math/rand"
// 	"sort"
// 	"time"

// 	"github.com/bingo/backend/internal/domain"
// 	redisGame "github.com/bingo/backend/pkg/redis"
// 	"github.com/bingo/backend/pkg/referral"
// 	utils "github.com/bingo/backend/pkg/utils"
// 	"github.com/google/uuid"
// )

// // botTelegramIDBase is the offset for synthetic bot Telegram IDs. Real Telegram
// // IDs are positive, so large negative IDs can never collide with a real user.
// const (
// 	botTelegramIDBase           = -1_000_000_000
// 	highPopulationGuardBotCount = 2
// 	maxCardsPerBot              = 2 // Allow each bot to hold up to 2 cards per game room
// )

// // botDisplayNames are Telegram-style display names used for house-controlled
// // filler bots, matching your clean list.
// var botDisplayNames = []string{
// 	"Natiye", "beki", "B", "bekaaaaaaaaa", "Solitude",
// 	"Godsproperty", "maki", "Beka", "Kiya", "Babi",
// 	"Yadi", "TemesgenYa", "N", "natiiii", "Eclipse",
// 	"YaAllah", "Moges", "Micky", "Mack", "Abela",
// 	"Fikir", "yabs", "Elsa", "edu", "Peace",
// 	"His", "B", "Abet", "Noah", "Kikiyeee",
// 	"Abi", "makgreat", "J", "hanicho", "Chaos",
// 	"Grace", "Ghosttt", "Tew", "Beka", "Mimi",
// 	"habib", "eduta", "K", "yabsss", "Loner",
// 	"Amen", "Edu", "Min", "Sami", "Nani",
// 	"Mela", "hani", "D", "sammyy", "Vibes",
// 	"Chosen", "Kira", "Awo", "Nati", "Tuti",
// 	"Tseda", "sami", "M", "makkk", "Aura",
// 	"Gospel", "yabs", "Ishi", "Hani", "Jiji",
// 	"Beki", "beti", "H", "abiii", "Moonchild",
// 	"Jesus", "miki", "Kiya", "Edu", "Fifi",
// 	"Robi", "chala", "Y", "fikirrr", "Soul",
// 	"Faith", "hani", "Ere", "Yoni", "Chuchu",
// 	"Daveeeeee", "muni", "elias", "lilii", "Broken",
// 	"Marys", "beti", "Gida", "Dani", "Didi",
// 	"Yoni", "nami", "A", "danii", "Faded",
// 	"Alhamdulillah", "jo", "Ebak", "Fikir", "Bobo",
// 	"Kaleb", "rozi", "T", "robb", "Ghost",
// 	"Sabr", "solo", "Tik", "Selam", "Mamo",
// 	"Fema", "meron", "R", "jerryy", "Silence",
// 	"Tawakkul", "ezi", "Fen", "Tsega", "Jojo",
// 	"ezra", "yerus", "L", "tutuu", "Empty",
// 	"Blessed", "dan", "Lemen", "Geta", "Nono",
// 	"Bruk", "mina", "P", "eziii", "Void",
// 	"Redeemed", "rob", "Man", "Rahmet", "Pipi",
// 	"Safi", "tsed", "C", "fiooo", "Nothing",
// 	"Saved", "fik", "Hul", "Juma", "Toto",
// 	"Elu", "yofe", "V", "soll", "Shadow",
// 	"Holy", "lid", "Chil", "Hiwot", "Lolo",
// 	"Jeri", "abi", "Z", "babb", "Bliss",
// 	"Pray", "na", "Wey", "Tesfa", "Yoyo",
// 	"Redi", "meze", "G", "garii", "Serenity",
// 	"Lord", "kal", "Ged", "Birhan", "Gogo",
// 	"Bogi", "zed", "F", "noahh", "Lost",
// 	"Christ", "bru", "Zim", "Kal", "Shushu",
// 	"Miki", "jappy", "T", "sidd", "Tired",
// 	"Saint", "sel", "Besm", "Emnet", "Kuku",
// 	"Seli", "lela", "W", "yemii", "Enigma",
// 	"Peace", "mel", "Kef", "Tsehay", "Maca",
// 	"Lina", "eba", "Q", "farr", "Abyss",
// 	"Mercy", "Fitse", "Are", "Chereka", "Papa",
// 	"Miky", "tofik", "X", "AB", "Mirage",
// 	"Heaven", "Abenezer", "Dagmawi", "Kiyaye", "Daniel",
// 	"dagi", "jossi", "Kirubel", "kiraa", "Echo",
// 	"Angel", "Father", "Ende", "Semay", "Coco",
// 	"Fasilo", "Yichalal", "Demelash", "lderu", "Chill",
// 	"Prophet", "Dereje", "Dere", "Midir", "Zizu",
// 	"Hena", "hiwi", "User", "titi", "Dawn",
// 	"Nour", "Bomb", "Tel", "Zimta", "Ririfikir",
// 	"Amu", "mack", "U", "Timiro", "Dusk",
// 	"Deen", "Bella", "ErmiyasAsa", "Ewnet", "Davinchi",
// 	"YonasG", "yordi", "K", "mariiii", "Luna",
// 	"Glory", "Goytom", "Abrish", "Sewmalet", "Meseret",
// 	"Abdulselam", "Abdi", "Kedir", "babyyyy", "Starboy",
// 	"MyLove", "Yabsera", "Faya", "Nuro", "ElaBest",
// }

// // BotSettings holds the operator-tunable knobs supplied from config/env.
// type BotSettings struct {
// 	PoolSize        int           // how many bot accounts to seed
// 	WalletFloat     float64       // balance each bot wallet is topped up to
// 	MaxJoinsPerTick int           // bots added per game per sweep (spaces out joins)
// 	JoinDelay       time.Duration // hold bots back this long after the first real player joins
// 	CheckInterval   time.Duration // how often the auto-filler sweeps
// 	WinRate         float64       // probability (0-1) that bots win when they have a valid bingo alongside humans
// }

// // BotUseCase seeds a pool of house-owned filler bots and joins them into games
// type BotUseCase struct {
// 	botRepo         domain.BotRepository
// 	userRepo        domain.UserRepository
// 	walletRepo      domain.WalletRepository
// 	transactionRepo domain.TransactionRepository
// 	gameRepo        domain.GameRepository
// 	gameUC          *GameUseCase
// 	gameState       *redisGame.GameStateService
// 	redisService    *redisGame.GameStateService
// 	db              *sql.DB
// 	settings        BotSettings
// }

// // NewBotUseCase wires the bot use case.
// func NewBotUseCase(
// 	botRepo domain.BotRepository,
// 	userRepo domain.UserRepository,
// 	walletRepo domain.WalletRepository,
// 	transactionRepo domain.TransactionRepository,
// 	gameRepo domain.GameRepository,
// 	gameUC *GameUseCase,
// 	gameState *redisGame.GameStateService,
// 	db *sql.DB,
// 	settings BotSettings,
// ) *BotUseCase {
// 	if settings.WalletFloat <= 0 {
// 		settings.WalletFloat = 1000.0
// 	}
// 	if settings.PoolSize <= 0 {
// 		settings.PoolSize = 50
// 	}
// 	return &BotUseCase{
// 		botRepo:         botRepo,
// 		userRepo:        userRepo,
// 		walletRepo:      walletRepo,
// 		transactionRepo: transactionRepo,
// 		gameRepo:        gameRepo,
// 		gameUC:          gameUC,
// 		gameState:       gameState,
// 		redisService:    gameState,
// 		db:              db,
// 		settings:        settings,
// 	}
// }

// // ---- Config passthrough (admin dashboard) ----

// func (uc *BotUseCase) GetConfig(ctx context.Context) (*domain.BotConfig, error) {
// 	return uc.botRepo.GetConfig(ctx)
// }

// func (uc *BotUseCase) UpdateConfig(ctx context.Context, req domain.UpdateBotConfigRequest) (*domain.BotConfig, error) {
// 	cfg, err := uc.botRepo.GetConfig(ctx)
// 	if err != nil {
// 		return nil, err
// 	}
// 	if req.Enabled != nil {
// 		cfg.Enabled = *req.Enabled
// 	}
// 	if req.MinRealPlayers != nil {
// 		if *req.MinRealPlayers < 0 {
// 			return nil, fmt.Errorf("min_real_players cannot be negative")
// 		}
// 		cfg.MinRealPlayers = *req.MinRealPlayers
// 	}
// 	if req.TargetBots != nil {
// 		if *req.TargetBots < 0 {
// 			return nil, fmt.Errorf("target_bots cannot be negative")
// 		}
// 		cfg.TargetBots = *req.TargetBots
// 	}
// 	if req.MinimumRoomPlayers != nil {
// 		if *req.MinimumRoomPlayers < 0 || *req.MinimumRoomPlayers > domain.MaxCardID {
// 			return nil, fmt.Errorf("minimum_room_players must be between 0 and %d", domain.MaxCardID)
// 		}
// 		cfg.MinimumRoomPlayers = *req.MinimumRoomPlayers
// 	}
// 	if req.Tiers != nil {
// 		cfg.Tiers = *req.Tiers
// 	}
// 	if req.WinRate != nil {
// 		if *req.WinRate < 0 || *req.WinRate > 1 {
// 			return nil, fmt.Errorf("win_rate must be between 0 and 1")
// 		}
// 		cfg.WinRate = *req.WinRate
// 	}
// 	if req.BiasedDrawMode != nil {
// 		if !req.BiasedDrawMode.IsValid() {
// 			return nil, fmt.Errorf("biased_draw_mode must be disabled, legacy, or protected")
// 		}
// 		cfg.BiasedDrawMode = *req.BiasedDrawMode
// 		cfg.BotAlwaysWin = cfg.BiasedDrawMode != domain.BiasedDrawModeDisabled
// 	} else if req.BotAlwaysWin != nil {
// 		cfg.BotAlwaysWin = *req.BotAlwaysWin
// 		if cfg.BotAlwaysWin {
// 			cfg.BiasedDrawMode = domain.BiasedDrawModeProtected
// 		} else {
// 			cfg.BiasedDrawMode = domain.BiasedDrawModeDisabled
// 		}
// 	}
// 	if err := uc.botRepo.UpdateConfig(ctx, cfg); err != nil {
// 		return nil, err
// 	}
// 	return cfg, nil
// }

// // ---- Seeding ----

// func (uc *BotUseCase) SeedPool(ctx context.Context, count int) error {
// 	if count <= 0 {
// 		count = uc.settings.PoolSize
// 	}
// 	return uc.EnsureBotPool(ctx, count)
// }

// func (uc *BotUseCase) EnsureBotPool(ctx context.Context, size int) error {
// 	if size <= 0 {
// 		size = 50
// 	}
// 	for i := 1; i <= size; i++ {
// 		telegramID := int64(botTelegramIDBase - i)

// 		existing, err := uc.userRepo.FindByTelegramID(ctx, telegramID)
// 		if err == nil && existing != nil {
// 			if ferr := uc.fundBotWallet(ctx, existing.ID); ferr != nil {
// 				return fmt.Errorf("fund existing bot %d: %w", i, ferr)
// 			}
// 			continue
// 		}

// 		if err := uc.createBot(ctx, i, telegramID); err != nil {
// 			return fmt.Errorf("create bot %d: %w", i, err)
// 		}
// 	}
// 	return nil
// }

// func (uc *BotUseCase) createBot(ctx context.Context, index int, telegramID int64) error {
// 	code, err := uc.uniqueReferralCode(ctx)
// 	if err != nil {
// 		return err
// 	}

// 	name := botDisplayNames[(index-1)%len(botDisplayNames)]
// 	phone := fmt.Sprintf("BOT-%08d", index)

// 	tx, err := uc.db.BeginTx(ctx, nil)
// 	if err != nil {
// 		return err
// 	}
// 	defer tx.Rollback()

// 	user := &domain.User{
// 		ID:          uuid.New(),
// 		TelegramID:  telegramID,
// 		FirstName:   name,
// 		PhoneNumber: phone,
// 		ReferalCode: code,
// 		Role:        "user",
// 		IsBot:       true,
// 	}
// 	if err := uc.userRepo.Create(ctx, tx, user); err != nil {
// 		return err
// 	}

// 	floatAmt := uc.settings.WalletFloat
// 	if floatAmt <= 0 {
// 		floatAmt = 1000.0
// 	}

// 	wallet := &domain.Wallet{UserID: user.ID, Balance: floatAmt}
// 	if err := uc.walletRepo.Create(ctx, tx, wallet); err != nil {
// 		return err
// 	}

// 	if err := uc.recordBotFunding(ctx, tx, user.ID, floatAmt); err != nil {
// 		return err
// 	}

// 	return tx.Commit()
// }

// func (uc *BotUseCase) uniqueReferralCode(ctx context.Context) (string, error) {
// 	for i := 0; i < domain.MaxReferralCodeGenerationAttempts; i++ {
// 		code, err := referral.GenerateReferralCode()
// 		if err != nil {
// 			return "", fmt.Errorf("failed to generate referral code: %w", err)
// 		}
// 		if _, err := uc.userRepo.FindByReferralCode(ctx, code); err != nil {
// 			return code, nil
// 		}
// 	}
// 	return "", fmt.Errorf("failed to generate unique referral code")
// }

// func (uc *BotUseCase) fundBotWallet(ctx context.Context, userID uuid.UUID) error {
// 	tx, err := uc.db.BeginTx(ctx, nil)
// 	if err != nil {
// 		return err
// 	}
// 	defer tx.Rollback()

// 	wallet, err := uc.walletRepo.LockForUpdate(ctx, tx, userID)
// 	if err != nil {
// 		return err
// 	}
// 	floatAmt := uc.settings.WalletFloat
// 	if floatAmt <= 0 {
// 		floatAmt = 1000.0
// 	}
// 	topUp := floatAmt - wallet.Balance
// 	if topUp <= 0 {
// 		return tx.Commit()
// 	}
// 	if err := uc.walletRepo.UpdateBalance(ctx, tx, userID, topUp); err != nil {
// 		return err
// 	}
// 	if err := uc.recordBotFunding(ctx, tx, userID, topUp); err != nil {
// 		return err
// 	}
// 	return tx.Commit()
// }

// func (uc *BotUseCase) recordBotFunding(ctx context.Context, tx *sql.Tx, userID uuid.UUID, amount float64) error {
// 	ref := "BOT_FUNDING"
// 	return uc.transactionRepo.Create(ctx, tx, &domain.Transaction{
// 		UserID:    userID,
// 		Type:      domain.TransactionTypeDeposit,
// 		Category:  domain.TransactionCategoryBotFunding,
// 		Amount:    amount,
// 		Status:    domain.TransactionStatusCompleted,
// 		Reference: &ref,
// 	})
// }

// // ---- Filling (Optimized Bulk Join with Admin Control & Pacing) ----

// func (uc *BotUseCase) FillGame(ctx context.Context, gameID uuid.UUID, requested int) (*domain.BotFillResult, error) {
// 	// Manual admin fill: bypass throttling so admin requests fulfill immediately
// 	return uc.fill(ctx, gameID, requested, false, false)
// }

// func (uc *BotUseCase) fill(ctx context.Context, gameID uuid.UUID, requested int, allowEmpty bool, applyPacing bool) (*domain.BotFillResult, error) {
// 	game, err := uc.gameRepo.FindByID(ctx, gameID)
// 	if err != nil {
// 		return nil, fmt.Errorf("game not found: %w", err)
// 	}
// 	if game.State != domain.GameStateWaiting && game.State != domain.GameStateCountdown {
// 		return nil, fmt.Errorf("game is not accepting new players")
// 	}

// 	realPlayers, err := uc.botRepo.CountRealPlayersInGame(ctx, gameID)
// 	if err != nil {
// 		return nil, err
// 	}
// 	botPlayers, err := uc.botRepo.CountBotsInGame(ctx, gameID)
// 	if err != nil {
// 		return nil, err
// 	}

// 	result := &domain.BotFillResult{GameID: gameID, Requested: requested, RealPlayers: realPlayers, BotPlayers: botPlayers}

// 	if !allowEmpty && realPlayers < 1 {
// 		return result, nil
// 	}

// 	botCardCounts, err := uc.botCardCountsInGame(ctx, gameID)
// 	if err != nil {
// 		return nil, err
// 	}
// 	freeCards, err := uc.freeCards(ctx, gameID)
// 	if err != nil {
// 		return nil, err
// 	}

// 	poolCheckSize := uc.settings.PoolSize * 2
// 	if poolCheckSize < 100 {
// 		poolCheckSize = 100
// 	}
// 	bots, err := uc.botRepo.ListBots(ctx, poolCheckSize+requested+10)
// 	if err != nil || len(bots) == 0 {
// 		_ = uc.EnsureBotPool(ctx, 50)
// 		bots, err = uc.botRepo.ListBots(ctx, 100)
// 		if err != nil || len(bots) == 0 {
// 			return result, fmt.Errorf("no bot accounts available in database")
// 		}
// 	}

// 	type cardAssignment struct {
// 		userID uuid.UUID
// 		cardID int
// 	}
// 	var assignments []cardAssignment

// 	added := 0
// 	for _, bot := range bots {
// 		if added >= requested || len(freeCards) == 0 {
// 			break
// 		}

// 		for botCardCounts[bot.ID] < maxCardsPerBot && added < requested && len(freeCards) > 0 {
// 			if err := uc.ensureAffordable(ctx, bot.ID, game.BetAmount); err != nil {
// 				break
// 			}

// 			cardID := freeCards[len(freeCards)-1]
// 			freeCards = freeCards[:len(freeCards)-1]

// 			assignments = append(assignments, cardAssignment{userID: bot.ID, cardID: cardID})
// 			botCardCounts[bot.ID]++
// 			added++
// 		}
// 	}

// 	if len(assignments) == 0 {
// 		result.Added = 0
// 		result.BotPlayers = botPlayers
// 		return result, nil
// 	}

// 	if applyPacing && uc.settings.MaxJoinsPerTick > 0 && len(assignments) > uc.settings.MaxJoinsPerTick {
// 		assignments = assignments[:uc.settings.MaxJoinsPerTick]
// 	}

// 	// ---- BULK TRANSACTION: Insert this batch atomically ----
// 	tx, err := uc.db.BeginTx(ctx, nil)
// 	if err != nil {
// 		return nil, fmt.Errorf("failed to begin bulk join tx: %w", err)
// 	}
// 	defer tx.Rollback()

// 	game, err = uc.gameRepo.LockForUpdate(ctx, tx, gameID)
// 	if err != nil {
// 		return nil, fmt.Errorf("failed to lock game for bulk join: %w", err)
// 	}
// 	if game.State != domain.GameStateWaiting && game.State != domain.GameStateCountdown {
// 		return nil, errors.New("game is no longer accepting new players")
// 	}

// 	prevPlayerCount := game.PlayerCount

// 	for _, asm := range assignments {
// 		player := &domain.GamePlayer{
// 			ID:           uuid.New(),
// 			GameID:       gameID,
// 			UserID:       asm.userID,
// 			CardID:       asm.cardID,
// 			Paid:         false,
// 			IsEliminated: false,
// 		}
// 		if err := uc.gameRepo.AddPlayer(ctx, tx, player); err != nil {
// 			continue
// 		}
// 	}

// 	active, err := uc.gameRepo.GetActivePlayersTx(ctx, tx, gameID)
// 	if err != nil {
// 		return nil, fmt.Errorf("failed to read active players: %w", err)
// 	}
// 	distinct := make(map[uuid.UUID]bool, len(active))
// 	for _, p := range active {
// 		distinct[p.UserID] = true
// 	}
// 	game.PlayerCount = len(distinct)
// 	game.PrizePool = float64(len(active)) * game.BetAmount * (1 - game.HouseCut)

// 	if err := uc.gameRepo.UpdateTx(ctx, tx, game); err != nil {
// 		return nil, fmt.Errorf("failed to update game counters: %w", err)
// 	}

// 	if err := tx.Commit(); err != nil {
// 		return nil, fmt.Errorf("failed to commit bulk join tx: %w", err)
// 	}

// 	for _, asm := range assignments {
// 		uc.redisService.AddPlayer(ctx, gameID, asm.userID)
// 		uc.redisService.AddTakenCard(ctx, gameID, asm.cardID)
// 	}
// 	uc.redisService.SaveGameState(ctx, game)

// 	if game.State == domain.GameStateWaiting && prevPlayerCount < domain.MinPlayers && game.PlayerCount >= domain.MinPlayers {
// 		utils.GoSafe("startCountdown", func() { uc.gameUC.startCountdown(context.Background(), gameID) })
// 	}

// 	uc.redisService.PublishEvent(ctx, gameID, domain.WebSocketEventPlayerJoined, map[string]interface{}{
// 		"prize_pool":   game.PrizePool,
// 		"player_count": game.PlayerCount,
// 	})

// 	result.Added = len(assignments)
// 	result.BotPlayers = botPlayers + len(assignments)
// 	return result, nil
// }

// func (uc *BotUseCase) ensureAffordable(ctx context.Context, userID uuid.UUID, bet float64) error {
// 	wallet, err := uc.walletRepo.FindByUserID(ctx, userID)
// 	if err == nil && wallet.Balance >= bet {
// 		return nil
// 	}
// 	return uc.fundBotWallet(ctx, userID)
// }

// func (uc *BotUseCase) botCardCountsInGame(ctx context.Context, gameID uuid.UUID) (map[uuid.UUID]int, error) {
// 	players, err := uc.gameRepo.GetPlayers(ctx, gameID)
// 	if err != nil {
// 		return nil, err
// 	}
// 	counts := make(map[uuid.UUID]int)
// 	for _, p := range players {
// 		if p != nil && p.IsBot && !p.IsEliminated && p.LeftAt == nil {
// 			counts[p.UserID]++
// 		}
// 	}
// 	return counts, nil
// }

// func (uc *BotUseCase) freeCards(ctx context.Context, gameID uuid.UUID) ([]int, error) {
// 	taken, err := uc.gameRepo.GetTakenCards(ctx, gameID)
// 	if err != nil {
// 		return nil, err
// 	}
// 	takenSet := make(map[int]bool, len(taken))
// 	for _, c := range taken {
// 		takenSet[c] = true
// 	}
// 	free := make([]int, 0, domain.MaxCardID)
// 	for c := domain.MinCardID; c <= domain.MaxCardID; c++ {
// 		if !takenSet[c] {
// 			free = append(free, c)
// 		}
// 	}
// 	rand.Shuffle(len(free), func(i, j int) { free[i], free[j] = free[j], free[i] })
// 	return free, nil
// }

// // ---- Auto-fill sweeper ----

// func (uc *BotUseCase) joinDelayFor(gameID uuid.UUID) time.Duration {
// 	base := uc.settings.JoinDelay
// 	if base <= 0 {
// 		return 0
// 	}
// 	spread := float64(gameID[0]) / 256.0
// 	return base + time.Duration(float64(base)*0.5*spread)
// }

// // getShiftMultiplier calculates the bot volume multiplier based on 15 equal shifts (96 minutes each)
// // synchronized to local time in East Africa Time (EAT / UTC+3, Addis Ababa).
// func getShiftMultiplier() float64 {
// 	loc, err := time.LoadLocation("Africa/Addis_Ababa")
// 	if err != nil {
// 		loc = time.FixedZone("EAT", 3*60*60)
// 	}
// 	now := time.Now().In(loc)
// 	totalMinutes := now.Hour()*60 + now.Minute()

// 	shiftDuration := 96 // 24 hours / 15 shifts = 96 minutes per shift
// 	shiftIndex := totalMinutes / shiftDuration
// 	if shiftIndex >= 15 {
// 		shiftIndex = 14
// 	}

// 	multipliers := []float64{
// 		0.7,  // Shift 1:  00:00 - 01:36 (Good night players)
// 		0.6,  // Shift 2:  01:36 - 03:12 (Good night players)
// 		0.2,  // Shift 3:  03:12 - 04:48 (Drop very low)
// 		0.2,  // Shift 4:  04:48 - 06:24 (Drop very low)
// 		0.4,  // Shift 5:  06:24 - 08:00 (Early morning recovery)
// 		0.5,  // Shift 6:  08:00 - 09:36 (Morning start)
// 		0.7,  // Shift 7:  09:36 - 11:12 (Average)
// 		0.85, // Shift 8:  11:12 - 12:48 (Average)
// 		1.0,  // Shift 9:  12:48 - 14:24 (High)
// 		1.0,  // Shift 10: 14:24 - 16:00 (Very high)
// 		1.0,  // Shift 11: 16:00 - 17:36 (Very high)
// 		1.0,  // Shift 12: 17:36 - 19:12 (Very high)
// 		1.0,  // Shift 13: 19:12 - 20:48 (Very high)
// 		1.0,  // Shift 14: 20:48 - 22:24 (Very high)
// 		1.0,  // Shift 15: 22:24 - 23:59 (Very high)
// 	}

// 	return multipliers[shiftIndex]
// }

// // desiredBotCount computes target bots scaled by the 15 time shifts in local time.
// func desiredBotCount(realPlayers, minimumRoomPlayers int, hasSpendableBonus, savedPolicyEnabled bool) int {
// 	if minimumRoomPlayers <= 0 {
// 		minimumRoomPlayers = domain.MinPlayers
// 	}

// 	multiplier := getShiftMultiplier()
// 	effectiveMinPlayers := int(float64(minimumRoomPlayers) * multiplier)
// 	if effectiveMinPlayers < domain.MinPlayers {
// 		effectiveMinPlayers = domain.MinPlayers
// 	}

// 	if realPlayers < effectiveMinPlayers {
// 		return effectiveMinPlayers - realPlayers
// 	}
// 	if hasSpendableBonus && savedPolicyEnabled {
// 		return highPopulationGuardBotCount
// 	}
// 	return 0
// }

// func (uc *BotUseCase) trimBots(ctx context.Context, gameID uuid.UUID, requested int) int {
// 	if requested <= 0 {
// 		return 0
// 	}
// 	players, err := uc.gameRepo.GetPlayers(ctx, gameID)
// 	if err != nil {
// 		return 0
// 	}

// 	latestByBot := make(map[uuid.UUID]*domain.GamePlayer)
// 	for _, p := range players {
// 		if p == nil || !p.IsBot || p.IsEliminated || p.LeftAt != nil {
// 			continue
// 		}
// 		current := latestByBot[p.UserID]
// 		if current == nil || p.JoinedAt.After(current.JoinedAt) {
// 			latestByBot[p.UserID] = p
// 		}
// 	}

// 	bots := make([]*domain.GamePlayer, 0, len(latestByBot))
// 	for _, p := range latestByBot {
// 		bots = append(bots, p)
// 	}
// 	sort.Slice(bots, func(i, j int) bool {
// 		if bots[i].JoinedAt.Equal(bots[j].JoinedAt) {
// 			return bots[i].UserID.String() > bots[j].UserID.String()
// 		}
// 		return bots[i].JoinedAt.After(bots[j].JoinedAt)
// 	})

// 	removed := 0
// 	for _, bot := range bots {
// 		if removed >= requested || ctx.Err() != nil {
// 			break
// 		}
// 		if err := uc.gameUC.LeaveGame(ctx, gameID, domain.LeaveGameRequest{UserID: bot.UserID}); err == nil {
// 			removed++
// 		}
// 	}
// 	return removed
// }

// func (uc *BotUseCase) Run(ctx context.Context) {
// 	ticker := time.NewTicker(uc.settings.CheckInterval)
// 	defer ticker.Stop()

// 	for {
// 		select {
// 		case <-ctx.Done():
// 			return
// 		case <-ticker.C:
// 			uc.sweep(ctx)
// 		}
// 	}
// }

// func (uc *BotUseCase) sweep(ctx context.Context) {
// 	cfg, err := uc.botRepo.GetConfig(ctx)
// 	if err != nil || !cfg.Enabled {
// 		return
// 	}

// 	minRoomPlayers := cfg.MinimumRoomPlayers
// 	if minRoomPlayers < domain.MinPlayers {
// 		minRoomPlayers = domain.MinPlayers
// 	}

// 	savedPolicyEnabled := cfg.EffectiveBiasedDrawMode() != domain.BiasedDrawModeDisabled
// 	for _, tier := range cfg.TierList() {
// 		t := tier
// 		games, err := uc.gameRepo.FindAvailable(ctx, &t, domain.MaxAvailableGamesLimit)
// 		if err != nil {
// 			continue
// 		}
// 		for _, game := range games {
// 			realPlayers, err := uc.botRepo.CountRealPlayersInGame(ctx, game.ID)
// 			if err != nil {
// 				continue
// 			}

// 			hasSpendableBonus := false
// 			if realPlayers >= minRoomPlayers && savedPolicyEnabled {
// 				hasSpendableBonus, err = uc.botRepo.HasSpendableBonusPlayerInGame(ctx, game.ID, game.BetAmount)
// 				if err != nil {
// 					continue
// 				}
// 			}

// 			desired := desiredBotCount(realPlayers, minRoomPlayers, hasSpendableBonus, savedPolicyEnabled)
// 			botPlayers, err := uc.botRepo.CountBotsInGame(ctx, game.ID)
// 			if err != nil {
// 				continue
// 			}

// 			if botPlayers > desired {
// 				uc.trimBots(ctx, game.ID, botPlayers-desired)
// 				continue
// 			}
// 			if botPlayers == desired {
// 				continue
// 			}

// 			// Bypass join delay check if join delay is zero or disabled, ensuring swift entry
// 			if realPlayers > 0 && uc.settings.JoinDelay > 0 {
// 				if delay := uc.joinDelayFor(game.ID); delay > 0 {
// 					age, hasReal, err := uc.botRepo.SecondsSinceFirstRealPlayer(ctx, game.ID)
// 					if err != nil || !hasReal || time.Duration(age*float64(time.Second)) < delay {
// 						continue
// 					}
// 				}
// 			}

// 			need := desired - botPlayers
// 			if _, err := uc.fill(ctx, game.ID, need, realPlayers == 0, true); err != nil {
// 				continue
// 			}
// 		}
// 	}
// }


package usecase

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math/rand"
	"sort"
	"time"

	"github.com/bingo/backend/internal/domain"
	redisGame "github.com/bingo/backend/pkg/redis"
	"github.com/bingo/backend/pkg/referral"
	utils "github.com/bingo/backend/pkg/utils"
	"github.com/google/uuid"
)

// botTelegramIDBase is the offset for synthetic bot Telegram IDs. Real Telegram
// IDs are positive, so large negative IDs can never collide with a real user.
const (
	botTelegramIDBase           = -1_000_000_000
	highPopulationGuardBotCount = 2
	maxCardsPerBot              = 3 // Increased to allow each bot to hold up to 3 cards per game room (targeting 362+ card slots)
)

// botDisplayNames are Telegram-style display names used for house-controlled
// filler bots, matching your clean list.
var botDisplayNames = []string{
	"Natiye", "beki", "B", "bekaaaaaaaaa", "Solitude",
	"Godsproperty", "maki", "Beka", "Kiya", "Babi",
	"Yadi", "TemesgenYa", "N", "natiiii", "Eclipse",
	"YaAllah", "Moges", "Micky", "Mack", "Abela",
	"Fikir", "yabs", "Elsa", "edu", "Peace",
	"His", "B", "Abet", "Noah", "Kikiyeee",
	"Abi", "makgreat", "J", "hanicho", "Chaos",
	"Grace", "Ghosttt", "Tew", "Beka", "Mimi",
	"habib", "eduta", "K", "yabsss", "Loner",
	"Amen", "Edu", "Min", "Sami", "Nani",
	"Mela", "hani", "D", "sammyy", "Vibes",
	"Chosen", "Kira", "Awo", "Nati", "Tuti",
	"Tseda", "sami", "M", "makkk", "Aura",
	"Gospel", "yabs", "Ishi", "Hani", "Jiji",
	"Beki", "beti", "H", "abiii", "Moonchild",
	"Jesus", "miki", "Kiya", "Edu", "Fifi",
	"Robi", "chala", "Y", "fikirrr", "Soul",
	"Faith", "hani", "Ere", "Yoni", "Chuchu",
	"Daveeeeee", "muni", "elias", "lilii", "Broken",
	"Marys", "beti", "Gida", "Dani", "Didi",
	"Yoni", "nami", "A", "danii", "Faded",
	"Alhamdulillah", "jo", "Ebak", "Fikir", "Bobo",
	"Kaleb", "rozi", "T", "robb", "Ghost",
	"Sabr", "solo", "Tik", "Selam", "Mamo",
	"Fema", "meron", "R", "jerryy", "Silence",
	"Tawakkul", "ezi", "Fen", "Tsega", "Jojo",
	"ezra", "yerus", "L", "tutuu", "Empty",
	"Blessed", "dan", "Lemen", "Geta", "Nono",
	"Bruk", "mina", "P", "eziii", "Void",
	"Redeemed", "rob", "Man", "Rahmet", "Pipi",
	"Safi", "tsed", "C", "fiooo", "Nothing",
	"Saved", "fik", "Hul", "Juma", "Toto",
	"Elu", "yofe", "V", "soll", "Shadow",
	"Holy", "lid", "Chil", "Hiwot", "Lolo",
	"Jeri", "abi", "Z", "babb", "Bliss",
	"Pray", "na", "Wey", "Tesfa", "Yoyo",
	"Redi", "meze", "G", "garii", "Serenity",
	"Lord", "kal", "Ged", "Birhan", "Gogo",
	"Bogi", "zed", "F", "noahh", "Lost",
	"Christ", "bru", "Zim", "Kal", "Shushu",
	"Miki", "jappy", "T", "sidd", "Tired",
	"Saint", "sel", "Besm", "Emnet", "Kuku",
	"Seli", "lela", "W", "yemii", "Enigma",
	"Peace", "mel", "Kef", "Tsehay", "Maca",
	"Lina", "eba", "Q", "farr", "Abyss",
	"Mercy", "Fitse", "Are", "Chereka", "Papa",
	"Miky", "tofik", "X", "AB", "Mirage",
	"Heaven", "Abenezer", "Dagmawi", "Kiyaye", "Daniel",
	"dagi", "jossi", "Kirubel", "kiraa", "Echo",
	"Angel", "Father", "Ende", "Semay", "Coco",
	"Fasilo", "Yichalal", "Demelash", "lderu", "Chill",
	"Prophet", "Dereje", "Dere", "Midir", "Zizu",
	"Hena", "hiwi", "User", "titi", "Dawn",
	"Nour", "Bomb", "Tel", "Zimta", "Ririfikir",
	"Amu", "mack", "U", "Timiro", "Dusk",
	"Deen", "Bella", "ErmiyasAsa", "Ewnet", "Davinchi",
	"YonasG", "yordi", "K", "mariiii", "Luna",
	"Glory", "Goytom", "Abrish", "Sewmalet", "Meseret",
	"Abdulselam", "Abdi", "Kedir", "babyyyy", "Starboy",
	"MyLove", "Yabsera", "Faya", "Nuro", "ElaBest",
}

// BotSettings holds the operator-tunable knobs supplied from config/env.
type BotSettings struct {
	PoolSize        int           // how many bot accounts to seed
	WalletFloat     float64       // balance each bot wallet is topped up to
	MaxJoinsPerTick int           // bots added per game per sweep (spaces out joins)
	JoinDelay       time.Duration // hold bots back this long after the first real player joins
	CheckInterval   time.Duration // how often the auto-filler sweeps
	WinRate         float64       // probability (0-1) that bots win when they have a valid bingo alongside humans
}

// BotUseCase seeds a pool of house-owned filler bots and joins them into games
type BotUseCase struct {
	botRepo         domain.BotRepository
	userRepo        domain.UserRepository
	walletRepo      domain.WalletRepository
	transactionRepo domain.TransactionRepository
	gameRepo        domain.GameRepository
	gameUC          *GameUseCase
	gameState       *redisGame.GameStateService
	redisService    *redisGame.GameStateService
	db              *sql.DB
	settings        BotSettings
}

// NewBotUseCase wires the bot use case.
func NewBotUseCase(
	botRepo domain.BotRepository,
	userRepo domain.UserRepository,
	walletRepo domain.WalletRepository,
	transactionRepo domain.TransactionRepository,
	gameRepo domain.GameRepository,
	gameUC *GameUseCase,
	gameState *redisGame.GameStateService,
	db *sql.DB,
	settings BotSettings,
) *BotUseCase {
	if settings.WalletFloat <= 0 {
		settings.WalletFloat = 1000.0
	}
	if settings.PoolSize <= 0 {
		settings.PoolSize = 50
	}
	return &BotUseCase{
		botRepo:         botRepo,
		userRepo:        userRepo,
		walletRepo:      walletRepo,
		transactionRepo: transactionRepo,
		gameRepo:        gameRepo,
		gameUC:          gameUC,
		gameState:       gameState,
		redisService:    gameState,
		db:              db,
		settings:        settings,
	}
}

// ---- Config passthrough (admin dashboard) ----

func (uc *BotUseCase) GetConfig(ctx context.Context) (*domain.BotConfig, error) {
	return uc.botRepo.GetConfig(ctx)
}

func (uc *BotUseCase) UpdateConfig(ctx context.Context, req domain.UpdateBotConfigRequest) (*domain.BotConfig, error) {
	cfg, err := uc.botRepo.GetConfig(ctx)
	if err != nil {
		return nil, err
	}
	if req.Enabled != nil {
		cfg.Enabled = *req.Enabled
	}
	if req.MinRealPlayers != nil {
		if *req.MinRealPlayers < 0 {
			return nil, fmt.Errorf("min_real_players cannot be negative")
		}
		cfg.MinRealPlayers = *req.MinRealPlayers
	}
	if req.TargetBots != nil {
		if *req.TargetBots < 0 {
			return nil, fmt.Errorf("target_bots cannot be negative")
		}
		cfg.TargetBots = *req.TargetBots
	}
	if req.MinimumRoomPlayers != nil {
		if *req.MinimumRoomPlayers < 0 || *req.MinimumRoomPlayers > domain.MaxCardID {
			return nil, fmt.Errorf("minimum_room_players must be between 0 and %d", domain.MaxCardID)
		}
		cfg.MinimumRoomPlayers = *req.MinimumRoomPlayers
	}
	if req.Tiers != nil {
		cfg.Tiers = *req.Tiers
	}
	if req.WinRate != nil {
		if *req.WinRate < 0 || *req.WinRate > 1 {
			return nil, fmt.Errorf("win_rate must be between 0 and 1")
		}
		cfg.WinRate = *req.WinRate
	}
	if req.BiasedDrawMode != nil {
		if !req.BiasedDrawMode.IsValid() {
			return nil, fmt.Errorf("biased_draw_mode must be disabled, legacy, or protected")
		}
		cfg.BiasedDrawMode = *req.BiasedDrawMode
		cfg.BotAlwaysWin = cfg.BiasedDrawMode != domain.BiasedDrawModeDisabled
	} else if req.BotAlwaysWin != nil {
		cfg.BotAlwaysWin = *req.BotAlwaysWin
		if cfg.BotAlwaysWin {
			cfg.BiasedDrawMode = domain.BiasedDrawModeProtected
		} else {
			cfg.BiasedDrawMode = domain.BiasedDrawModeDisabled
		}
	}
	if err := uc.botRepo.UpdateConfig(ctx, cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

// ---- Seeding ----

func (uc *BotUseCase) SeedPool(ctx context.Context, count int) error {
	if count <= 0 {
		count = uc.settings.PoolSize
	}
	return uc.EnsureBotPool(ctx, count)
}

func (uc *BotUseCase) EnsureBotPool(ctx context.Context, size int) error {
	if size <= 0 {
		size = 50
	}
	for i := 1; i <= size; i++ {
		telegramID := int64(botTelegramIDBase - i)

		existing, err := uc.userRepo.FindByTelegramID(ctx, telegramID)
		if err == nil && existing != nil {
			if ferr := uc.fundBotWallet(ctx, existing.ID); ferr != nil {
				return fmt.Errorf("fund existing bot %d: %w", i, ferr)
			}
			continue
		}

		if err := uc.createBot(ctx, i, telegramID); err != nil {
			return fmt.Errorf("create bot %d: %w", i, err)
		}
	}
	return nil
}

func (uc *BotUseCase) createBot(ctx context.Context, index int, telegramID int64) error {
	code, err := uc.uniqueReferralCode(ctx)
	if err != nil {
		return err
	}

	name := botDisplayNames[(index-1)%len(botDisplayNames)]
	phone := fmt.Sprintf("BOT-%08d", index)

	tx, err := uc.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	user := &domain.User{
		ID:          uuid.New(),
		TelegramID:  telegramID,
		FirstName:   name,
		PhoneNumber: phone,
		ReferalCode: code,
		Role:        "user",
		IsBot:       true,
	}
	if err := uc.userRepo.Create(ctx, tx, user); err != nil {
		return err
	}

	floatAmt := uc.settings.WalletFloat
	if floatAmt <= 0 {
		floatAmt = 1000.0
	}

	wallet := &domain.Wallet{UserID: user.ID, Balance: floatAmt}
	if err := uc.walletRepo.Create(ctx, tx, wallet); err != nil {
		return err
	}

	if err := uc.recordBotFunding(ctx, tx, user.ID, floatAmt); err != nil {
		return err
	}

	return tx.Commit()
}

func (uc *BotUseCase) uniqueReferralCode(ctx context.Context) (string, error) {
	for i := 0; i < domain.MaxReferralCodeGenerationAttempts; i++ {
		code, err := referral.GenerateReferralCode()
		if err != nil {
			return "", fmt.Errorf("failed to generate referral code: %w", err)
		}
		if _, err := uc.userRepo.FindByReferralCode(ctx, code); err != nil {
			return code, nil
		}
	}
	return "", fmt.Errorf("failed to generate unique referral code")
}

func (uc *BotUseCase) fundBotWallet(ctx context.Context, userID uuid.UUID) error {
	tx, err := uc.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	wallet, err := uc.walletRepo.LockForUpdate(ctx, tx, userID)
	if err != nil {
		return err
	}
	floatAmt := uc.settings.WalletFloat
	if floatAmt <= 0 {
		floatAmt = 1000.0
	}
	topUp := floatAmt - wallet.Balance
	if topUp <= 0 {
		return tx.Commit()
	}
	if err := uc.walletRepo.UpdateBalance(ctx, tx, userID, topUp); err != nil {
		return err
	}
	if err := uc.recordBotFunding(ctx, tx, userID, topUp); err != nil {
		return err
	}
	return tx.Commit()
}

func (uc *BotUseCase) recordBotFunding(ctx context.Context, tx *sql.Tx, userID uuid.UUID, amount float64) error {
	ref := "BOT_FUNDING"
	return uc.transactionRepo.Create(ctx, tx, &domain.Transaction{
		UserID:    userID,
		Type:      domain.TransactionTypeDeposit,
		Category:  domain.TransactionCategoryBotFunding,
		Amount:    amount,
		Status:    domain.TransactionStatusCompleted,
		Reference: &ref,
	})
}

// ---- Filling (Optimized Bulk Join with Admin Control & Pacing) ----

func (uc *BotUseCase) FillGame(ctx context.Context, gameID uuid.UUID, requested int) (*domain.BotFillResult, error) {
	// Manual admin fill: bypass throttling so admin requests fulfill immediately
	return uc.fill(ctx, gameID, requested, false, false)
}

func (uc *BotUseCase) fill(ctx context.Context, gameID uuid.UUID, requested int, allowEmpty bool, applyPacing bool) (*domain.BotFillResult, error) {
	game, err := uc.gameRepo.FindByID(ctx, gameID)
	if err != nil {
		return nil, fmt.Errorf("game not found: %w", err)
	}
	if game.State != domain.GameStateWaiting && game.State != domain.GameStateCountdown {
		return nil, fmt.Errorf("game is not accepting new players")
	}

	realPlayers, err := uc.botRepo.CountRealPlayersInGame(ctx, gameID)
	if err != nil {
		return nil, err
	}
	botPlayers, err := uc.botRepo.CountBotsInGame(ctx, gameID)
	if err != nil {
		return nil, err
	}

	result := &domain.BotFillResult{GameID: gameID, Requested: requested, RealPlayers: realPlayers, BotPlayers: botPlayers}

	if !allowEmpty && realPlayers < 1 {
		return result, nil
	}

	botCardCounts, err := uc.botCardCountsInGame(ctx, gameID)
	if err != nil {
		return nil, err
	}
	freeCards, err := uc.freeCards(ctx, gameID)
	if err != nil {
		return nil, err
	}

	poolCheckSize := uc.settings.PoolSize * 2
	if poolCheckSize < 100 {
		poolCheckSize = 100
	}
	bots, err := uc.botRepo.ListBots(ctx, poolCheckSize+requested+10)
	if err != nil || len(bots) == 0 {
		_ = uc.EnsureBotPool(ctx, 50)
		bots, err = uc.botRepo.ListBots(ctx, 100)
		if err != nil || len(bots) == 0 {
			return result, fmt.Errorf("no bot accounts available in database")
		}
	}

	type cardAssignment struct {
		userID uuid.UUID
		cardID int
	}
	var assignments []cardAssignment

	added := 0
	for _, bot := range bots {
		if added >= requested || len(freeCards) == 0 {
			break
		}

		for botCardCounts[bot.ID] < maxCardsPerBot && added < requested && len(freeCards) > 0 {
			if err := uc.ensureAffordable(ctx, bot.ID, game.BetAmount); err != nil {
				break
			}

			cardID := freeCards[len(freeCards)-1]
			freeCards = freeCards[:len(freeCards)-1]

			assignments = append(assignments, cardAssignment{userID: bot.ID, cardID: cardID})
			botCardCounts[bot.ID]++
			added++
		}
	}

	if len(assignments) == 0 {
		result.Added = 0
		result.BotPlayers = botPlayers
		return result, nil
	}

	if applyPacing && uc.settings.MaxJoinsPerTick > 0 && len(assignments) > uc.settings.MaxJoinsPerTick {
		assignments = assignments[:uc.settings.MaxJoinsPerTick]
	}

	// ---- BULK TRANSACTION: Insert this batch atomically ----
	tx, err := uc.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to begin bulk join tx: %w", err)
	}
	defer tx.Rollback()

	game, err = uc.gameRepo.LockForUpdate(ctx, tx, gameID)
	if err != nil {
		return nil, fmt.Errorf("failed to lock game for bulk join: %w", err)
	}
	if game.State != domain.GameStateWaiting && game.State != domain.GameStateCountdown {
		return nil, errors.New("game is no longer accepting new players")
	}

	prevPlayerCount := game.PlayerCount

	for _, asm := range assignments {
		player := &domain.GamePlayer{
			ID:           uuid.New(),
			GameID:       gameID,
			UserID:       asm.userID,
			CardID:       asm.cardID,
			Paid:         false,
			IsEliminated: false,
		}
		if err := uc.gameRepo.AddPlayer(ctx, tx, player); err != nil {
			continue
		}
	}

	active, err := uc.gameRepo.GetActivePlayersTx(ctx, tx, gameID)
	if err != nil {
		return nil, fmt.Errorf("failed to read active players: %w", err)
	}
	distinct := make(map[uuid.UUID]bool, len(active))
	for _, p := range active {
		distinct[p.UserID] = true
	}
	game.PlayerCount = len(distinct)
	game.PrizePool = float64(len(active)) * game.BetAmount * (1 - game.HouseCut)

	if err := uc.gameRepo.UpdateTx(ctx, tx, game); err != nil {
		return nil, fmt.Errorf("failed to update game counters: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit bulk join tx: %w", err)
	}

	for _, asm := range assignments {
		uc.redisService.AddPlayer(ctx, gameID, asm.userID)
		uc.redisService.AddTakenCard(ctx, gameID, asm.cardID)
	}
	uc.redisService.SaveGameState(ctx, game)

	if game.State == domain.GameStateWaiting && prevPlayerCount < domain.MinPlayers && game.PlayerCount >= domain.MinPlayers {
		utils.GoSafe("startCountdown", func() { uc.gameUC.startCountdown(context.Background(), gameID) })
	}

	uc.redisService.PublishEvent(ctx, gameID, domain.WebSocketEventPlayerJoined, map[string]interface{}{
		"prize_pool":   game.PrizePool,
		"player_count": game.PlayerCount,
	})

	result.Added = len(assignments)
	result.BotPlayers = botPlayers + len(assignments)
	return result, nil
}

func (uc *BotUseCase) ensureAffordable(ctx context.Context, userID uuid.UUID, bet float64) error {
	wallet, err := uc.walletRepo.FindByUserID(ctx, userID)
	if err == nil && wallet.Balance >= bet {
		return nil
	}
	return uc.fundBotWallet(ctx, userID)
}

func (uc *BotUseCase) botCardCountsInGame(ctx context.Context, gameID uuid.UUID) (map[uuid.UUID]int, error) {
	players, err := uc.gameRepo.GetPlayers(ctx, gameID)
	if err != nil {
		return nil, err
	}
	counts := make(map[uuid.UUID]int)
	for _, p := range players {
		if p != nil && p.IsBot && !p.IsEliminated && p.LeftAt == nil {
			counts[p.UserID]++
		}
	}
	return counts, nil
}

func (uc *BotUseCase) freeCards(ctx context.Context, gameID uuid.UUID) ([]int, error) {
	taken, err := uc.gameRepo.GetTakenCards(ctx, gameID)
	if err != nil {
		return nil, err
	}
	takenSet := make(map[int]bool, len(taken))
	for _, c := range taken {
		takenSet[c] = true
	}
	free := make([]int, 0, domain.MaxCardID)
	for c := domain.MinCardID; c <= domain.MaxCardID; c++ {
		if !takenSet[c] {
			free = append(free, c)
		}
	}
	rand.Shuffle(len(free), func(i, j int) { free[i], free[j] = free[j], free[i] })
	return free, nil
}

// ---- Auto-fill sweeper ----

func (uc *BotUseCase) joinDelayFor(gameID uuid.UUID) time.Duration {
	base := uc.settings.JoinDelay
	if base <= 0 {
		return 0
	}
	spread := float64(gameID[0]) / 256.0
	return base + time.Duration(float64(base)*0.5*spread)
}

// getShiftMultiplier calculates the bot volume multiplier based on 15 equal shifts (96 minutes each)
// synchronized to local time in East Africa Time (EAT / UTC+3, Addis Ababa).
func getShiftMultiplier() float64 {
	loc, err := time.LoadLocation("Africa/Addis_Ababa")
	if err != nil {
		loc = time.FixedZone("EAT", 3*60*60)
	}
	now := time.Now().In(loc)
	totalMinutes := now.Hour()*60 + now.Minute()

	shiftDuration := 96 // 24 hours / 15 shifts = 96 minutes per shift
	shiftIndex := totalMinutes / shiftDuration
	if shiftIndex >= 15 {
		shiftIndex = 14
	}

	multipliers := []float64{
		0.8,  // Shift 1:  00:00 - 01:36
		0.7,  // Shift 2:  01:36 - 03:12
		0.3,  // Shift 3:  03:12 - 04:48
		0.3,  // Shift 4:  04:48 - 06:24
		0.5,  // Shift 5:  06:24 - 08:00
		0.6,  // Shift 6:  08:00 - 09:36
		0.8,  // Shift 7:  09:36 - 11:12
		1.0,  // Shift 8:  11:12 - 12:48
		1.2,  // Shift 9:  12:48 - 14:24 (High volume)
		1.4,  // Shift 10: 14:24 - 16:00 (Peak volume)
		1.4,  // Shift 11: 16:00 - 17:36 (Peak volume)
		1.4,  // Shift 12: 17:36 - 19:12 (Peak volume)
		1.4,  // Shift 13: 19:12 - 20:48 (Peak volume)
		1.4,  // Shift 14: 20:48 - 22:24 (Peak volume)
		1.4,  // Shift 15: 22:24 - 23:59 (Peak volume)
	}

	return multipliers[shiftIndex]
}

// desiredBotCount computes target bots scaled by the 15 time shifts in local time.
func desiredBotCount(realPlayers, minimumRoomPlayers int, hasSpendableBonus, savedPolicyEnabled bool) int {
	if minimumRoomPlayers <= 0 {
		minimumRoomPlayers = domain.MinPlayers
	}

	multiplier := getShiftMultiplier()
	effectiveMinPlayers := int(float64(minimumRoomPlayers) * multiplier)
	if effectiveMinPlayers < domain.MinPlayers {
		effectiveMinPlayers = domain.MinPlayers
	}

	if realPlayers < effectiveMinPlayers {
		return effectiveMinPlayers - realPlayers
	}
	if hasSpendableBonus && savedPolicyEnabled {
		return highPopulationGuardBotCount
	}
	return 0
}

func (uc *BotUseCase) trimBots(ctx context.Context, gameID uuid.UUID, requested int) int {
	if requested <= 0 {
		return 0
	}
	players, err := uc.gameRepo.GetPlayers(ctx, gameID)
	if err != nil {
		return 0
	}

	latestByBot := make(map[uuid.UUID]*domain.GamePlayer)
	for _, p := range players {
		if p == nil || !p.IsBot || p.IsEliminated || p.LeftAt != nil {
			continue
		}
		current := latestByBot[p.UserID]
		if current == nil || p.JoinedAt.After(current.JoinedAt) {
			latestByBot[p.UserID] = p
		}
	}

	bots := make([]*domain.GamePlayer, 0, len(latestByBot))
	for _, p := range latestByBot {
		bots = append(bots, p)
	}
	sort.Slice(bots, func(i, j int) bool {
		if bots[i].JoinedAt.Equal(bots[j].JoinedAt) {
			return bots[i].UserID.String() > bots[j].UserID.String()
		}
		return bots[i].JoinedAt.After(bots[j].JoinedAt)
	})

	removed := 0
	for _, bot := range bots {
		if removed >= requested || ctx.Err() != nil {
			break
		}
		if err := uc.gameUC.LeaveGame(ctx, gameID, domain.LeaveGameRequest{UserID: bot.UserID}); err == nil {
			removed++
		}
	}
	return removed
}

func (uc *BotUseCase) Run(ctx context.Context) {
	ticker := time.NewTicker(uc.settings.CheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			uc.sweep(ctx)
		}
	}
}

func (uc *BotUseCase) sweep(ctx context.Context) {
	cfg, err := uc.botRepo.GetConfig(ctx)
	if err != nil || !cfg.Enabled {
		return
	}

	minRoomPlayers := cfg.MinimumRoomPlayers
	if minRoomPlayers < domain.MinPlayers {
		minRoomPlayers = domain.MinPlayers
	}

	savedPolicyEnabled := cfg.EffectiveBiasedDrawMode() != domain.BiasedDrawModeDisabled
	for _, tier := range cfg.TierList() {
		t := tier
		games, err := uc.gameRepo.FindAvailable(ctx, &t, domain.MaxAvailableGamesLimit)
		if err != nil {
			continue
		}
		for _, game := range games {
			realPlayers, err := uc.botRepo.CountRealPlayersInGame(ctx, game.ID)
			if err != nil {
				continue
			}

			hasSpendableBonus := false
			if realPlayers >= minRoomPlayers && savedPolicyEnabled {
				hasSpendableBonus, err = uc.botRepo.HasSpendableBonusPlayerInGame(ctx, game.ID, game.BetAmount)
				if err != nil {
					continue
				}
			}

			desired := desiredBotCount(realPlayers, minRoomPlayers, hasSpendableBonus, savedPolicyEnabled)
			botPlayers, err := uc.botRepo.CountBotsInGame(ctx, game.ID)
			if err != nil {
				continue
			}

			if botPlayers > desired {
				uc.trimBots(ctx, game.ID, botPlayers-desired)
				continue
			}
			if botPlayers == desired {
				continue
			}

			if realPlayers > 0 && uc.settings.JoinDelay > 0 {
				if delay := uc.joinDelayFor(game.ID); delay > 0 {
					age, hasReal, err := uc.botRepo.SecondsSinceFirstRealPlayer(ctx, game.ID)
					if err != nil || !hasReal || time.Duration(age*float64(time.Second)) < delay {
						continue
					}
				}
			}

			need := desired - botPlayers
			if _, err := uc.fill(ctx, game.ID, need, realPlayers == 0, true); err != nil {
				continue
			}
		}
	}
}