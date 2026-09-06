package handler

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/bingo/backend/internal/domain"
	"github.com/bingo/backend/internal/usecase"
	"github.com/bingo/backend/pkg/telegram"
	"github.com/bingo/backend/pkg/utils"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// TelegramHandler handles Telegram bot webhook updates. The bot is the only way
// users register (it captures the phone number a Mini App cannot read), so this
// is the registration gateway in front of the Mini App.
type TelegramHandler struct {
	userUseCase     *usecase.UserUseCase
	walletUseCase   *usecase.WalletUseCase
	bonusUseCase    *usecase.BonusUseCase
	campaignUseCase *usecase.BonusCampaignUseCase
	promoRepo       domain.PromoRepository
	bot             *telegram.Bot
	webhookSecret   string
	miniAppBase     string // Mini App origin, no trailing slash
	appVersion      string // per-deploy cache-buster (see appURL)
	botUsername     string // @username (no @), for building in-chat invite links

	// depositAccounts is the house number a player pays for each method, shown
	// in-chat when they start a deposit. A method absent here is not offered in
	// chat (no account to pay, and it could never auto-verify).
	depositAccounts map[domain.PaymentMethod]string

	// promoWaiting marks chats whose NEXT message should be treated as a
	// promo-code attempt (set when the user taps the promo menu button).
	// In-memory is fine: worst case after a restart the user just taps the
	// button again. Entries expire after promoWaitTTL.
	promoMu      sync.Mutex
	promoWaiting map[int64]time.Time

	// depositConvos holds the in-chat deposit conversation for each chat
	// (method → amount → transaction number). In-memory for the same reason as
	// promoWaiting: a restart mid-deposit just means the player taps Deposit
	// again. Entries expire after depositConvoTTL.
	depositMu     sync.Mutex
	depositConvos map[int64]*depositConvo

	// withdrawConvos is the analogous in-chat withdraw conversation
	// (method → amount → destination number).
	withdrawMu     sync.Mutex
	withdrawConvos map[int64]*withdrawConvo

	// pendingReferral remembers the referral code from a /start ref_<code> deep
	// link until the invited user finishes registering (shares their phone). In
	// memory like the rest: worst case after a restart the referral link is not
	// credited, which is acceptable.
	referralMu      sync.Mutex
	pendingReferral map[int64]pendingRef
}

// referralTTL is how long a captured invite code waits for the user to finish
// registering before it lapses.
const referralTTL = 30 * time.Minute

type pendingRef struct {
	code     string
	deadline time.Time
}

// promoWaitTTL is how long a "send me your promo code" prompt stays armed.
const promoWaitTTL = 5 * time.Minute

// depositConvoTTL is how long an in-progress deposit stays open before the
// player has to start over. Generous, because they leave the chat to actually
// send the money and come back with the receipt.
const depositConvoTTL = 20 * time.Minute

// depositStep is where a deposit conversation has reached.
type depositStep int

const (
	depAwaitingAmount depositStep = iota // method chosen; waiting for the amount sent
	depAwaitingTxn                       // amount known; waiting for the receipt number
)

// depositConvo is one player's in-flight deposit.
type depositConvo struct {
	step     depositStep
	method   domain.PaymentMethod
	amount   float64
	deadline time.Time
}

// withdrawStep is where a withdraw conversation has reached.
type withdrawStep int

const (
	wdAwaitingAmount  withdrawStep = iota // method chosen; waiting for the amount
	wdAwaitingAccount                     // amount known; waiting for the payout number
)

// withdrawConvo is one player's in-flight withdrawal.
type withdrawConvo struct {
	step     withdrawStep
	method   domain.PaymentMethod
	amount   float64
	deadline time.Time
}

// NewTelegramHandler creates a new Telegram webhook handler.
func NewTelegramHandler(userUseCase *usecase.UserUseCase, walletUseCase *usecase.WalletUseCase, bonusUseCase *usecase.BonusUseCase, campaignUseCase *usecase.BonusCampaignUseCase, promoRepo domain.PromoRepository, bot *telegram.Bot, webhookSecret, miniAppURL, botUsername string, depositAccounts map[domain.PaymentMethod]string) *TelegramHandler {
	base := strings.TrimRight(miniAppURL, "/")
	version := miniAppCacheVersion()
	return &TelegramHandler{
		userUseCase:     userUseCase,
		walletUseCase:   walletUseCase,
		bonusUseCase:    bonusUseCase,
		campaignUseCase: campaignUseCase,
		promoRepo:       promoRepo,
		bot:             bot,
		webhookSecret:   webhookSecret,
		botUsername:     strings.TrimPrefix(strings.TrimSpace(botUsername), "@"),
		depositAccounts: depositAccounts,
		// appURL bakes a per-deploy cache-buster into every Mini App URL.
		// Telegram caches the web app per-URL and ignores our no-cache
		// headers, so without a changing query string players keep seeing the
		// previous build after a deploy. Resolved once at startup (stable
		// within a deploy, so repeated opens still hit Telegram's cache;
		// changes on the next deploy).
		miniAppBase:     base,
		appVersion:      version,
		promoWaiting:    make(map[int64]time.Time),
		depositConvos:   make(map[int64]*depositConvo),
		withdrawConvos:  make(map[int64]*withdrawConvo),
		pendingReferral: make(map[int64]pendingRef),
	}
}

// stashReferral records the invite code from a deep link for a chat, to be
// applied when that user registers.
func (h *TelegramHandler) stashReferral(chatID int64, code string) {
	h.referralMu.Lock()
	defer h.referralMu.Unlock()
	h.pendingReferral[chatID] = pendingRef{code: code, deadline: time.Now().Add(referralTTL)}
}

// takeReferral returns and clears a chat's pending invite code (empty if none
// or expired).
func (h *TelegramHandler) takeReferral(chatID int64) string {
	h.referralMu.Lock()
	defer h.referralMu.Unlock()
	p, ok := h.pendingReferral[chatID]
	if !ok {
		return ""
	}
	delete(h.pendingReferral, chatID)
	if time.Now().After(p.deadline) {
		return ""
	}
	return p.code
}

// startPayload extracts the parameter after "/start" (Telegram delivers a deep
// link t.me/bot?start=X as the message "/start X").
func startPayload(text string) string {
	parts := strings.SplitN(strings.TrimSpace(text), " ", 2)
	if len(parts) < 2 {
		return ""
	}
	return strings.TrimSpace(parts[1])
}

