package service

import (
	"encoding/json"
	"net/url"
	"regexp"
	"slices"
	"strings"

	"github.com/mhsanaei/3x-ui/v3/internal/crypto/nodetoken"
	"github.com/mhsanaei/3x-ui/v3/internal/util/common"
)

// CardPayConfig is the destination card for manual card-to-card payments.
type CardPayConfig struct {
	Enabled      bool   `json:"enabled"`
	CardNumber   string `json:"cardNumber" example:"6037991812345678"`
	HolderName   string `json:"holderName" example:"Ali Rezaei"`
	BankName     string `json:"bankName" example:"Melli"`
	Instructions string `json:"instructions"`
}

// ZarinpalConfig is the Zarinpal merchant account. MerchantId is a secret.
type ZarinpalConfig struct {
	Enabled    bool   `json:"enabled"`
	MerchantId string `json:"merchantId"`
	Sandbox    bool   `json:"sandbox"`
}

// IDPayConfig is the IDPay account. ApiKey is a secret.
type IDPayConfig struct {
	Enabled bool   `json:"enabled"`
	ApiKey  string `json:"apiKey"`
	Sandbox bool   `json:"sandbox"`
}

// CryptoConfig is the NOWPayments account. ApiKey and IpnSecret are secrets.
type CryptoConfig struct {
	Enabled   bool   `json:"enabled"`
	ApiKey    string `json:"apiKey"`
	IpnSecret string `json:"ipnSecret"`
	Sandbox   bool   `json:"sandbox"`
}

// StripeConfig is the Stripe account. SecretKey and WebhookSecret are secrets;
// a sk_test_ key is Stripe's test mode.
type StripeConfig struct {
	Enabled       bool   `json:"enabled"`
	SecretKey     string `json:"secretKey"`
	WebhookSecret string `json:"webhookSecret"`
}

// PayPalConfig is the PayPal REST app. ClientSecret is a secret; the client id
// is public by design.
type PayPalConfig struct {
	Enabled      bool   `json:"enabled"`
	ClientId     string `json:"clientId"`
	ClientSecret string `json:"clientSecret"`
	Sandbox      bool   `json:"sandbox"`
}

// ReferralConfig pays a customer a share of a friend's first purchase, into
// their wallet, in the currency of the purchase.
type ReferralConfig struct {
	Enabled      bool `json:"enabled"`
	BonusPercent int  `json:"bonusPercent" example:"10"`
}

// ForceJoinChat is a channel or group a buyer must belong to before the bot
// serves them. Chat is an @username or a numeric chat id; Link is where the
// buyer joins (derived from an @username when blank).
type ForceJoinChat struct {
	Chat string `json:"chat" example:"@mychannel"`
	Link string `json:"link" example:"https://t.me/mychannel"`
}

// BotLink is an extra button on the bot menu, such as a support channel.
type BotLink struct {
	Label string `json:"label" example:"Tutorials"`
	URL   string `json:"url" example:"https://t.me/mychannel"`
}

// ShopBotConfig tunes the Telegram bot's shop. ReceiptChatId is a group or
// channel that gets card receipts besides the admins' own chats; MiniApp adds
// the "open shop" button.
type ShopBotConfig struct {
	ForceJoin     []ForceJoinChat `json:"forceJoin"`
	Links         []BotLink       `json:"links"`
	ReceiptChatId int64           `json:"receiptChatId" example:"-1001234567890"`
	MiniApp       bool            `json:"miniApp"`
}

// CustomOfferConfig sells a quota and a duration the buyer picks, priced per
// GB and per day in Currency's minor unit. TemplateId supplies the inbounds.
type CustomOfferConfig struct {
	Enabled     bool   `json:"enabled"`
	TemplateId  int    `json:"templateId" example:"1"`
	Currency    string `json:"currency" example:"IRT"`
	PricePerGB  int64  `json:"pricePerGb" example:"2000"`
	PricePerDay int64  `json:"pricePerDay" example:"1000"`
	MinGB       int    `json:"minGb" example:"5"`
	MaxGB       int    `json:"maxGb" example:"500"`
	MinDays     int    `json:"minDays" example:"7"`
	MaxDays     int    `json:"maxDays" example:"365"`
	LimitIP     int    `json:"limitIp" example:"2"`
}

