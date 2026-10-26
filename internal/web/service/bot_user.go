package service

import (
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// seenRefresh is how stale last_seen_at may get before a visit rewrites it.
const seenRefresh = 10 * time.Minute

// BotUserService keeps the small per-Telegram-account state of the shop bot.
type BotUserService struct{}

func clip(s string, runes int) string {
	if utf8.RuneCountInString(s) <= runes {
		return s
	}
	return string([]rune(s)[:runes])
}

// Touch records a visit. A new account starts with defaultLang (Telegram's own
// language guess); a returning one only has its name, handle and last-seen
// time refreshed, and only when something changed or the stamp is stale.
func (s *BotUserService) Touch(tgID int64, firstName, username, defaultLang string) (*model.BotUser, error) {
	db := database.GetDB()
	now := time.Now().UnixMilli()
	firstName, username = clip(firstName, 64), clip(username, 64)
	var u model.BotUser
	err := db.Where("tg_id = ?", tgID).First(&u).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		u = model.BotUser{TgID: tgID, Lang: defaultLang, FirstName: firstName, Username: username, LastSeenAt: now}
		err = db.Clauses(clause.OnConflict{DoNothing: true}).Create(&u).Error
		if err != nil {
			return nil, err
		}
		return s.Get(tgID)
	}
	if err != nil {
		return nil, err
	}
	if u.FirstName == firstName && u.Username == username && now-u.LastSeenAt < seenRefresh.Milliseconds() {
		return &u, nil
	}
	u.FirstName, u.Username, u.LastSeenAt = firstName, username, now
	err = db.Model(&model.BotUser{}).Where("id = ?", u.Id).
		Updates(map[string]any{"first_name": firstName, "username": username, "last_seen_at": now}).Error
	return &u, err
}

// Get returns the account's row, or a zero-value one when it never visited.
func (s *BotUserService) Get(tgID int64) (*model.BotUser, error) {
	var u model.BotUser
	err := database.GetDB().Where("tg_id = ?", tgID).First(&u).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return &model.BotUser{TgID: tgID}, nil
	}
	return &u, err
}

func (s *BotUserService) update(tgID int64, column string, value any) error {
	db := database.GetDB()
	if err := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&model.BotUser{TgID: tgID}).Error; err != nil {
		return err
	}
	return db.Model(&model.BotUser{}).Where("tg_id = ?", tgID).Update(column, value).Error
}

// SetLang stores the language the account picked; the caller checks it is one
// the panel ships.
func (s *BotUserService) SetLang(tgID int64, lang string) error { return s.update(tgID, "lang", lang) }

// SetBanned bars or readmits an account.
func (s *BotUserService) SetBanned(tgID int64, banned bool) error {
	return s.update(tgID, "banned", banned)
}

// IsBanned reports whether the account is barred from the shop bot.
func (s *BotUserService) IsBanned(tgID int64) bool {
	var n int64
	database.GetDB().Model(&model.BotUser{}).Where("tg_id = ? AND banned = ?", tgID, true).Count(&n)
	return n > 0
}

// RememberReferral keeps the referral code an account arrived with, once: a
// later link does not take the referrer away from whoever brought them first.
func (s *BotUserService) RememberReferral(tgID int64, code string) error {
	code = strings.ToUpper(strings.TrimSpace(code))
	if code == "" || len(code) > 16 {
		return nil
	}
	db := database.GetDB()
	if err := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&model.BotUser{TgID: tgID}).Error; err != nil {
		return err
	}
	return db.Model(&model.BotUser{}).Where("tg_id = ? AND referral_code = ''", tgID).Update("referral_code", code).Error
}