// armPromo marks chatID so its next message is read as a promo code.
func (h *TelegramHandler) armPromo(chatID int64) {
	h.promoMu.Lock()
	defer h.promoMu.Unlock()
	h.promoWaiting[chatID] = time.Now().Add(promoWaitTTL)
}

// disarmPromo reports whether chatID was armed (and clears it).
func (h *TelegramHandler) disarmPromo(chatID int64) bool {
	h.promoMu.Lock()
	defer h.promoMu.Unlock()
	deadline, ok := h.promoWaiting[chatID]
	if !ok {
		return false
	}
	delete(h.promoWaiting, chatID)
	return time.Now().Before(deadline)
}

// appURL builds a deep link into the Mini App (e.g. appURL("/wallet")) with
// the per-deploy cache-buster attached. The SPA serves every path, so a menu
// button can land the player directly on the right screen.
func (h *TelegramHandler) appURL(path string) string {
	return withCacheVersion(h.miniAppBase+path, h.appVersion)
}

// miniAppCacheVersion returns a token that changes on every deploy. On Render
// that's the deployed commit (RENDER_GIT_COMMIT); locally it falls back to the
// process start time, so a restart still busts the cache.
func miniAppCacheVersion() string {
	if c := os.Getenv("RENDER_GIT_COMMIT"); c != "" {
		if len(c) > 8 {
			c = c[:8]
		}
		return c
	}
	return strconv.FormatInt(time.Now().Unix(), 10)
}

// withCacheVersion appends ?v=<version> (or &v= when the URL already has a
// query) so Telegram treats each deploy's URL as new. A blank base or version
// is returned unchanged.
// VersionedMiniAppURL gives persistent Telegram launchers the same per-deploy
// cache-buster used by in-chat Mini App buttons.
func VersionedMiniAppURL(rawURL string) string {
	return withCacheVersion(rawURL, miniAppCacheVersion())
}

func withCacheVersion(rawURL, version string) string {
	if rawURL == "" || version == "" {
		return rawURL
	}
	sep := "?"
	if strings.Contains(rawURL, "?") {
		sep = "&"
	}
	return rawURL + sep + "v=" + version
}

// Webhook handles POST /telegram/webhook — the endpoint registered with
// Telegram via setWebhook. It ALWAYS returns 200 quickly (even on internal
// errors) so Telegram does not retry and back off the bot.
func (h *TelegramHandler) Webhook(c *gin.Context) {
	// Verify the request really came from Telegram via the shared secret.
	if h.webhookSecret != "" {
		if c.GetHeader("X-Telegram-Bot-Api-Secret-Token") != h.webhookSecret {
			c.Status(http.StatusUnauthorized)
			return
		}
	}

	var update telegram.Update
	if err := c.ShouldBindJSON(&update); err != nil {
		// Malformed body — ack so Telegram stops resending, but log it.
		log.Printf("[telegram] failed to parse update: %v", err)
		c.Status(http.StatusOK)
		return
	}

	// An inline-button tap (currently: choosing a deposit method) arrives as a
	// callback_query, not a message.
	if update.CallbackQuery != nil {
		h.handleCallback(c, update.CallbackQuery)
		c.Status(http.StatusOK)
		return
	}

	msg := update.Message
	if msg == nil || msg.From == nil {
		c.Status(http.StatusOK) // nothing actionable (edited message, etc.)
		return
	}

	switch {
	case msg.Contact != nil:
		h.handleContact(c, msg)
	case strings.HasPrefix(msg.Text, "/start"):
		h.handleStart(c, msg)
	default:
		h.handleMenuText(c, msg)
	}

	c.Status(http.StatusOK)
}

// Main-menu button labels — the persistent reply keyboard every registered
// user sees (mirrors the layout in mainMenu). web_app buttons open the Mini
// App directly; plain buttons echo their label back as a message and are
// routed by exact match in handleMenuText.
const (
	btnPlay     = "🎮 ቢንጎ ተጫወት"
	btnPromo    = "🎁 ፕሮሞ ኮድ"
	btnDeposit  = "💰 ገቢ ለማድረግ"
	btnWithdraw = "💸 ወጪ ለማድረግ"
	btnInvite   = "🔗 ጋብዝ & አግኝ"
	btnProfile  = "👤 ፕሮፋይል & ሂሳብ"
	btnHelp     = "🆘 እርዳታ"
	btnLanguage = "🌍 ቋንቋ / Language"
	btnAgent    = "📢 አጀንት ፕሮሞተር"
)

// mainMenu is the persistent reply keyboard (the button grid pinned above the
// system keyboard). All buttons are PLAIN text buttons: Telegram does not pass
// initData to Mini Apps launched from a reply-keyboard web_app button, so the
// app could not authenticate the user ("open inside Telegram" guard). Instead
// each tap is answered instantly with an INLINE web_app button (which does
// carry initData) that opens the Mini App on the matching screen.
func (h *TelegramHandler) mainMenu() *telegram.ReplyMarkup {
	txt := func(text string) telegram.KeyboardButton {
		return telegram.KeyboardButton{Text: text}
	}
	return &telegram.ReplyMarkup{
		Keyboard: [][]telegram.KeyboardButton{
			{txt(btnPlay), txt(btnPromo)},
			{txt(btnDeposit), txt(btnWithdraw)},
			{txt(btnInvite), txt(btnProfile)},
			{txt(btnHelp), txt(btnLanguage)},
			{txt(btnAgent)},
		},
		ResizeKeyboard: true,
		IsPersistent:   true,
	}
}

// appButton replies with one inline web_app button that opens the Mini App at
// path — the only launch method from chat that authenticates the user.
func (h *TelegramHandler) appButton(chatID int64, text, buttonLabel, path string) {
	h.reply(chatID, text, &telegram.ReplyMarkup{
		InlineKeyboard: [][]telegram.InlineKeyboardButton{{
			{Text: buttonLabel, WebApp: &telegram.WebAppInfo{URL: h.appURL(path)}},
		}},
	})
}

