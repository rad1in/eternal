package service

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"strconv"
	"strings"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/util/tgwebapp"
)

const (
	// initData older than this is not accepted for a new session.
	initDataMaxAge = 24 * time.Hour
	// A session token lives as long as a Mini App is plausibly left open.
	shopSessionTTL = 12 * time.Hour
)

// ShopSession is a buyer signed in through the Telegram Mini App.
type ShopSession struct {
	Token    string        `json:"token"`
	TgId     int64         `json:"tgId" example:"123456789"`
	Name     string        `json:"name" example:"Ali"`
	Lang     string        `json:"lang" example:"fa"`
	Services []ShopService `json:"services"`
}

// ShopSessionService turns verified Telegram launch data into a short-lived
// signed token. The token is stateless: the panel secret is its only key.
type ShopSessionService struct {
	settings SettingService
	config   SalesConfigService
	orders   OrderService
	users    BotUserService
}

// Exchange verifies a Mini App's initData against the bot token and opens a
// session for the Telegram account it names.
func (s *ShopSessionService) Exchange(initData, host string) (*ShopSession, error) {
	cfg, err := s.config.Get()
	if err != nil {
		return nil, err
	}
	if !cfg.Bot.MiniApp {
		return nil, salesErr(ErrGatewayUnavailable)
	}
	botToken, err := s.settings.GetTgBotToken()
	if err != nil {
		return nil, err
	}
	data, err := tgwebapp.Validate(initData, botToken, initDataMaxAge, time.Now())
	if err != nil {
		return nil, salesErr(ErrProofInvalid)
	}
	if s.users.IsBanned(data.User.ID) {
		return nil, salesErr(ErrBanned)
	}
	token, err := s.sign(data.User.ID, time.Now().Add(shopSessionTTL))
	if err != nil {
		return nil, err
	}
	services, err := s.orders.OwnedServices(data.User.ID, host)
	if err != nil {
		return nil, err
	}
	return &ShopSession{
		Token: token, TgId: data.User.ID, Name: data.User.FirstName, Lang: data.User.LanguageCode, Services: services,
	}, nil
}

func (s *ShopSessionService) key() ([]byte, error) {
	secret, err := s.settings.GetSecret()
	if err != nil {
		return nil, err
	}
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte("eternal/shop-telegram-session/v1"))
	return mac.Sum(nil), nil
}

func (s *ShopSessionService) sign(tgId int64, expires time.Time) (string, error) {
	key, err := s.key()
	if err != nil {
		return "", err
	}
	payload := strconv.FormatInt(tgId, 10) + ":" + strconv.FormatInt(expires.Unix(), 10)
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString([]byte(payload)) + "." + hex.EncodeToString(mac.Sum(nil)), nil
}

// Verify returns the Telegram id a live session token was issued for.
func (s *ShopSessionService) Verify(token string) (int64, error) {
	body, sig, ok := strings.Cut(token, ".")
	if !ok {
		return 0, salesErr(ErrProofInvalid)
	}
	raw, err := base64.RawURLEncoding.DecodeString(body)
	if err != nil {
		return 0, salesErr(ErrProofInvalid)
	}
	key, err := s.key()
	if err != nil {
		return 0, err
	}
	mac := hmac.New(sha256.New, key)
	mac.Write(raw)
	if !hmac.Equal([]byte(sig), []byte(hex.EncodeToString(mac.Sum(nil)))) {
		return 0, salesErr(ErrProofInvalid)
	}
	idText, expText, ok := strings.Cut(string(raw), ":")
	id, idErr := strconv.ParseInt(idText, 10, 64)
	exp, expErr := strconv.ParseInt(expText, 10, 64)
	if !ok || idErr != nil || expErr != nil || id <= 0 || time.Now().Unix() > exp {
		return 0, salesErr(ErrProofInvalid)
	}
	return id, nil
}