// Names of the secret fields, as ClearSecrets takes them.
const (
	secretZarinpalMerchant = "zarinpal.merchantId"
	secretIDPayKey         = "idpay.apiKey"
	secretCryptoKey        = "crypto.apiKey"
	secretCryptoIPN        = "crypto.ipnSecret"
	secretStripeKey        = "stripe.secretKey"
	secretStripeWebhook    = "stripe.webhookSecret"
	secretPayPalSecret     = "paypal.clientSecret"
)

// SalesConfig is everything the operator configures about selling. It is one
// JSON setting. Secrets are served blank and kept when a save leaves them
// blank; ClearSecrets names the ones to empty on purpose.
type SalesConfig struct {
	PublicEnabled bool           `json:"publicEnabled"`
	PublicBaseURL string         `json:"publicBaseUrl" example:"https://shop.example.com"`
	Card          CardPayConfig  `json:"card"`
	Zarinpal      ZarinpalConfig `json:"zarinpal"`
	IDPay         IDPayConfig    `json:"idpay"`
	Crypto        CryptoConfig   `json:"crypto"`
	Stripe        StripeConfig   `json:"stripe"`
	PayPal        PayPalConfig   `json:"paypal"`
	WalletEnabled bool           `json:"walletEnabled"`
	// TelegramShop opens the shop to customers inside the Telegram bot.
	TelegramShop bool              `json:"telegramShop"`
	Referral     ReferralConfig    `json:"referral"`
	Bot          ShopBotConfig     `json:"bot"`
	Custom       CustomOfferConfig `json:"custom"`
	ClearSecrets []string          `json:"clearSecrets,omitempty"`
}

// SalesConfigView is the configuration as the admin API serves it: secrets are
// blank and the Has flags say which are set.
type SalesConfigView struct {
	SalesConfig
	HasZarinpalMerchantId  bool `json:"hasZarinpalMerchantId"`
	HasIdpayApiKey         bool `json:"hasIdpayApiKey"`
	HasCryptoApiKey        bool `json:"hasCryptoApiKey"`
	HasCryptoIpnSecret     bool `json:"hasCryptoIpnSecret"`
	HasStripeSecretKey     bool `json:"hasStripeSecretKey"`
	HasStripeWebhookSecret bool `json:"hasStripeWebhookSecret"`
	HasPaypalClientSecret  bool `json:"hasPaypalClientSecret"`
}

const salesConfigKey = "salesConfig"

// SalesConfigService reads and writes the sales configuration.
type SalesConfigService struct {
	settings SettingService
}

// secrets lists the secret fields of cfg by name.
func (c *SalesConfig) secrets() map[string]*string {
	return map[string]*string{
		secretZarinpalMerchant: &c.Zarinpal.MerchantId,
		secretIDPayKey:         &c.IDPay.ApiKey,
		secretCryptoKey:        &c.Crypto.ApiKey,
		secretCryptoIPN:        &c.Crypto.IpnSecret,
		secretStripeKey:        &c.Stripe.SecretKey,
		secretStripeWebhook:    &c.Stripe.WebhookSecret,
		secretPayPalSecret:     &c.PayPal.ClientSecret,
	}
}

func secretAAD(name string) []byte { return []byte("settings/salesConfig/" + name) }

// load reads the stored configuration with its secrets decrypted.
func (s *SalesConfigService) load() (*SalesConfig, error) {
	raw, err := s.settings.getString(salesConfigKey)
	if err != nil {
		return nil, err
	}
	cfg := &SalesConfig{}
	if strings.TrimSpace(raw) == "" {
		return cfg, nil
	}
	if err := json.Unmarshal([]byte(raw), cfg); err != nil {
		return nil, err
	}
	for name, field := range cfg.secrets() {
		plain, err := nodetoken.DecryptBound(secretAAD(name), *field)
		if err != nil {
			return nil, err
		}
		*field = plain
	}
	return cfg, nil
}