// handleMenuText routes taps on the plain (non-web_app) menu buttons, and
// falls back to re-showing the menu for anything unrecognized. Unregistered
// users are funneled into the /start registration flow instead.
func (h *TelegramHandler) handleMenuText(c *gin.Context, msg *telegram.Message) {
	user, err := h.userUseCase.FindUserByTelegramID(c.Request.Context(), msg.From.ID)
	if err != nil || user == nil {
		h.handleStart(c, msg)
		return
	}

	text := strings.TrimSpace(msg.Text)

	// A tap on the promo button arms the chat: the NEXT message is the code.
	if text != btnPromo && h.disarmPromo(msg.Chat.ID) {
		h.redeemPromo(c, msg, user.ID, text)
		return
	}

	// Mid-deposit, a non-menu message is the answer to the current step
	// (amount, then receipt number). A menu button instead cancels the deposit
	// and is handled normally below.
	if convo := h.getDeposit(msg.Chat.ID); convo != nil {
		if isMenuLabel(text) {
			h.clearDeposit(msg.Chat.ID)
		} else {
			h.handleDepositInput(c, msg, user.ID, convo, text)
			return
		}
	}

	// Same for a withdraw in progress (amount, then destination number).
	if convo := h.getWithdraw(msg.Chat.ID); convo != nil {
		if isMenuLabel(text) {
			h.clearWithdraw(msg.Chat.ID)
		} else {
			h.handleWithdrawInput(c, msg, user, convo, text)
			return
		}
	}

	switch text {
	case btnPlay:
		h.appButton(msg.Chat.ID, "🎮 ለመጫወት ከታች ይንኩ 👇", "🎮 ቢንጎ ተጫወት / Play", "/")
	case btnDeposit:
		h.startDeposit(c, msg, user.ID)
	case btnWithdraw:
		h.startWithdraw(c, msg, user.ID)
	case btnInvite:
		h.showInvite(msg.Chat.ID, user)
	case btnProfile:
		h.showBalance(c, msg, user)
	case btnPromo:
		// A tap arms the chat: the NEXT message is read as a promo code.
		h.armPromo(msg.Chat.ID)
		h.reply(msg.Chat.ID,
			"🎁 የፕሮሞ ኮድዎን አሁን ይላኩ 👇\n\nSend your promo code now 👇",
			nil)
	case btnHelp:
		h.reply(msg.Chat.ID,
			"🆘 እርዳታ / Help\n\n"+
				"🎮 ለመጫወት፦ «"+btnPlay+"» ይንኩ፣ ካርድ ይምረጡ — ጨዋታው ካርታዎን በራስ-ሰር ያደምቃል።\n"+
				"💰 ገቢ ለማድረግ፦ በቴሌብር ወይም ኤም-ፔሳ ገንዘብ ልከው ደረሰኙን በመተግበሪያው ያስገቡ።\n"+
				"💸 ወጪ ለማድረግ፦ ከቦርሳ ገጹ ላይ ይጠይቁ።\n\n"+
				"ችግር ካጋጠመዎ ከታች ያለውን ይንኩ 👇",
			&telegram.ReplyMarkup{InlineKeyboard: [][]telegram.InlineKeyboardButton{{
				{Text: "🛟 ችግር ሪፖርት / Report a problem", WebApp: &telegram.WebAppInfo{URL: h.appURL("/report")}},
			}}})
	case btnLanguage:
		h.reply(msg.Chat.ID,
			"🌍 ቋንቋ (አማርኛ / English) በመተግበሪያው ውስጥ ከፕሮፋይል ገጽ መቀየር ይችላሉ።\n\nYou can switch the app language from the Profile page.",
			&telegram.ReplyMarkup{InlineKeyboard: [][]telegram.InlineKeyboardButton{{
				{Text: "👤 ፕሮፋይል / Profile", WebApp: &telegram.WebAppInfo{URL: h.appURL("/profile")}},
			}}})
	case btnAgent:
		h.reply(msg.Chat.ID,
			"📢 አጀንት/ፕሮሞተር ለመሆን ይፈልጋሉ?\n\nበመተግበሪያው ውስጥ «ችግር ሪፖርት» ገጹን ተጠቅመው መልዕክት ይላኩልን — እናገኝዎታለን።",
			&telegram.ReplyMarkup{InlineKeyboard: [][]telegram.InlineKeyboardButton{{
				{Text: "✉️ መልዕክት ላክ / Contact us", WebApp: &telegram.WebAppInfo{URL: h.appURL("/report")}},
			}}})
	default:
		h.reply(msg.Chat.ID,
			"እባክዎ ከታች ካለው ማውጫ ይምረጡ 👇\nPlease choose from the menu below 👇",
			h.mainMenu())
	}
}

// redeemPromo applies a promo code sent after the promo button was tapped and
// answers every outcome distinctly (in Amharic + English).
func (h *TelegramHandler) redeemPromo(c *gin.Context, msg *telegram.Message, userID uuid.UUID, code string) {
	if h.promoRepo == nil {
		h.reply(msg.Chat.ID, "🎁 የፕሮሞ ኮድ ስርዓት በአሁኑ ጊዜ አይገኝም። / Promo codes are currently unavailable.", h.mainMenu())
		return
	}

	amount, err := h.promoRepo.Redeem(c.Request.Context(), code, userID)
	switch {
	case err == nil:
		h.reply(msg.Chat.ID,
			fmt.Sprintf("🎉 እንኳን ደስ አለዎት! %.0f ብር ቦነስ ወደ ቦርሳዎ ተጨምሯል።\n\nCongratulations! A %.0f birr bonus was added to your wallet. 💰", amount, amount),
			h.mainMenu())
	case errors.Is(err, domain.ErrPromoAlreadyRedeemed):
		h.reply(msg.Chat.ID, "ℹ️ ይህን ኮድ ከዚህ በፊት ተጠቅመዋል።\nYou have already used this code.", h.mainMenu())
	case errors.Is(err, domain.ErrPromoExpired), errors.Is(err, domain.ErrPromoInactive):
		h.reply(msg.Chat.ID, "⌛ ይቅርታ፣ የዚህ ኮድ ጊዜ አልፎበታል።\nSorry, this code is no longer valid.", h.mainMenu())
	case errors.Is(err, domain.ErrPromoExhausted):
		h.reply(msg.Chat.ID, "😔 ይቅርታ፣ የዚህ ኮድ ተጠቃሚዎች ብዛት ተሟልቷል።\nSorry, this code has reached its redemption limit.", h.mainMenu())
	case errors.Is(err, domain.ErrPromoNotFound):
		h.reply(msg.Chat.ID, "❌ ኮዱ አልተገኘም። እባክዎ በትክክል መጻፉን ያረጋግጡና «"+btnPromo+"»ን ነክተው እንደገና ይሞክሩ።\nCode not found — check the spelling and try again.", h.mainMenu())
	default:
		log.Printf("[telegram] promo redeem failed for user %s: %v", userID, err)
		h.reply(msg.Chat.ID, "⚠️ የሆነ ችግር ተፈጥሯል፣ እባክዎ ቆየት ብለው ይሞክሩ።\nSomething went wrong — please try again later.", h.mainMenu())
	}
}

