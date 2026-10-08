package bot

import (
	"database/sql"
	"fmt"
	"log"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type AgentBotService struct {
	bot         *tgbotapi.BotAPI
	db          *sql.DB
	mainBotName string // Main Bingo Bot username (where players register)
}

func NewAgentBotService(token string, db *sql.DB, mainBotName string) (*AgentBotService, error) {
	bot, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		return nil, fmt.Errorf("failed to create agent bot: %w", err)
	}

	// Sanitize mainBotName: Remove '@' and spaces
	cleanMainBotName := strings.TrimPrefix(strings.TrimSpace(mainBotName), "@")

	log.Printf("Agent Bot authorized as @%s (Targeting Main Bot: @%s)", bot.Self.UserName, cleanMainBotName)
	return &AgentBotService{
		bot:         bot,
		db:          db,
		mainBotName: cleanMainBotName,
	}, nil
}

func (s *AgentBotService) Start() {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := s.bot.GetUpdatesChan(u)

	for update := range updates {
		if update.Message == nil {
			continue
		}

		go s.handleMessage(update.Message)
	}
}

func (s *AgentBotService) handleMessage(msg *tgbotapi.Message) {
	telegramID := msg.From.ID

	// Verify or auto-register agent
	agentID, refCode, err := s.getOrCreateAgent(telegramID)
	if err != nil {
		s.reply(msg.Chat.ID, "❌ Account error. Please contact admin support.")
		log.Printf("Error fetching agent %d: %v", telegramID, err)
		return
	}

	// Always fetch live balance directly from DB
	balance := s.getAgentBalance(agentID)

	switch msg.Command() {
	case "start":
		text := fmt.Sprintf(
			"👋 **Welcome to the Genzeb Agent Portal!**\n\n"+
				"🆔 Agent Code: `%s`\n"+
				"💰 Wallet Balance: **%.2f ETB**\n\n"+
				"Commands:\n"+
				"/link - Get your player invitation link\n"+
				"/stats - View performance & active players\n"+
				"/withdraw - Cash out your earnings",
			refCode, balance,
		)
		s.replyMarkdown(msg.Chat.ID, text)

	case "link":
		// Direct deep link to main player bot
		link := fmt.Sprintf("https://t.me/%s?start=ref_%s", s.mainBotName, refCode)
		text := fmt.Sprintf(
			"🔗 **Your Unique Referral Link:**\n\n"+
				"`%s`\n\n"+
				"Share this link with players! You earn commissions every time they play a round.",
			link,
		)
		s.replyMarkdown(msg.Chat.ID, text)

	case "stats":
		s.handleStats(msg.Chat.ID, agentID)

	case "withdraw":
		s.handleWithdrawRequest(msg.Chat.ID, agentID, balance)

	default:
		s.reply(msg.Chat.ID, "Use /link, /stats, or /withdraw to manage your agent account.")
	}
}

func (s *AgentBotService) getOrCreateAgent(telegramID int64) (string, string, error) {
	var id, refCode string

	// 1. Fetch existing agent
	query := `SELECT id, referral_code FROM agents WHERE telegram_id = $1`
	err := s.db.QueryRow(query, telegramID).Scan(&id, &refCode)

	if err == nil {
		return id, refCode, nil
	}

	if err != sql.ErrNoRows {
		return "", "", err
	}

	// 2. Auto-generate referral code for new agent
	refCode = fmt.Sprintf("AG%d", telegramID%1000000)
	insertQuery := `
		INSERT INTO agents (telegram_id, referral_code, balance)
		VALUES ($1, $2, 0.00)
		RETURNING id, referral_code`

	err = s.db.QueryRow(insertQuery, telegramID, refCode).Scan(&id, &refCode)
	if err != nil {
		return "", "", err
	}

	return id, refCode, nil
}

func (s *AgentBotService) getAgentBalance(agentID string) float64 {
	var balance float64
	_ = s.db.QueryRow(`SELECT COALESCE(balance, 0.00) FROM agents WHERE id = $1`, agentID).Scan(&balance)
	return balance
}

func (s *AgentBotService) handleStats(chatID int64, agentID string) {
	var totalPlayers int
	var todayCommissions sql.NullFloat64
	var lifetimeCommissions sql.NullFloat64

	// Count referred real players
	_ = s.db.QueryRow(`
		SELECT COUNT(*) FROM users 
		WHERE agent_id = $1 AND is_bot = false`, agentID,
	).Scan(&totalPlayers)

	// Today's commissions
	_ = s.db.QueryRow(`
		SELECT COALESCE(SUM(amount), 0.00) FROM agent_commissions 
		WHERE agent_id = $1 AND created_at >= CURRENT_DATE`, agentID,
	).Scan(&todayCommissions)

	// Lifetime commissions
	_ = s.db.QueryRow(`
		SELECT COALESCE(SUM(amount), 0.00) FROM agent_commissions 
		WHERE agent_id = $1`, agentID,
	).Scan(&lifetimeCommissions)

	text := fmt.Sprintf(
		"📊 **Agent Performance Dashboard**\n\n"+
			"👥 Total Players Referred: **%d**\n"+
			"📈 Today's Earnings: **%.2f ETB**\n"+
			"💎 Lifetime Earnings: **%.2f ETB**",
		totalPlayers, todayCommissions.Float64, lifetimeCommissions.Float64,
	)
	s.replyMarkdown(chatID, text)
}

func (s *AgentBotService) handleWithdrawRequest(chatID int64, agentID string, balance float64) {
	// Rule 1: Sunday-only withdrawals check
	if time.Now().Weekday() != time.Sunday {
		s.reply(chatID, "🗓 Withdrawals are only processed on Sundays. Please check back then!")
		return
	}

	// Rule 2: Minimum balance threshold
	minWithdrawal := 100.00
	if balance < minWithdrawal {
		s.reply(chatID, fmt.Sprintf("⚠️ Minimum withdrawal is %.2f ETB. Your current balance is %.2f ETB.", minWithdrawal, balance))
		return
	}

	tx, err := s.db.Begin()
	if err != nil {
		s.reply(chatID, "❌ Processing error. Please try again later.")
		return
	}
	defer tx.Rollback()

	// Deduct balance from agent wallet
	_, err = tx.Exec(`UPDATE agents SET balance = balance - $1 WHERE id = $2`, balance, agentID)
	if err != nil {
		s.reply(chatID, "Failed to update agent balance.")
		return
	}

	// Log transaction linked to agent
	_, err = tx.Exec(`
		INSERT INTO transactions (id, user_id, type, category, amount, status)
		VALUES (gen_random_uuid(), NULL, 'withdrawal', 'agent_payout', $1, 'pending')`,
		balance,
	)
	if err != nil {
		s.reply(chatID, "Failed to log withdrawal record.")
		return
	}

	if err := tx.Commit(); err != nil {
		s.reply(chatID, "Failed to complete request.")
		return
	}

	text := fmt.Sprintf("✅ **Withdrawal Requested!**\n\nAmount: **%.2f ETB**\nStatus: Pending Admin Approval", balance)
	s.replyMarkdown(chatID, text)
}

func (s *AgentBotService) reply(chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	_, _ = s.bot.Send(msg)
}

func (s *AgentBotService) replyMarkdown(chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = "Markdown"
	_, _ = s.bot.Send(msg)
}