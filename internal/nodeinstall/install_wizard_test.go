package nodeinstall

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// shellFunc returns the named function of the real install.sh.
func shellFunc(t *testing.T, lines []string, name string) string {
	t.Helper()
	for i, l := range lines {
		if l == name+"() {" {
			for j := i; j < len(lines); j++ {
				if lines[j] == "}" {
					return strings.Join(lines[i:j+1], "\n")
				}
			}
		}
	}
	t.Fatalf("install.sh has no function %s", name)
	return ""
}

// wizardBranch is config_after_install up to the end of the setup-wizard branch,
// cut out of the real install.sh so the test runs the shipped condition.
func wizardBranch(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "install.sh"))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(string(raw), "\n")
	start, end := -1, -1
	for i, l := range lines {
		if l == "config_after_install() {" {
			start = i
		}
		if start >= 0 && strings.Contains(l, "This build has no setup wizard") {
			end = i + 1
			break
		}
	}
	if start < 0 || end < 0 {
		t.Fatal("cannot find the setup-wizard branch in install.sh")
	}
	return shellFunc(t, lines, "kv_value") + "\n" + shellFunc(t, lines, "prepare_setup_wizard") + "\n" + strings.Join(lines[start:end+1], "\n")
}

func runBranch(t *testing.T, noninteractive string, extraEnv ...string) (string, string) {
	t.Helper()
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash is not installed")
	}
	dir := t.TempDir()
	calls := filepath.Join(dir, "calls")
	stub := "#!/bin/bash\necho \"$*\" >> " + calls + "\n" +
		"case \"$1\" in\n" +
		"  setting) echo 'hasDefaultCredential: true'; echo 'webBasePath: /'; echo 'port: 2053'; echo 'cert: ';;\n" +
		"  setup-init) echo setup_state=pending; echo setup_port=28431; echo setup_token=tok;;\n" +
		"esac\n"
	if err := os.WriteFile(filepath.Join(dir, "eternal"), []byte(stub), 0o755); err != nil {
		t.Fatal(err)
	}
	script := "xui_folder=" + dir + "\nNONINTERACTIVE=" + noninteractive + "\n" +
		"curl() { printf '203.0.113.10\\n200'; }\n" +
		"red=;green=;yellow=;plain=\n" +
		wizardBranch(t) + "\necho TERMINAL_FLOW\n}\nconfig_after_install\n"
	cmd := exec.Command("bash", "-c", script)
	cmd.Env = append(os.Environ(), extraEnv...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("script failed: %v\n%s", err, out)
	}
	recorded, _ := os.ReadFile(calls)
	return string(out), string(recorded)
}

func TestAnInteractiveFreshInstallOpensTheSetupWizard(t *testing.T) {
	out, calls := runBranch(t, "0", "XUI_USERNAME=", "XUI_PASSWORD=")
	if !strings.Contains(calls, "setup-init") || strings.Contains(out, "TERMINAL_FLOW") {
		t.Fatalf("want the wizard; calls:\n%s\noutput:\n%s", calls, out)
	}
}

func TestANonInteractiveInstallKeepsTheTerminalFlowSoTheNodeInstallerGetsItsResult(t *testing.T) {
	out, calls := runBranch(t, "1", "XUI_USERNAME=", "XUI_PASSWORD=")
	if strings.Contains(calls, "setup-init") || !strings.Contains(out, "TERMINAL_FLOW") {
		t.Fatalf("a non-interactive install must not open the wizard (it would never write install-result.env); calls:\n%s\noutput:\n%s", calls, out)
	}
}
