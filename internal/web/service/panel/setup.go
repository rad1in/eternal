package panel

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"net"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/util/crypto"
	"github.com/mhsanaei/3x-ui/v3/internal/util/random"
	"github.com/mhsanaei/3x-ui/v3/internal/web/global"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
)

// SetupError is a refusal the wizard shows by code; the codes are the keys of its translations.
type SetupError struct{ Code string }

func (e *SetupError) Error() string { return e.Code }

var (
	ErrSetupBadToken   = &SetupError{"bad_token"}
	ErrSetupDone       = &SetupError{"already_done"}
	ErrSetupConfigured = &SetupError{"already_configured"}
)

func setupErr(code string) error { return &SetupError{code} }

const (
	setupTokenBytes = 16
	minPort         = 1024
	maxPort         = 65535
	portPickTries   = 50
)

var (
	usernameRule = regexp.MustCompile(`^[A-Za-z0-9_.-]{3,32}$`)
	basePathRule = regexp.MustCompile(`^/[A-Za-z0-9_-]+(/[A-Za-z0-9_-]+)*/$`)
	commonPasses = map[string]bool{
		"admin": true, "password": true, "password1": true, "password123": true, "12345678": true,
		"123456789": true, "1234567890": true, "qwerty123": true, "qwertyuiop": true, "letmein123": true,
		"admin123": true, "administrator": true, "iloveyou": true, "11111111": true, "00000000": true,
	}
)

// SetupInput is what the wizard submits on its last step.
type SetupInput struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Port     int    `json:"port"`
	BasePath string `json:"basePath"`
	Language string `json:"language"`
}

// SetupResult is the address the panel moves to once setup is done.
type SetupResult struct {
	Port     int    `json:"port" example:"28431"`
	BasePath string `json:"basePath" example:"/k3j9x2m1q8/"`
}

// SetupSuggestion pre-fills the wizard.
type SetupSuggestion struct {
	Port     int    `json:"suggestedPort" example:"28431"`
	BasePath string `json:"suggestedBasePath" example:"/k3j9x2m1q8/"`
}

// SetupService runs the first-run wizard: one token printed by the installer
// opens it, and finishing it replaces the default login.
type SetupService struct {
	settings service.SettingService
	users    UserService
	restart  func()
	mu       sync.Mutex
}

// NewSetupService builds the service with its own restart hook; the zero value restarts the real panel.
func NewSetupService(restart func()) *SetupService { return &SetupService{restart: restart} }

func (s *SetupService) restartPanel() {
	if s.restart != nil {
		s.restart()
		return
	}
	_ = (&PanelService{}).RestartPanel(2 * time.Second)
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// Pending reports whether the wizard is still waiting to be completed.
func (s *SetupService) Pending() bool {
	pending, err := s.settings.SetupPending()
	return err == nil && pending
}

// Load publishes the stored wizard state to the request gate at server start. A
// wizard left open on a panel whose login was changed some other way is closed,
// so that panel is never hidden behind it.
func (s *SetupService) Load() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Pending() {
		if isDefault, err := s.hasDefaultLogin(); err == nil && !isDefault {
			s.closeLocked()
		}
	}
	global.SetSetupPending(s.Pending())
}

func (s *SetupService) closeLocked() {
	_ = s.settings.SetSetupTokenHash("")
	_ = s.settings.SetSetupPending(false)
	global.SetSetupPending(false)
}

func (s *SetupService) hasDefaultLogin() (bool, error) {
	owner, err := s.users.GetPrimaryOwner()
	if err != nil {
		return false, err
	}
	return owner.Username == "admin" && crypto.CheckPasswordHash(owner.Password, "admin"), nil
}

