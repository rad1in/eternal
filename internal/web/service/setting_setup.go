package service

const (
	settingSetupPending   = "setupPending"
	settingSetupTokenHash = "setupTokenHash"
)

// SetupPending reports whether the first-run wizard is still waiting to be completed.
func (s *SettingService) SetupPending() (bool, error) { return s.getBool(settingSetupPending) }

func (s *SettingService) SetSetupPending(pending bool) error {
	return s.setBool(settingSetupPending, pending)
}

// SetupTokenHash is the SHA-256 of the one-time token that opens the wizard.
func (s *SettingService) SetupTokenHash() (string, error) { return s.getString(settingSetupTokenHash) }

func (s *SettingService) SetSetupTokenHash(hash string) error {
	return s.setString(settingSetupTokenHash, hash)
}
