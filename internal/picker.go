package internal

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

// pickDirectory opens the OS-native folder picker dialog and returns the selected path.
func pickDirectory() (string, error) {
	switch runtime.GOOS {
	case "darwin":
		out, err := exec.Command("osascript", "-e",
			`POSIX path of (choose folder with prompt "Select folder")`).Output()
		if err != nil {
			return "", fmt.Errorf("cancelled")
		}
		return strings.TrimSpace(string(out)), nil

	case "linux":
		// Try zenity (GNOME), then kdialog (KDE)
		if out, err := exec.Command("zenity", "--file-selection", "--directory",
			"--title=Select folder").Output(); err == nil {
			return strings.TrimSpace(string(out)), nil
		}
		if out, err := exec.Command("kdialog", "--getexistingdirectory",
			"--title", "Select folder").Output(); err == nil {
			return strings.TrimSpace(string(out)), nil
		}
		return "", fmt.Errorf("no folder picker found (install zenity or kdialog)")

	case "windows":
		script := `Add-Type -AssemblyName System.Windows.Forms;` +
			`$d=New-Object System.Windows.Forms.FolderBrowserDialog;` +
			`$d.Description='Select folder';` +
			`if($d.ShowDialog() -eq 'OK'){$d.SelectedPath}`
		out, err := exec.Command("powershell", "-NoProfile", "-Command", script).Output()
		if err != nil || strings.TrimSpace(string(out)) == "" {
			return "", fmt.Errorf("cancelled")
		}
		return strings.TrimSpace(string(out)), nil

	default:
		return "", fmt.Errorf("folder picker not supported on %s", runtime.GOOS)
	}
}