func (s *SetupService) newToken() (string, error) {
	raw := make([]byte, setupTokenBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	token := hex.EncodeToString(raw)
	if err := s.settings.SetSetupTokenHash(hashToken(token)); err != nil {
		return "", err
	}
	return token, nil
}

// Init opens the wizard on a panel that still has its default login: it picks a
// port unless one is given and returns the token that opens the wizard. Calling
// it again while the wizard is open replaces the token.
func (s *SetupService) Init(port int) (token string, chosen int, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	isDefault, err := s.hasDefaultLogin()
	if err != nil {
		return "", 0, err
	}
	if !isDefault {
		s.closeLocked()
		return "", 0, ErrSetupConfigured
	}
	if port == 0 && s.Pending() {
		port, _ = s.settings.GetPort()
	}
	if port == 0 {
		if port, err = pickFreePort(); err != nil {
			return "", 0, err
		}
	} else if err := validatePort(port); err != nil {
		return "", 0, err
	}
	if err := s.settings.SetPort(port); err != nil {
		return "", 0, err
	}
	if token, err = s.newToken(); err != nil {
		return "", 0, err
	}
	if err := s.settings.SetSetupPending(true); err != nil {
		return "", 0, err
	}
	global.SetSetupPending(true)
	return token, port, nil
}

// VerifyToken checks a presented token against the stored hash in constant time.
func (s *SetupService) VerifyToken(token string) bool {
	if token == "" || !s.Pending() {
		return false
	}
	stored, err := s.settings.SetupTokenHash()
	if err != nil || stored == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(stored), []byte(hashToken(token))) == 1
}

// Suggest returns the port installed for this panel and a fresh secret path.
func (s *SetupService) Suggest() SetupSuggestion {
	port, err := s.settings.GetPort()
	if err != nil || port < minPort {
		port, _ = pickFreePort()
	}
	return SetupSuggestion{Port: port, BasePath: "/" + random.Seq(14) + "/"}
}

// PortFree reports whether the wizard may move the panel to port.
func (s *SetupService) PortFree(port int) bool {
	if validatePort(port) != nil {
		return false
	}
	if current, err := s.settings.GetPort(); err == nil && current == port {
		return true
	}
	return listenable(port)
}

// Finish replaces the default login, moves the panel to the chosen address, ends
// the wizard and restarts the panel so it listens there.
func (s *SetupService) Finish(token string, in SetupInput) (*SetupResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.Pending() {
		return nil, ErrSetupDone
	}
	if !s.VerifyToken(token) {
		return nil, ErrSetupBadToken
	}
	if err := validateUsername(in.Username); err != nil {
		return nil, err
	}
	if err := validatePassword(in.Username, in.Password); err != nil {
		return nil, err
	}
	if err := validatePort(in.Port); err != nil {
		return nil, err
	}
	if !s.PortFree(in.Port) {
		return nil, setupErr("port_in_use")
	}
	if err := validateBasePath(in.BasePath); err != nil {
		return nil, err
	}

	if err := s.users.UpdateFirstUser(in.Username, in.Password); err != nil {
		return nil, fmt.Errorf("setup: save login: %w", err)
	}
	if err := s.settings.SetPort(in.Port); err != nil {
		return nil, fmt.Errorf("setup: save port: %w", err)
	}
	if err := s.settings.SetBasePath(in.BasePath); err != nil {
		return nil, fmt.Errorf("setup: save path: %w", err)
	}
	if err := s.settings.SetSetupTokenHash(""); err != nil {
		return nil, err
	}
	if err := s.settings.SetSetupPending(false); err != nil {
		return nil, err
	}
	global.SetSetupPending(false)
	s.restartPanel()
	return &SetupResult{Port: in.Port, BasePath: in.BasePath}, nil
}

func validateUsername(name string) error {
	if !usernameRule.MatchString(name) {
		return setupErr("username_invalid")
	}
	return nil
}

func validatePassword(username, password string) error {
	if len(password) < 8 || len(password) > 128 || strings.EqualFold(password, username) || commonPasses[strings.ToLower(password)] {
		return setupErr("password_weak")
	}
	return nil
}

func validatePort(port int) error {
	if port < minPort || port > maxPort {
		return setupErr("port_invalid")
	}
	return nil
}

func validateBasePath(path string) error {
	if len(path) < 4 || len(path) > 64 || !basePathRule.MatchString(path) {
		return setupErr("base_path_invalid")
	}
	return nil
}

func listenable(port int) bool {
	l, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return false
	}
	_ = l.Close()
	return true
}

// pickFreePort returns a random port in the dynamic range that nothing listens on.
func pickFreePort() (int, error) {
	const lo, hi = 20000, 60000
	for range portPickTries {
		n, err := rand.Int(rand.Reader, big.NewInt(hi-lo))
		if err != nil {
			return 0, err
		}
		if port := lo + int(n.Int64()); listenable(port) {
			return port, nil
		}
	}
	return 0, errors.New("setup: no free port found")
}