// isMenuLabel reports whether text is one of the persistent menu buttons, so a
// deposit conversation knows to yield to a menu tap instead of swallowing it.
func isMenuLabel(text string) bool {
	switch text {
	case btnPlay, btnPromo, btnDeposit, btnWithdraw, btnInvite, btnProfile, btnHelp, btnLanguage, btnAgent:
		return true
	}
	return false
}

// showInvite answers the Invite button IN CHAT with the player's own invite
// link and a one-tap Share button — no Mini App trip needed. Falls back to the
// app only if the link can't be built.
func (h *TelegramHandler) showInvite(chatID int64, user *domain.User) {
	if user.ReferalCode == "" || h.botUsername == "" {
		h.appButton(chatID, "🔗 ጓደኞችዎን ጋብዘው ቦነስ ያግኙ 👇", "🔗 ጋብዝ & አግኝ / Invite", "/referral")
		return
	}
	link := fmt.Sprintf("https://t.me/%s?start=ref_%s", h.botUsername, user.ReferalCode)
	shareText := "🎯 EDL ቢንጎ ተቀላቀሉኝ! / Join me on EDL Bingo!"
	shareURL := "https://t.me/share/url?url=" + url.QueryEscape(link) + "&text=" + url.QueryEscape(shareText)

	msg := fmt.Sprintf(
		"🔗 ጓደኛ ጋብዘው 15 ብር የመጫወቻ ቦነስ ያግኙ!\n"+
			"የጋበዙት ሰው የመጀመሪያ ገቢውን ሲያደርግ 15 ብር የመጫወቻ ቦነስ ያገኛሉ (ካርድ ለመግዛት፤ አሸንፈው ወደ ገንዘብ ይቀይሩት)።\n\n"+
			"የእርስዎ ሊንክ (ለመቅዳት ይንኩት):\n%s\n\n"+
			"Invite a friend, get a 15 birr PLAY bonus when they make their first deposit — play it to win real cash. Tap the link to copy it, or Share below. 👇",
		link)
	h.reply(chatID, msg, &telegram.ReplyMarkup{InlineKeyboard: [][]telegram.InlineKeyboardButton{
		{{Text: "📨 አጋራ / Share", URL: shareURL}},
	}})
}

// showBalance answers the "Profile & balance" button IN CHAT — cash plus
// play-only bonus — instead of opening the Mini App. A read, so no
// conversation state is involved.
func (h *TelegramHandler) showBalance(c *gin.Context, msg *telegram.Message, user *domain.User) {
	ctx := c.Request.Context()
	wallet, err := h.walletUseCase.GetWalletByTelegramID(ctx, msg.From.ID)
	if err != nil || wallet == nil {
		log.Printf("[telegram] balance lookup failed for tg_id=%d: %v", msg.From.ID, err)
		h.reply(msg.Chat.ID, "⚠️ ሂሳብዎን ማምጣት አልተቻለም፣ እባክዎ ቆየት ብለው ይሞክሩ።\nCouldn't load your balance — please try again shortly.", h.mainMenu())
		return
	}

	line := fmt.Sprintf("💰 ቀሪ ሂሳብ / Balance: %.2f ብር", wallet.Balance)
	// Bonus is optional context; never fail the balance reply over it.
	if h.bonusUseCase != nil {
		if bal, berr := h.bonusUseCase.Balance(ctx, user.ID); berr == nil && bal != nil && bal.Amount > 0 {
			line += fmt.Sprintf("\n🎁 ቦነስ / Bonus: %.2f ብር", bal.Amount)
		}
	}
	h.reply(msg.Chat.ID, line, h.mainMenu())
}

// ---- In-chat deposit -------------------------------------------------------

// getDeposit returns the live deposit conversation for a chat, or nil (clearing
// an expired one).
func (h *TelegramHandler) getDeposit(chatID int64) *depositConvo {
	h.depositMu.Lock()
	defer h.depositMu.Unlock()
	convo, ok := h.depositConvos[chatID]
	if !ok {
		return nil
	}
	if time.Now().After(convo.deadline) {
		delete(h.depositConvos, chatID)
		return nil
	}
	return convo
}

func (h *TelegramHandler) setDeposit(chatID int64, convo *depositConvo) {
	convo.deadline = time.Now().Add(depositConvoTTL)
	h.depositMu.Lock()
	h.depositConvos[chatID] = convo
	h.depositMu.Unlock()
}

func (h *TelegramHandler) clearDeposit(chatID int64) {
	h.depositMu.Lock()
	delete(h.depositConvos, chatID)
	h.depositMu.Unlock()
}

// startDeposit answers the Deposit button with an inline method picker. Only
// methods with a configured house account are offered — a method with no
// account has nowhere for the player to pay and could never auto-verify, so it
// falls back to the Mini App wallet instead.
// minDeposit returns the operator-configured minimum deposit, falling back to the
// default if settings can't be read, so a deposit prompt always shows a figure.
func (h *TelegramHandler) minDeposit(ctx context.Context) float64 {
	if s, err := h.walletUseCase.GetSettings(ctx); err == nil && s.MinDeposit > 0 {
		return s.MinDeposit
	}
	return domain.DefaultMinDeposit
}

