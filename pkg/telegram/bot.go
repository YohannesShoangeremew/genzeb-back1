package telegram

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// botAPIBase is the Telegram Bot API root. Each call is botAPIBase/bot<token>/<method>.
const botAPIBase = "https://api.telegram.org"

// Bot is a minimal Telegram Bot API client — just enough to send messages and
// keyboards from the webhook handler.
type Bot struct {
	token  string
	client *http.Client
}

// NewBot creates a Bot API client bound to the given bot token.
func NewBot(token string) *Bot {
	return &Bot{
		token:  token,
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

// ---- Incoming update types (only the fields we use) ----

// Update is one entry from a webhook POST body.
type Update struct {
	UpdateID      int64          `json:"update_id"`
	Message       *Message       `json:"message"`
	CallbackQuery *CallbackQuery `json:"callback_query"`
}

// CallbackQuery is the payload when a user taps an inline button carrying
// callback_data (as opposed to a web_app button, which opens the Mini App).
// It is how the in-chat deposit flow reads a method choice.
type CallbackQuery struct {
	ID      string       `json:"id"`
	From    *MessageUser `json:"from"`
	Message *Message     `json:"message"` // the message the button was attached to
	Data    string       `json:"data"`    // the callback_data of the tapped button
}

// Message is a Telegram message. Contact is set when the user shares a phone.
type Message struct {
	MessageID int64        `json:"message_id"`
	From      *MessageUser `json:"from"`
	Chat      Chat         `json:"chat"`
	Text      string       `json:"text"`
	Contact   *Contact     `json:"contact"`
}

// MessageUser is the sender of a message.
type MessageUser struct {
	ID        int64  `json:"id"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Username  string `json:"username"`
}

// Chat identifies where to reply.
type Chat struct {
	ID int64 `json:"id"`
}

// Contact is the payload when a user taps a request_contact button.
type Contact struct {
	PhoneNumber string `json:"phone_number"`
	FirstName   string `json:"first_name"`
	LastName    string `json:"last_name"`
	UserID      int64  `json:"user_id"`
}

// ---- Outgoing reply markup ----

// ReplyMarkup is either a custom keyboard (with request_contact) or an inline
// keyboard (with a web_app button). Unused fields are omitted.
type ReplyMarkup struct {
	Keyboard        [][]KeyboardButton       `json:"keyboard,omitempty"`
	InlineKeyboard  [][]InlineKeyboardButton `json:"inline_keyboard,omitempty"`
	ResizeKeyboard  bool                     `json:"resize_keyboard,omitempty"`
	OneTimeKeyboard bool                     `json:"one_time_keyboard,omitempty"`
	// IsPersistent keeps the custom keyboard always visible (the client shows
	// it instead of hiding it after one use) — the "bot main menu" pattern.
	IsPersistent bool `json:"is_persistent,omitempty"`
}

// KeyboardButton is a button on the custom reply keyboard. Exactly one of the
// optional fields may be set: RequestContact asks for the user's phone;
// WebApp opens a Mini App directly (private chats only). A plain button just
// echoes Text back to the bot as a message.
type KeyboardButton struct {
	Text           string      `json:"text"`
	RequestContact bool        `json:"request_contact,omitempty"`
	WebApp         *WebAppInfo `json:"web_app,omitempty"`
}

// InlineKeyboardButton is a button under the message. Exactly one action field
// is set: WebApp opens a Mini App; CallbackData sends that string back to the
// webhook as a CallbackQuery (a tap the bot handles in-chat, no Mini App); URL
// opens a link (e.g. a t.me/share/url share dialog).
type InlineKeyboardButton struct {
	Text         string      `json:"text"`
	WebApp       *WebAppInfo `json:"web_app,omitempty"`
	CallbackData string      `json:"callback_data,omitempty"`
	URL          string      `json:"url,omitempty"`
}

// WebAppInfo points an inline button at a Mini App URL.
type WebAppInfo struct {
	URL string `json:"url"`
}

// ContactRequestKeyboard returns a one-time keyboard with a single button that
// asks the user to share their phone number.
func ContactRequestKeyboard(buttonText string) *ReplyMarkup {
	return &ReplyMarkup{
		Keyboard:        [][]KeyboardButton{{{Text: buttonText, RequestContact: true}}},
		ResizeKeyboard:  true,
		OneTimeKeyboard: true,
	}
}

// PlayButton returns an inline keyboard with a single web_app button that opens
// the Mini App at miniAppURL.
func PlayButton(buttonText, miniAppURL string) *ReplyMarkup {
	return &ReplyMarkup{
		InlineKeyboard: [][]InlineKeyboardButton{{{Text: buttonText, WebApp: &WebAppInfo{URL: miniAppURL}}}},
	}
}

// SendMessage sends text to chatID. replyMarkup may be nil. It returns an error
// if the Bot API responds with ok=false or a transport error occurs.
func (b *Bot) SendMessage(chatID int64, text string, replyMarkup *ReplyMarkup) error {
	payload := map[string]any{
		"chat_id": chatID,
		"text":    text,
	}
	if replyMarkup != nil {
		payload["reply_markup"] = replyMarkup
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal sendMessage payload: %w", err)
	}

	url := fmt.Sprintf("%s/bot%s/sendMessage", botAPIBase, b.token)
	resp, err := b.client.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("sendMessage request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("sendMessage failed: status %d: %s", resp.StatusCode, string(raw))
	}
	return nil
}

// SetWebAppMenuButton sets the chat menu button (the persistent button next to
// the message box) to a Mini App launcher opening webAppURL. Idempotent — safe
// to call on boot.
func (b *Bot) SetWebAppMenuButton(text, webAppURL string) error {
	payload := map[string]any{
		"menu_button": map[string]any{
			"type":    "web_app",
			"text":    text,
			"web_app": map[string]string{"url": webAppURL},
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal setChatMenuButton payload: %w", err)
	}
	url := fmt.Sprintf("%s/bot%s/setChatMenuButton", botAPIBase, b.token)
	resp, err := b.client.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("setChatMenuButton request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("setChatMenuButton failed: status %d: %s", resp.StatusCode, string(raw))
	}
	return nil
}

// AnswerCallbackQuery acknowledges an inline-button tap. Telegram shows a
// loading spinner on the button until this is called; leaving it unanswered
// makes the button look stuck. text (optional) pops a small toast to the user.
func (b *Bot) AnswerCallbackQuery(callbackID, text string) error {
	payload := map[string]any{"callback_query_id": callbackID}
	if text != "" {
		payload["text"] = text
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal answerCallbackQuery payload: %w", err)
	}
	url := fmt.Sprintf("%s/bot%s/answerCallbackQuery", botAPIBase, b.token)
	resp, err := b.client.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("answerCallbackQuery request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("answerCallbackQuery failed: status %d: %s", resp.StatusCode, string(raw))
	}
	return nil
}
