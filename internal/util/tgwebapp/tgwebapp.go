// Package tgwebapp verifies the initData a Telegram Mini App hands to its page.
package tgwebapp

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

var (
	ErrBadSignature = errors.New("telegram init data signature is invalid")
	ErrExpired      = errors.New("telegram init data is outside its freshness window")
	ErrNoUser       = errors.New("telegram init data carries no user")
)

// User is the account Telegram vouches for in initData.
type User struct {
	ID           int64  `json:"id"`
	FirstName    string `json:"first_name"`
	Username     string `json:"username"`
	LanguageCode string `json:"language_code"`
}

// InitData is a verified launch payload.
type InitData struct {
	User     User
	AuthDate time.Time
}

// Validate checks initData as Telegram documents it: the hash is the
// HMAC-SHA256 of the sorted key=value lines, keyed by HMAC-SHA256("WebAppData",
// botToken). Data older than maxAge, or dated in the future, is refused.
func Validate(initData, botToken string, maxAge time.Duration, now time.Time) (*InitData, error) {
	if botToken == "" || initData == "" {
		return nil, ErrBadSignature
	}
	values, err := url.ParseQuery(initData)
	if err != nil {
		return nil, ErrBadSignature
	}
	got := values.Get("hash")
	if got == "" {
		return nil, ErrBadSignature
	}
	values.Del("hash")
	lines := make([]string, 0, len(values))
	for k := range values {
		lines = append(lines, k+"="+values.Get(k))
	}
	sort.Strings(lines)

	secret := hmac.New(sha256.New, []byte("WebAppData"))
	secret.Write([]byte(botToken))
	mac := hmac.New(sha256.New, secret.Sum(nil))
	mac.Write([]byte(strings.Join(lines, "\n")))
	want := hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(got), []byte(want)) {
		return nil, ErrBadSignature
	}

	secs, err := strconv.ParseInt(values.Get("auth_date"), 10, 64)
	if err != nil {
		return nil, ErrExpired
	}
	issued := time.Unix(secs, 0)
	if issued.After(now.Add(time.Minute)) || now.Sub(issued) > maxAge {
		return nil, ErrExpired
	}

	var user User
	if err := json.Unmarshal([]byte(values.Get("user")), &user); err != nil || user.ID <= 0 {
		return nil, ErrNoUser
	}
	return &InitData{User: user, AuthDate: issued}, nil
}