func (h *TelegramHandler) startDeposit(c *gin.Context, msg *telegram.Message, _ uuid.UUID) {
	// A method an admin has switched off is dropped from the picker, same as a
	// method with no configured house account — the player is never offered a
	// channel they can't actually pay into.
	settings, _ := h.walletUseCase.GetSettings(c.Request.Context())
	var rows [][]telegram.InlineKeyboardButton
	for _, m := range domain.SupportedPaymentMethods {
		if strings.TrimSpace(h.depositAccounts[m]) == "" {
			continue
		}
		if settings != nil && !settings.DepositMethodEnabled(m) {
			continue
		}
		rows = append(rows, []telegram.InlineKeyboardButton{
			{Text: depositMethodLabel(m), CallbackData: "dep:" + string(m)},
		})
	}

	if len(rows) == 0 {
		// Nothing configured for in-chat deposit — keep the old Mini App path.
		h.appButton(msg.Chat.ID, "💰 ገንዘብ ለማስገባት ከታች ይንኩ 👇", "💰 ቦርሳ ክፈት / Open Wallet", "/wallet")
		return
	}
	rows = append(rows, []telegram.InlineKeyboardButton{{Text: "❌ ተወው / Cancel", CallbackData: "dep:cancel"}})

	h.reply(msg.Chat.ID,
		"💰 በየትኛው መንገድ ገቢ ማድረግ ይፈልጋሉ?\nWhich method do you want to deposit with?",
		&telegram.ReplyMarkup{InlineKeyboard: rows})
}

// handleCallback processes an inline-button tap. Every path answers the
// callback so the button stops showing its loading spinner.
func (h *TelegramHandler) handleCallback(c *gin.Context, cq *telegram.CallbackQuery) {
	if cq.From == nil || cq.Message == nil {
		return
	}
	chatID := cq.Message.Chat.ID

	user, err := h.userUseCase.FindUserByTelegramID(c.Request.Context(), cq.From.ID)
	if err != nil || user == nil {
		_ = h.bot.AnswerCallbackQuery(cq.ID, "")
		h.reply(chatID, "እባክዎ በመጀመሪያ /start ይጫኑ።\nPlease tap /start first.", nil)
		return
	}

	data := cq.Data
	switch {
	case data == "dep:cancel":
		h.clearDeposit(chatID)
		_ = h.bot.AnswerCallbackQuery(cq.ID, "ተሰርዟል / Cancelled")
		h.reply(chatID, "እሺ፣ ተሰርዟል።\nOkay, cancelled.", h.mainMenu())

	case strings.HasPrefix(data, "dep:"):
		method := domain.PaymentMethod(strings.TrimPrefix(data, "dep:"))
		account := strings.TrimSpace(h.depositAccounts[method])
		// Re-check availability at tap time: the account must be configured and the
		// admin must not have switched this channel off since the menu was shown.
		settings, _ := h.walletUseCase.GetSettings(c.Request.Context())
		methodDisabled := settings != nil && !settings.DepositMethodEnabled(method)
		if !domain.IsSupportedPaymentMethod(method) || account == "" || methodDisabled {
			_ = h.bot.AnswerCallbackQuery(cq.ID, "")
			h.reply(chatID, "ይህ መንገድ አሁን አይገኝም።\nThat method isn't available right now.", h.mainMenu())
			return
		}
		h.clearWithdraw(chatID) // one wallet action at a time
		h.setDeposit(chatID, &depositConvo{step: depAwaitingAmount, method: method})
		_ = h.bot.AnswerCallbackQuery(cq.ID, "")
		minDep := h.minDeposit(c.Request.Context())
		h.reply(chatID, fmt.Sprintf(
			"✅ %s ተመርጧል።\n\n"+
				"1️⃣ ገንዘቡን ወደዚህ ቁጥር ይላኩ፦\n📱 %s\n\n"+
				"2️⃣ ከዚያ የላኩትን መጠን (በ ብር) ይጻፉ።\n"+
				"⚠️ ዝቅተኛው የማስገቢያ መጠን %.0f ብር ነው።\n\n"+
				"Send the money to %s, then type the amount you sent (in birr).\n"+
				"Minimum deposit is %.0f birr.",
			depositMethodLabel(method), account, minDep, account, minDep), nil)

	case data == "wd:cancel":
		h.clearWithdraw(chatID)
		_ = h.bot.AnswerCallbackQuery(cq.ID, "ተሰርዟል / Cancelled")
		h.reply(chatID, "እሺ፣ ተሰርዟል።\nOkay, cancelled.", h.mainMenu())

	case data == "wd:self":
		// "Use my registered number" — finalise with a blank account, which the
		// use case resolves to the player's verified registration phone.
		convo := h.getWithdraw(chatID)
		if convo == nil || convo.step != wdAwaitingAccount {
			_ = h.bot.AnswerCallbackQuery(cq.ID, "")
			return
		}
		h.clearWithdraw(chatID)
		_ = h.bot.AnswerCallbackQuery(cq.ID, "")
		h.doWithdraw(c, msg2From(cq), user.ID, convo.method, convo.amount, "")

	case strings.HasPrefix(data, "wd:"):
		method := domain.PaymentMethod(strings.TrimPrefix(data, "wd:"))
		if !domain.IsSupportedPaymentMethod(method) {
			_ = h.bot.AnswerCallbackQuery(cq.ID, "")
			h.reply(chatID, "ይህ መንገድ አሁን አይገኝም።\nThat method isn't available right now.", h.mainMenu())
			return
		}
		h.clearDeposit(chatID) // one wallet action at a time
		h.setWithdraw(chatID, &withdrawConvo{step: wdAwaitingAmount, method: method})
		_ = h.bot.AnswerCallbackQuery(cq.ID, "")
		bal := 0.0
		if w, werr := h.walletUseCase.GetWalletByTelegramID(c.Request.Context(), cq.From.ID); werr == nil && w != nil {
			bal = w.Balance
		}
		h.reply(chatID, fmt.Sprintf(
			"✅ %s ተመርጧል።\n\nምን ያህል ማውጣት ይፈልጋሉ? (በ ብር)\nዝቅተኛ %.0f ብር · ያለዎት %.2f ብር\n\nHow much do you want to withdraw? (min %.0f, you have %.2f)",
			depositMethodLabel(method), domain.MinWithdrawalAmount, bal, domain.MinWithdrawalAmount, bal), nil)

	case data == "bonus:claim":
		// The claim itself is the ONLY guard against a double claim: the use
		// case (and its unique constraint) reject a second attempt, so even a
		// double-tapped button can never pay twice — the second returns
		// ErrCampaignAlreadyClaimed, reported as "already claimed".
		if h.campaignUseCase == nil {
			_ = h.bot.AnswerCallbackQuery(cq.ID, "")
			h.reply(chatID, "🎁 አሁን ምንም ቦነስ የለም።\nNo bonus right now.", h.mainMenu())
			return
		}
		claim, cerr := h.campaignUseCase.Claim(c.Request.Context(), user.ID)
		if cerr != nil {
			_ = h.bot.AnswerCallbackQuery(cq.ID, "")
			h.reply(chatID, claimErrorMessage(cerr), h.mainMenu())
			return
		}
		_ = h.bot.AnswerCallbackQuery(cq.ID, "🎉")
		h.reply(chatID, fmt.Sprintf(
			"🎉 እንኳን ደስ አለዎት! %.0f ብር ቦነስ አግኝተዋል።\nCongratulations! You got %.0f birr bonus. 💰",
			claim.Amount, claim.Amount), h.mainMenu())

	default:
		_ = h.bot.AnswerCallbackQuery(cq.ID, "")
	}
}

