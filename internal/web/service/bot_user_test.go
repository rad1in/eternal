package service

import "testing"

func TestBotUserKeepsItsChosenLanguageAcrossVisits(t *testing.T) {
	setupBulkDB(t)
	svc := &BotUserService{}

	u, err := svc.Touch(77, "Ali", "ali_r", "fa-IR")
	if err != nil || u.Lang != "fa-IR" {
		t.Fatalf("first visit = %+v, %v; want the Telegram language guess", u, err)
	}
	if err := svc.SetLang(77, "ru-RU"); err != nil {
		t.Fatal(err)
	}
	u, err = svc.Touch(77, "Ali R", "ali_r", "fa-IR")
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := svc.Get(77); got.Lang != "ru-RU" || got.FirstName != "Ali R" {
		t.Fatalf("after a second visit = %+v, want the chosen ru-RU kept and the new name stored", got)
	}
}

func TestBanAndReferralWorkBeforeTheFirstVisitAndReferralIsKeptOnce(t *testing.T) {
	setupBulkDB(t)
	svc := &BotUserService{}

	if svc.IsBanned(5) {
		t.Fatal("an unknown account is banned")
	}
	if err := svc.SetBanned(5, true); err != nil || !svc.IsBanned(5) {
		t.Fatalf("ban = %v / banned %v", err, svc.IsBanned(5))
	}
	if err := svc.SetBanned(5, false); err != nil || svc.IsBanned(5) {
		t.Fatalf("unban = %v / banned %v", err, svc.IsBanned(5))
	}

	if err := svc.RememberReferral(5, " k7m2qx9p "); err != nil {
		t.Fatal(err)
	}
	if err := svc.RememberReferral(5, "ZZZZZZZZ"); err != nil {
		t.Fatal(err)
	}
	if got, _ := svc.Get(5); got.ReferralCode != "K7M2QX9P" {
		t.Fatalf("referral = %q, want the first code kept", got.ReferralCode)
	}
}
