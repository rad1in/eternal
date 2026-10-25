package tgwebapp

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
)

// signInitData builds initData the way Telegram documents it: the data-check
// string is the sorted key=value lines, keyed by HMAC("WebAppData", token).
func signInitData(t *testing.T, token string, fields map[string]string) string {
	t.Helper()
	keys := make([]string, 0, len(fields))
	for k := range fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	lines := make([]string, 0, len(keys))
	for _, k := range keys {
		lines = append(lines, k+"="+fields[k])
	}
	secret := hmac.New(sha256.New, []byte("WebAppData"))
	secret.Write([]byte(token))
	mac := hmac.New(sha256.New, secret.Sum(nil))
	mac.Write([]byte(strings.Join(lines, "\n")))
	q := url.Values{}
	for k, v := range fields {
		q.Set(k, v)
	}
	q.Set("hash", hex.EncodeToString(mac.Sum(nil)))
	return q.Encode()
}

func TestValidateAcceptsTelegramDocumentedSignature(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	token := "123456:ABC-DEF"
	initData := signInitData(t, token, map[string]string{
		"auth_date": strconv.FormatInt(now.Add(-time.Minute).Unix(), 10),
		"query_id":  "AAH",
		"user":      `{"id":777,"first_name":"Ali","username":"ali_r","language_code":"fa"}`,
	})

	got, err := Validate(initData, token, time.Hour, now)
	if err != nil {
		t.Fatal(err)
	}
	if got.User.ID != 777 || got.User.FirstName != "Ali" || got.User.Username != "ali_r" || got.User.LanguageCode != "fa" {
		t.Fatalf("user = %+v", got.User)
	}
}

func TestValidateRefusals(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	token := "123456:ABC-DEF"
	fresh := strconv.FormatInt(now.Add(-time.Minute).Unix(), 10)
	user := `{"id":777,"first_name":"Ali"}`
	good := signInitData(t, token, map[string]string{"auth_date": fresh, "user": user})

	tampered := strings.Replace(good, url.QueryEscape(`"id":777`), url.QueryEscape(`"id":778`), 1)
	if tampered == good {
		t.Fatal("test bug: tampering did not change the payload")
	}
	stale := signInitData(t, token, map[string]string{"auth_date": strconv.FormatInt(now.Add(-48*time.Hour).Unix(), 10), "user": user})
	future := signInitData(t, token, map[string]string{"auth_date": strconv.FormatInt(now.Add(time.Hour).Unix(), 10), "user": user})
	noUser := signInitData(t, token, map[string]string{"auth_date": fresh})
	badUser := signInitData(t, token, map[string]string{"auth_date": fresh, "user": `{"id":0}`})

	cases := []struct {
		name  string
		data  string
		token string
		want  error
	}{
		{"tampered payload", tampered, token, ErrBadSignature},
		{"signed by another bot", good, "999:OTHER", ErrBadSignature},
		{"missing hash", strings.Split(good, "&hash=")[0], token, ErrBadSignature},
		{"stale", stale, token, ErrExpired},
		{"from the future", future, token, ErrExpired},
		{"no user", noUser, token, ErrNoUser},
		{"user without id", badUser, token, ErrNoUser},
		{"empty", "", token, ErrBadSignature},
		{"no bot token", good, "", ErrBadSignature},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := Validate(c.data, c.token, time.Hour, now); !errors.Is(err, c.want) {
				t.Fatalf("err = %v, want %v", err, c.want)
			}
		})
	}
}