// msg2From adapts a callback's sender into the *telegram.Message shape doWithdraw
// needs for its follow-up balance read (which keys on the sender's Telegram ID).
func msg2From(cq *telegram.CallbackQuery) *telegram.Message {
	return &telegram.Message{From: cq.From, Chat: cq.Message.Chat}
}

// handleDepositInput consumes the player's typed answer to the current deposit
// step: first the amount, then the receipt number (which triggers the actual
// deposit).
func (h *TelegramHandler) handleDepositInput(c *gin.Context, msg *telegram.Message, userID uuid.UUID, convo *depositConvo, text string) {
	switch convo.step {
	case depAwaitingAmount:
		amount, err := strconv.ParseFloat(strings.TrimSpace(text), 64)
		if err != nil || amount <= 0 {
			h.reply(msg.Chat.ID, "❓ እባክዎ የላኩትን መጠን በቁጥር ይጻፉ (ለምሳሌ 100)።\nPlease type the amount you sent as a number (e.g. 100).", nil)
			return
		}
		if minDep := h.minDeposit(c.Request.Context()); amount < minDep {
			h.reply(msg.Chat.ID, fmt.Sprintf(
				"⚠️ ዝቅተኛው የማስገቢያ መጠን %.0f ብር ነው። እባክዎ %.0f ብር ወይም ከዚያ በላይ ያስገቡ።\n\nThe minimum deposit is %.0f birr. Please enter %.0f or more.",
				minDep, minDep, minDep, minDep), nil)
			return
		}
		convo.amount = amount
		convo.step = depAwaitingTxn
		h.setDeposit(msg.Chat.ID, convo) // refresh the TTL
		h.reply(msg.Chat.ID, fmt.Sprintf(
			"👍 %.2f ብር።\n\nአሁን የክፍያውን ደረሰኝ ቁጥር (transaction/receipt number) ይጻፉ።\n\nNow paste the transaction / receipt number from your payment.",
			amount), nil)

	case depAwaitingTxn:
		h.clearDeposit(msg.Chat.ID)
		h.doDeposit(c, msg, userID, convo.method, convo.amount, text)
	}
}

// doDeposit runs the deposit through the SAME use case the Mini App uses —
// duplicate-reference guard, external verification, auto-credit or pending —
// and reports the outcome in chat.
func (h *TelegramHandler) doDeposit(c *gin.Context, msg *telegram.Message, userID uuid.UUID, method domain.PaymentMethod, amount float64, txnID string) {
	tx, err := h.walletUseCase.Deposit(c.Request.Context(), domain.DepositRequest{
		UserID:          userID,
		Amount:          amount,
		TransactionType: method,
		TransactionID:   txnID,
	})
	if err != nil {
		h.reply(msg.Chat.ID, depositErrorMessage(err), h.mainMenu())
		return
	}

	// A verified deposit is auto-approved and already in the wallet; anything
	// else is left pending for an admin. Report each honestly.
	if tx != nil && tx.Status == domain.TransactionStatusCompleted {
		msgText := fmt.Sprintf("✅ ተሳክቷል! %.2f ብር ወደ ሂሳብዎ ተጨምሯል።\nDone! %.2f birr added to your balance.", tx.Amount, tx.Amount)
		if wallet, werr := h.walletUseCase.GetWalletByTelegramID(c.Request.Context(), msg.From.ID); werr == nil && wallet != nil {
			msgText += fmt.Sprintf("\n\n💰 ቀሪ ሂሳብ / Balance: %.2f ብር", wallet.Balance)
		}
		h.reply(msg.Chat.ID, msgText, h.mainMenu())
		return
	}
	h.reply(msg.Chat.ID,
		"🕓 ደረሰኝዎ ተቀብለናል። በአስተዳዳሪ ማረጋገጫ በኋላ ሂሳብዎ ይሞላል።\nWe received your receipt — it'll be credited after admin review.",
		h.mainMenu())
}

// depositMethodLabel is the button/label text for a payment method.
func depositMethodLabel(m domain.PaymentMethod) string {
	switch m {
	case domain.PaymentMethodTelebirr:
		return "💰 Telebirr"
	case domain.PaymentMethodCBEBirr:
		return "🏦 CBE Birr"
	case domain.PaymentMethodMpesa:
		return "📱 M-Pesa"
	default:
		return string(m)
	}
}

// depositErrorMessage turns a Deposit error into a player-facing bilingual
// line, singling out the duplicate-receipt case that a user can actually act on.
func depositErrorMessage(err error) string {
	m := strings.ToLower(err.Error())
	switch {
	case strings.Contains(m, "already") || strings.Contains(m, "duplicate"):
		return "⚠️ ይህ ደረሰኝ ቁጥር ቀድሞ ጥቅም ላይ ውሏል።\nThis receipt number has already been used."
	case strings.Contains(m, "requested amount"):
		// The receipt is real but the amount typed doesn't match what was paid.
		return "⚠️ የጻፉት መጠን ከደረሰኙ ጋር አይመሳሰልም። እባክዎ በትክክል የላኩትን መጠን አስገብተው እንደገና ይሞክሩ።\nThe amount you typed doesn't match the receipt — re-enter the exact amount you sent."
	case strings.Contains(m, "provider"):
		return "⚠️ የመረጡት መንገድ ከደረሰኙ ጋር አይመሳሰልም።\nThat receipt is from a different payment method than the one you picked."
	case strings.Contains(m, "verif") || strings.Contains(m, "not found") || strings.Contains(m, "match"):
		return "❌ ክፍያውን ማረጋገጥ አልተቻለም። ቁጥሩን ያረጋግጡና እንደገና ይሞክሩ።\nWe couldn't verify that payment — check the number and try again."
	default:
		return "⚠️ ገቢ ማድረግ አልተሳካም፣ እባክዎ እንደገና ይሞክሩ።\nDeposit failed — please try again."
	}
}