// Get returns the configuration with secrets in the clear, for the server's
// own use. Never hand this to a response.
func (s *SalesConfigService) Get() (*SalesConfig, error) { return s.load() }

// GetView returns the configuration safe to serve to the admin UI.
func (s *SalesConfigService) GetView() (*SalesConfigView, error) {
	cfg, err := s.load()
	if err != nil {
		return nil, err
	}
	view := &SalesConfigView{
		HasZarinpalMerchantId:  cfg.Zarinpal.MerchantId != "",
		HasIdpayApiKey:         cfg.IDPay.ApiKey != "",
		HasCryptoApiKey:        cfg.Crypto.ApiKey != "",
		HasCryptoIpnSecret:     cfg.Crypto.IpnSecret != "",
		HasStripeSecretKey:     cfg.Stripe.SecretKey != "",
		HasStripeWebhookSecret: cfg.Stripe.WebhookSecret != "",
		HasPaypalClientSecret:  cfg.PayPal.ClientSecret != "",
	}
	for _, field := range cfg.secrets() {
		*field = ""
	}
	view.SalesConfig = *cfg
	return view, nil
}

// Save validates and stores cfg. Only an enabled section is validated, so a
// half-filled form can be parked disabled.
func (s *SalesConfigService) Save(cfg *SalesConfig) error {
	if cfg == nil {
		return common.NewError("empty sales config")
	}
	existing, err := s.load()
	if err != nil {
		return err
	}
	stored := *cfg
	stored.ClearSecrets = nil
	for name, field := range stored.secrets() {
		*field = strings.TrimSpace(*field)
		switch {
		case slices.Contains(cfg.ClearSecrets, name):
			*field = ""
		case *field == "":
			*field = *existing.secrets()[name]
		}
	}
	if err := stored.normalize(); err != nil {
		return err
	}
	for name, field := range stored.secrets() {
		enc, err := nodetoken.EncryptBound(secretAAD(name), *field)
		if err != nil {
			return err
		}
		*field = enc
	}
	raw, err := json.Marshal(&stored)
	if err != nil {
		return err
	}
	return s.settings.saveSetting(salesConfigKey, string(raw))
}

// normalize trims fields and rejects an enabled method that cannot work.
func (c *SalesConfig) normalize() error {
	c.PublicBaseURL = strings.TrimRight(strings.TrimSpace(c.PublicBaseURL), "/")
	card := &c.Card
	card.HolderName = strings.TrimSpace(card.HolderName)
	card.BankName = strings.TrimSpace(card.BankName)
	card.Instructions = strings.TrimSpace(card.Instructions)
	card.CardNumber = digitsOnly(card.CardNumber)
	if card.Enabled && len(card.CardNumber) != 16 {
		return common.NewError("card number must be 16 digits")
	}
	if c.Referral.Enabled && (c.Referral.BonusPercent < 1 || c.Referral.BonusPercent > 100) {
		return common.NewError("the referral bonus must be between 1 and 100 percent")
	}
	if err := c.normalizeBot(); err != nil {
		return err
	}
	if err := c.Custom.normalize(); err != nil {
		return err
	}
	online := c.Zarinpal.Enabled || c.IDPay.Enabled || c.Crypto.Enabled || c.Stripe.Enabled || c.PayPal.Enabled
	if online && !validBaseURL(c.PublicBaseURL) {
		return common.NewError("an online payment method needs the public base URL (http or https)")
	}
	if c.Zarinpal.Enabled && c.Zarinpal.MerchantId == "" {
		return common.NewError("zarinpal needs a merchant id")
	}
	if c.IDPay.Enabled && c.IDPay.ApiKey == "" {
		return common.NewError("idpay needs an API key")
	}
	if c.Crypto.Enabled && (c.Crypto.ApiKey == "" || c.Crypto.IpnSecret == "") {
		return common.NewError("crypto needs an API key and an IPN secret")
	}
	if c.Stripe.Enabled && c.Stripe.SecretKey == "" {
		return common.NewError("stripe needs a secret key")
	}
	c.PayPal.ClientId = strings.TrimSpace(c.PayPal.ClientId)
	if c.PayPal.Enabled && (c.PayPal.ClientId == "" || c.PayPal.ClientSecret == "") {
		return common.NewError("paypal needs a client id and a client secret")
	}
	return nil
}

func validBaseURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != "" && u.RawQuery == "" && u.Fragment == ""
}

// digitsOnly keeps the digits of s, folding Persian and Arabic-Indic digits
// to ASCII so a card number pasted from a Persian keyboard still validates.
func digitsOnly(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r >= '۰' && r <= '۹':
			b.WriteRune('0' + r - '۰')
		case r >= '٠' && r <= '٩':
			b.WriteRune('0' + r - '٠')
		}
	}
	return b.String()
}

const (
	maxForceJoin  = 3
	maxBotLinks   = 6
	maxLabelRunes = 40
	maxURLLen     = 300
	maxUnitPrice  = int64(1_000_000_000_000)
	maxCustomGB   = 100_000
	maxCustomDays = 3650
)

var forceJoinChat = regexp.MustCompile(`^(@[A-Za-z][A-Za-z0-9_]{4,31}|-?[0-9]{5,20})$`)

func (c *SalesConfig) normalizeBot() error {
	b := &c.Bot
	if len(b.ForceJoin) > maxForceJoin {
		return common.NewError("at most", maxForceJoin, "channels can be required")
	}
	for i := range b.ForceJoin {
		j := &b.ForceJoin[i]
		j.Chat, j.Link = strings.TrimSpace(j.Chat), strings.TrimSpace(j.Link)
		if !forceJoinChat.MatchString(j.Chat) {
			return common.NewError("a required channel must be an @username or a numeric chat id")
		}
		if j.Link == "" && strings.HasPrefix(j.Chat, "@") {
			j.Link = "https://t.me/" + strings.TrimPrefix(j.Chat, "@")
		}
		if !validButtonURL(j.Link) {
			return common.NewError("a required channel needs a join link (https or tg)")
		}
	}
	if len(b.Links) > maxBotLinks {
		return common.NewError("at most", maxBotLinks, "menu links are allowed")
	}
	for i := range b.Links {
		l := &b.Links[i]
		l.Label, l.URL = strings.TrimSpace(l.Label), strings.TrimSpace(l.URL)
		if l.Label == "" || len([]rune(l.Label)) > maxLabelRunes || !validButtonURL(l.URL) {
			return common.NewError("a menu link needs a label (up to 40 characters) and an https or tg URL")
		}
	}
	if b.MiniApp && !strings.HasPrefix(c.PublicBaseURL, "https://") {
		return common.NewError("the Mini App needs the public base URL to be https")
	}
	return nil
}

// validButtonURL accepts what Telegram accepts for a URL button.
func validButtonURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || len(raw) > maxURLLen {
		return false
	}
	return (u.Scheme == "https" && u.Host != "") || (u.Scheme == "tg" && u.Host != "")
}

func (o *CustomOfferConfig) normalize() error {
	o.Currency = strings.ToUpper(strings.TrimSpace(o.Currency))
	if !o.Enabled {
		return nil
	}
	if _, ok := CurrencyDecimals(o.Currency); !ok {
		return common.NewError("unsupported currency:", o.Currency)
	}
	if _, err := (&ClientService{}).GetClientTemplate(o.TemplateId); err != nil {
		return err
	}
	if o.PricePerGB < 0 || o.PricePerDay < 0 || o.PricePerGB > maxUnitPrice || o.PricePerDay > maxUnitPrice ||
		(o.PricePerGB == 0 && o.PricePerDay == 0) {
		return common.NewError("the per-GB and per-day prices cannot be negative and cannot both be zero")
	}
	if o.MinGB < 1 || o.MaxGB < o.MinGB || o.MaxGB > maxCustomGB ||
		o.MinDays < 1 || o.MaxDays < o.MinDays || o.MaxDays > maxCustomDays || o.LimitIP < 0 {
		return common.NewError("the custom quota and duration bounds are invalid")
	}
	return nil
}
