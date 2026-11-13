package installer

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// NSISScript is an installer script compiled by makensis.
func NSISScript(name, version, goarch, source, outfile string) string {
	installDir := `$PROGRAMFILES64\` + nsisString(name)
	if goarch == "386" {
		installDir = `$PROGRAMFILES\` + nsisString(name)
	}
	regKey := `Software\Microsoft\Windows\CurrentVersion\Uninstall\` + nsisString(name)
	exeName := nsisString(name) + ".exe"
	return fmt.Sprintf(`Unicode true
Name "%s %s"
OutFile "%s"
InstallDir "%s"
RequestExecutionLevel admin
ShowInstDetails show

Page directory
Page instfiles
UninstPage uninstConfirm
UninstPage instfiles

Section "Install"
  SetOutPath $INSTDIR
  File "/oname=%s" "%s"
  WriteUninstaller "$INSTDIR\Uninstall.exe"
  WriteRegStr HKLM "%s" "DisplayName" "%s"
  WriteRegStr HKLM "%s" "DisplayVersion" "%s"
  WriteRegStr HKLM "%s" "Publisher" "%s"
  WriteRegStr HKLM "%s" "UninstallString" "$\"$INSTDIR\Uninstall.exe$\""
  WriteRegDWORD HKLM "%s" "NoModify" 1
  WriteRegDWORD HKLM "%s" "NoRepair" 1
SectionEnd

Section "Uninstall"
  Delete "$INSTDIR\%s"
  Delete "$INSTDIR\Uninstall.exe"
  RMDir "$INSTDIR"
  DeleteRegKey HKLM "%s"
SectionEnd
`,
		nsisString(name), nsisString(version),
		nsisString(outfile),
		installDir,
		exeName, nsisString(filepath.ToSlash(source)),
		regKey, nsisString(name),
		regKey, nsisString(version),
		regKey, nsisString(name),
		regKey,
		regKey,
		regKey,
		exeName,
		regKey,
	)
}

// NSIS compiles script with makensis and writes the installer next to it.
func NSIS(script, outfile string) error {
	makensis, err := exec.LookPath("makensis")
	if err != nil {
		return fmt.Errorf("makensis is not installed; install NSIS to build Windows installers")
	}
	if err := os.MkdirAll(filepath.Dir(outfile), 0o755); err != nil {
		return err
	}
	scriptFile := strings.TrimSuffix(outfile, filepath.Ext(outfile)) + ".nsi"
	if err := os.WriteFile(scriptFile, []byte(script), 0o644); err != nil {
		return err
	}
	defer os.Remove(scriptFile)
	cmd := exec.Command(makensis, "-V2", scriptFile)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("makensis: %w\n%s", err, strings.TrimSpace(string(out)))
	}
	if _, err := os.Stat(outfile); err != nil {
		return fmt.Errorf("makensis did not write %s\n%s", outfile, strings.TrimSpace(string(out)))
	}
	return nil
}

func nsisString(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, `"`, "")
	s = strings.ReplaceAll(s, "$", "")
	return s
}