// ---- In-chat withdraw ------------------------------------------------------

func (h *TelegramHandler) getWithdraw(chatID int64) *withdrawConvo {
	h.withdrawMu.Lock()
	defer h.withdrawMu.Unlock()
	convo, ok := h.withdrawConvos[chatID]
	if !ok {
		return nil
	}
	if time.Now().After(convo.deadline) {
		delete(h.withdrawConvos, chatID)
		return nil
	}
	return convo
}

func (h *TelegramHandler) setWithdraw(chatID int64, convo *withdrawConvo) {
	convo.deadline = time.Now().Add(depositConvoTTL)
	h.withdrawMu.Lock()
	h.withdrawConvos[chatID] = convo
	h.withdrawMu.Unlock()
}

func (h *TelegramHandler) clearWithdraw(chatID int64) {
	h.withdrawMu.Lock()
	delete(h.withdrawConvos, chatID)
	h.withdrawMu.Unlock()
}

// startWithdraw answers the Withdraw button with a method picker. All methods
// are offered (unlike deposit): the payout goes to the player's OWN phone, so
// no house account needs configuring.
func (h *TelegramHandler) startWithdraw(c *gin.Context, msg *telegram.Message, _ uuid.UUID) {
	var rows [][]telegram.InlineKeyboardButton
	for _, m := range domain.SupportedPaymentMethods {
		rows = append(rows, []telegram.InlineKeyboardButton{
			{Text: depositMethodLabel(m), CallbackData: "wd:" + string(m)},
		})
	}
	rows = append(rows, []telegram.InlineKeyboardButton{{Text: "❌ ተወው / Cancel", CallbackData: "wd:cancel"}})

	h.reply(msg.Chat.ID,
		"💸 ገንዘብዎ በየትኛው መንገድ እንዲላክ ይፈልጋሉ?\nWhich method should we send your money to?",
		&telegram.ReplyMarkup{InlineKeyboard: rows})
}

// handleWithdrawInput consumes the player's typed answer: first the amount,
// then the destination number (a typed number finalises the withdrawal; the
// "use my number" button does so via the callback path).
func (h *TelegramHandler) handleWithdrawInput(c *gin.Context, msg *telegram.Message, user *domain.User, convo *withdrawConvo, text string) {
	switch convo.step {
	case wdAwaitingAmount:
		amount, err := strconv.ParseFloat(strings.TrimSpace(text), 64)
		if err != nil || amount <= 0 {
			h.reply(msg.Chat.ID, "❓ እባክዎ መጠኑን በቁጥር ይጻፉ (ለምሳሌ 100)።\nPlease type the amount as a number (e.g. 100).", nil)
			return
		}
		if amount < domain.MinWithdrawalAmount {
			h.reply(msg.Chat.ID, fmt.Sprintf("ዝቅተኛው የማውጣት መጠን %.0f ብር ነው።\nThe minimum withdrawal is %.0f birr.", domain.MinWithdrawalAmount, domain.MinWithdrawalAmount), nil)
			return
		}
		convo.amount = amount
		convo.step = wdAwaitingAccount
		h.setWithdraw(msg.Chat.ID, convo)

		// Offer the registered phone as a one-tap default; typing a number
		// overrides it.
		selfBtn := [][]telegram.InlineKeyboardButton{}
		if utils.IsEthiopianMobile(user.PhoneNumber) {
			selfBtn = append(selfBtn, []telegram.InlineKeyboardButton{
				{Text: "📱 የተመዘገብኩበት ቁጥር / Use my number", CallbackData: "wd:self"},
			})
		}
		h.reply(msg.Chat.ID,
			"ወደ የትኛው ቁጥር እንላክ? ቁጥሩን ይጻፉ፣ ወይም ከታች ይንኩ።\n\nWhich number should we send it to? Type the number, or tap below.",
			&telegram.ReplyMarkup{InlineKeyboard: selfBtn})

	case wdAwaitingAccount:
		h.clearWithdraw(msg.Chat.ID)
		h.doWithdraw(c, msg, user.ID, convo.method, convo.amount, strings.TrimSpace(text))
	}
}

// doWithdraw runs the withdrawal through the SAME use case the Mini App uses —
// min/daily-cap/deposit-gate checks, phone validation, balance debit — and
// reports the outcome. A blank account resolves to the registration phone.
func (h *TelegramHandler) doWithdraw(c *gin.Context, msg *telegram.Message, userID uuid.UUID, method domain.PaymentMethod, amount float64, account string) {
	tx, err := h.walletUseCase.Withdraw(c.Request.Context(), domain.WithdrawRequest{
		UserID:        userID,
		Amount:        amount,
		AccountNumber: account,
		AccountType:   method,
	})
	if err != nil {
		h.reply(msg.Chat.ID, withdrawErrorMessage(err), h.mainMenu())
		return
	}

	// The balance is debited immediately; the payout itself is pending admin
	// approval. Show the new balance so the debit is visible.
	line := fmt.Sprintf("✅ ጥያቄዎ ደርሷል! %.2f ብር እየተላከ ነው (በአስተዳዳሪ ማረጋገጫ በኋላ)።\nRequest received! %.2f birr is on its way (after admin approval).", amount, amount)
	if tx != nil {
		if wallet, werr := h.walletUseCase.GetWalletByTelegramID(c.Request.Context(), msg.From.ID); werr == nil && wallet != nil {
			line += fmt.Sprintf("\n\n💰 ቀሪ ሂሳብ / Balance: %.2f ብር", wallet.Balance)
		}
	}
	h.reply(msg.Chat.ID, line, h.mainMenu())
}

