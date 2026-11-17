// Package dialog opens the operating system's native folder picker. It works because
// Lord Books runs on the user's own machine; the browser can't reveal a folder's path.
package dialog

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

// ErrCanceled is returned when the user closes the dialog without choosing a folder.
var ErrCanceled = errors.New("seleção cancelada")

// ErrUnsupported is returned when no native picker is available (e.g. Linux without zenity/kdialog).
var ErrUnsupported = errors.New("seletor de pastas indisponível neste sistema")

// PickFolder blocks until the user picks a folder or cancels. start may be empty.
func PickFolder(ctx context.Context, prompt, start string) (string, error) {
	switch runtime.GOOS {
	case "darwin":
		return pickMac(ctx, prompt, start)
	case "windows":
		return pickWindows(ctx, prompt, start)
	default:
		return pickLinux(ctx, prompt, start)
	}
}

func pickMac(ctx context.Context, prompt, start string) (string, error) {
	// Arguments go through argv, never spliced into the script, so paths with quotes are safe.
	// Running the dialog in the frontmost app keeps it in front of the browser.
	script := []string{
		"on run argv",
		"set p to item 1 of argv",
		"set d to item 2 of argv",
		"tell application (path to frontmost application as text)",
		"if d is \"\" then",
		"set f to choose folder with prompt p",
		"else",
		"try",
		"set f to choose folder with prompt p default location (POSIX file d)",
		"on error number n",
		"if n is -128 then error number -128",
		"set f to choose folder with prompt p",
		"end try",
		"end if",
		"end tell",
		"return POSIX path of f",
		"end run",
	}
	args := []string{}
	for _, line := range script {
		args = append(args, "-e", line)
	}
	args = append(args, prompt, start)
	out, err := exec.CommandContext(ctx, "osascript", args...).CombinedOutput()
	if err != nil {
		if strings.Contains(string(out), "-128") {
			return "", ErrCanceled
		}
		return "", fmt.Errorf("osascript: %s", strings.TrimSpace(string(out)))
	}
	return strings.TrimRight(strings.TrimSpace(string(out)), "/"), nil
}

func pickWindows(ctx context.Context, prompt, start string) (string, error) {
	// Values are passed as environment variables so they are never parsed as PowerShell code.
	script := `Add-Type -AssemblyName System.Windows.Forms
$d = New-Object System.Windows.Forms.FolderBrowserDialog
$d.Description = $env:LB_PROMPT
$d.ShowNewFolderButton = $false
if ($env:LB_START) { $d.SelectedPath = $env:LB_START }
$owner = New-Object System.Windows.Forms.Form -Property @{TopMost = $true}
if ($d.ShowDialog($owner) -eq [System.Windows.Forms.DialogResult]::OK) { [Console]::Out.Write($d.SelectedPath) } else { exit 2 }`
	cmd := exec.CommandContext(ctx, "powershell", "-NoProfile", "-STA", "-Command", script)
	cmd.Env = append(cmd.Environ(), "LB_PROMPT="+prompt, "LB_START="+start)
	out, err := cmd.Output()
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) && exit.ExitCode() == 2 {
			return "", ErrCanceled
		}
		return "", fmt.Errorf("powershell: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

func pickLinux(ctx context.Context, prompt, start string) (string, error) {
	var cmd *exec.Cmd
	if p, err := exec.LookPath("zenity"); err == nil {
		args := []string{"--file-selection", "--directory", "--title=" + prompt}
		if start != "" {
			args = append(args, "--filename="+strings.TrimRight(start, "/")+"/")
		}
		cmd = exec.CommandContext(ctx, p, args...)
	} else if p, err := exec.LookPath("kdialog"); err == nil {
		if start == "" {
			start = "."
		}
		cmd = exec.CommandContext(ctx, p, "--getexistingdirectory", start, "--title", prompt)
	} else {
		return "", ErrUnsupported
	}
	out, err := cmd.Output()
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) && exit.ExitCode() == 1 {
			return "", ErrCanceled
		}
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
