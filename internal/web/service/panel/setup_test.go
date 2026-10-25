package panel

import (
	"errors"
	"net"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/config"
	"github.com/mhsanaei/3x-ui/v3/internal/database/dbtest"
	"github.com/mhsanaei/3x-ui/v3/internal/web/global"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
)

func setupFixture(t *testing.T) (*SetupService, *int) {
	t.Helper()
	t.Setenv("XUI_DB_FOLDER", t.TempDir())
	dbtest.InitDB(t, config.GetDBPath())
	t.Cleanup(func() { global.SetSetupPending(false) })
	restarts := new(int)
	svc := &SetupService{restart: func() { *restarts++ }}
	return svc, restarts
}

func freeTCPPort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

func validInput(t *testing.T) SetupInput {
	return SetupInput{Username: "owner_1", Password: "correct-horse-9", Port: freeTCPPort(t), BasePath: "/k3j9x2m1q8/"}
}

func TestInitOpensTheWizardOnlyWhileTheDefaultLoginIsInPlace(t *testing.T) {
	svc, _ := setupFixture(t)
	token, port, err := svc.Init(0)
	if err != nil || len(token) < 32 || port < 1024 {
		t.Fatalf("Init = %q, %d, %v", token, port, err)
	}
	if !global.SetupPending() || !svc.Pending() {
		t.Fatal("a fresh install must wait for the wizard")
	}
	if !svc.VerifyToken(token) || svc.VerifyToken(token+"x") || svc.VerifyToken("") {
		t.Fatal("only the printed token opens the wizard")
	}
	// A second run hands out a new token and retires the old one.
	again, _, err := svc.Init(0)
	if err != nil || again == token || svc.VerifyToken(token) || !svc.VerifyToken(again) {
		t.Fatalf("re-init must rotate the token, got %q after %q (%v)", again, token, err)
	}

	if err := (&UserService{}).UpdateFirstUser("someone", "a-long-password"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := (&SetupService{}).Init(0); err != ErrSetupConfigured {
		t.Fatalf("an installed panel must not reopen its wizard, got %v", err)
	}
}

func TestFinishReplacesTheDefaultLoginAndClosesTheWizard(t *testing.T) {
	svc, restarts := setupFixture(t)
	token, _, err := svc.Init(0)
	if err != nil {
		t.Fatal(err)
	}
	in := validInput(t)
	res, err := svc.Finish(token, in)
	if err != nil || res.Port != in.Port || res.BasePath != in.BasePath {
		t.Fatalf("Finish = %+v, %v", res, err)
	}
	users := &UserService{}
	if u, err := users.CheckUser(in.Username, in.Password, ""); err != nil || u == nil {
		t.Fatalf("the new login does not work: %v", err)
	}
	if u, _ := users.CheckUser("admin", "admin", ""); u != nil {
		t.Fatal("the default login still works after setup")
	}
	settings := service.SettingService{}
	if port, _ := settings.GetPort(); port != in.Port {
		t.Errorf("port = %d, want %d", port, in.Port)
	}
	if bp, _ := settings.GetBasePath(); bp != in.BasePath {
		t.Errorf("base path = %q, want %q", bp, in.BasePath)
	}
	if svc.Pending() || global.SetupPending() || svc.VerifyToken(token) {
		t.Fatal("the wizard must close and the token die once setup is done")
	}
	if *restarts != 1 {
		t.Errorf("the panel must restart once to move to its new address, restarted %d times", *restarts)
	}
	if _, err := svc.Finish(token, in); err != ErrSetupDone {
		t.Fatalf("a second Finish = %v, want already_done", err)
	}
}

func TestFinishRefusesBadInputWithAStableCode(t *testing.T) {
	svc, restarts := setupFixture(t)
	token, _, _ := svc.Init(0)
	taken, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatal(err)
	}
	defer taken.Close()
	takenPort := taken.Addr().(*net.TCPAddr).Port

	cases := []struct {
		name string
		edit func(*SetupInput)
		code string
	}{
		{"short username", func(i *SetupInput) { i.Username = "ab" }, "username_invalid"},
		{"username with a space", func(i *SetupInput) { i.Username = "my admin" }, "username_invalid"},
		{"short password", func(i *SetupInput) { i.Password = "abc1234" }, "password_weak"},
		{"common password", func(i *SetupInput) { i.Password = "password123" }, "password_weak"},
		{"password equals username", func(i *SetupInput) { i.Username, i.Password = "owner_1234", "owner_1234" }, "password_weak"},
		{"port too low", func(i *SetupInput) { i.Port = 80 }, "port_invalid"},
		{"port too high", func(i *SetupInput) { i.Port = 70000 }, "port_invalid"},
		{"port taken", func(i *SetupInput) { i.Port = takenPort }, "port_in_use"},
		{"path too short", func(i *SetupInput) { i.BasePath = "/a/" }, "base_path_invalid"},
		{"path with double slash", func(i *SetupInput) { i.BasePath = "/abc//def/" }, "base_path_invalid"},
		{"path with a dot", func(i *SetupInput) { i.BasePath = "/abc.def/" }, "base_path_invalid"},
	}
	for _, tc := range cases {
		in := validInput(t)
		tc.edit(&in)
		_, err := svc.Finish(token, in)
		var se *SetupError
		if !errors.As(err, &se) || se.Code != tc.code {
			t.Errorf("%s: err = %v, want code %s", tc.name, err, tc.code)
		}
	}
	if !svc.Pending() || *restarts != 0 {
		t.Fatal("a refused request must change nothing")
	}
	if _, err := svc.Finish("wrong", validInput(t)); err == nil || err.Error() != "bad_token" {
		t.Fatalf("wrong token = %v, want bad_token", err)
	}
}

func TestSuggestionsAreFreshAndUsable(t *testing.T) {
	svc, _ := setupFixture(t)
	_, port, _ := svc.Init(0)
	a, b := svc.Suggest(), svc.Suggest()
	if a.Port != port {
		t.Errorf("suggested port %d, want the one setup-init picked (%d)", a.Port, port)
	}
	if a.BasePath == b.BasePath || len(a.BasePath) < 10 || a.BasePath[0] != '/' || a.BasePath[len(a.BasePath)-1] != '/' {
		t.Errorf("base paths must be random and slash-wrapped: %q %q", a.BasePath, b.BasePath)
	}
	if err := validateBasePath(a.BasePath); err != nil {
		t.Errorf("suggested path fails its own rules: %v", err)
	}
}

// A wizard left open on a panel whose login was changed another way (the CLI,
// say) must not keep the panel hidden behind it after a restart.
func TestLoadClosesAWizardThatNoLongerHasADefaultLoginToReplace(t *testing.T) {
	svc, _ := setupFixture(t)
	if _, _, err := svc.Init(0); err != nil {
		t.Fatal(err)
	}
	if err := (&UserService{}).UpdateFirstUser("someone", "a-long-password"); err != nil {
		t.Fatal(err)
	}
	global.SetSetupPending(false)
	svc.Load()
	if svc.Pending() || global.SetupPending() {
		t.Fatal("the panel stayed hidden behind a wizard that has nothing left to set up")
	}
}