// withdrawErrorMessage maps a Withdraw error to a player-facing bilingual line.
func withdrawErrorMessage(err error) string {
	m := strings.ToLower(err.Error())
	switch {
	case strings.Contains(m, "insufficient"):
		return "⚠️ በቂ ቀሪ ሂሳብ የለዎትም።\nYou don't have enough balance."
	case strings.Contains(m, "remaining balance"):
		return fmt.Sprintf("⚠️ ቢያንስ %.0f ብር በሂሳብዎ መቅረት አለበት።\nYou must keep at least %.0f birr in your wallet.", domain.MinBalanceAfterWithdrawal, domain.MinBalanceAfterWithdrawal)
	case strings.Contains(m, "completed deposit"):
		return "⚠️ ገንዘብ ለማውጣት በመጀመሪያ ቢያንስ አንድ ጊዜ ገቢ ማድረግ አለብዎት።\nYou must make at least one deposit before you can withdraw."
	case strings.Contains(m, "minimum"):
		return fmt.Sprintf("ዝቅተኛው የማውጣት መጠን %.0f ብር ነው።\nThe minimum withdrawal is %.0f birr.", domain.MinWithdrawalAmount, domain.MinWithdrawalAmount)
	case strings.Contains(m, "phone") || strings.Contains(m, "account"):
		return "⚠️ ትክክለኛ የኢትዮጵያ ስልክ ቁጥር ያስገቡ (ለምሳሌ 0912345678)።\nPlease enter a valid Ethiopian phone number (e.g. 0912345678)."
	default:
		return "⚠️ ወጪ ማድረግ አልተሳካም፣ እባክዎ እንደገና ይሞክሩ።\nWithdrawal failed — please try again."
	}
}

// claimErrorMessage maps a campaign claim refusal to a bilingual line. The
// already-claimed case is what enforces "no double claim" for the player.
func claimErrorMessage(err error) string {
	switch {
	case errors.Is(err, domain.ErrCampaignAlreadyClaimed):
		return "ℹ️ የዛሬውን ቦነስ ቀድሞ ወስደዋል።\nYou already claimed today's bonus."
	case errors.Is(err, domain.ErrCampaignExhausted):
		return "😔 ይቅርታ፣ ቦታዎቹ አልቀዋል። ነገ ይሞክሩ።\nSorry, the bonus is finished — try again tomorrow."
	case errors.Is(err, domain.ErrCampaignNotEligible):
		return "ℹ️ ቦነሱን ለመውሰድ በመጀመሪያ አንድ ጊዜ ገቢ ማድረግ አለብዎት።\nDeposit once to unlock the bonus."
	case errors.Is(err, domain.ErrNoActiveCampaign):
		return "🎁 አሁን ምንም ቦነስ የለም።\nNo bonus running right now."
	default:
		return "⚠️ ቦነስ መውሰድ አልተሳካም፣ እባክዎ እንደገና ይሞክሩ።\nCouldn't claim — please try again."
	}
}

// handleStart greets the user. If already registered, it shows the persistent
// main menu straight away; otherwise it asks them to share their phone number.
func (h *TelegramHandler) handleStart(c *gin.Context, msg *telegram.Message) {
	// Capture an invite code from the deep link (/start ref_<code>) so it can be
	// applied when this person finishes registering below.
	if payload := startPayload(msg.Text); payload != "" {
		if code := strings.TrimPrefix(payload, "ref_"); code != "" {
			h.stashReferral(msg.Chat.ID, code)
		}
	}

	if user, err := h.userUseCase.FindUserByTelegramID(c.Request.Context(), msg.From.ID); err == nil && user != nil {
		h.reply(msg.Chat.ID,
			"እንኳን ደህና መጡ፣ "+user.FirstName+"! 🎉\nከታች ያለውን ማውጫ ይጠቀሙ 👇\n\nWelcome back! Use the menu below 👇",
			h.mainMenu())
		return
	}

	h.reply(msg.Chat.ID,
		"Welcome to Edl Bingo! · እንኳን ወደ እድል ቢንጎ በደህና መጡ! 🎯\n\nTo create your account, tap the button below to share your phone number.",
		telegram.ContactRequestKeyboard("📱 Share my phone number"))
}

// handleContact registers the user from their shared contact, then shows Play.
func (h *TelegramHandler) handleContact(c *gin.Context, msg *telegram.Message) {
	contact := msg.Contact

	// Only accept the user's OWN contact, not one forwarded from someone else.
	if contact.UserID != 0 && contact.UserID != msg.From.ID {
		h.reply(msg.Chat.ID, "Please share *your own* phone number using the button.", nil)
		return
	}

	if !utils.IsEthiopianMobile(contact.PhoneNumber) {
		h.reply(msg.Chat.ID, "Sorry, only Ethiopian phone numbers (+251) are supported.", nil)
		return
	}

	var lastName *string
	if ln := strings.TrimSpace(msg.From.LastName); ln != "" {
		lastName = &ln
	}

	_, _, err := h.userUseCase.CreateUser(c.Request.Context(), domain.CreateUserRequest{
		TelegramID:   msg.From.ID,
		FirstName:    msg.From.FirstName,
		LastName:     lastName,
		Phone:        contact.PhoneNumber,
		ReferrerCode: h.takeReferral(msg.Chat.ID),
	})

	// Treat "already registered" as success — the user just wants to play.
	if err != nil && !isAlreadyRegistered(err) {
		log.Printf("[telegram] register failed for tg_id=%d: %v", msg.From.ID, err)
		h.reply(msg.Chat.ID, "Something went wrong creating your account. Please try /start again.", nil)
		return
	}

	h.reply(msg.Chat.ID,
		"ምዝገባዎ ተጠናቋል! 🎉 ከታች ያለውን ማውጫ ይጠቀሙ 👇\n\nYou're all set! Use the menu below 👇",
		h.mainMenu())
}

// reply sends a message and logs (but swallows) any send error.
func (h *TelegramHandler) reply(chatID int64, text string, markup *telegram.ReplyMarkup) {
	if err := h.bot.SendMessage(chatID, text, markup); err != nil {
		log.Printf("[telegram] sendMessage to chat %d failed: %v", chatID, err)
	}
}

// isAlreadyRegistered reports whether err is a duplicate-user error from
// CreateUser (same telegram ID or phone already exists).
func isAlreadyRegistered(err error) bool {
	return strings.Contains(err.Error(), "already exists")
}
