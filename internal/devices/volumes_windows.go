package devices

import (
	"context"
	"os/exec"
	"strings"
)

// removableVolumes asks Windows for removable drives (DriveType 2), which is how
// Kindle, Kobo and SD cards appear.
func removableVolumes(ctx context.Context) ([]volume, error) {
	script := `Get-CimInstance Win32_LogicalDisk -Filter "DriveType=2" | ForEach-Object { "$($_.DeviceID)|$($_.VolumeName)" }`
	out, err := exec.CommandContext(ctx, "powershell", "-NoProfile", "-Command", script).Output()
	if err != nil {
		return nil, err
	}
	var vols []volume
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		drive, name, ok := strings.Cut(strings.TrimSpace(line), "|")
		if !ok || drive == "" {
			continue
		}
		if name == "" {
			name = drive
		}
		vols = append(vols, volume{mount: drive + `\`, name: name})
	}
	return vols, nil
}

func Eject(ctx context.Context, d Device) error {
	// The drive letter goes through an environment variable, never into the script text.
	script := `$s = New-Object -ComObject Shell.Application; $s.Namespace(17).ParseName($env:LB_DRIVE).InvokeVerb("Eject")`
	cmd := exec.CommandContext(ctx, "powershell", "-NoProfile", "-Command", script)
	cmd.Env = append(cmd.Environ(), "LB_DRIVE="+strings.TrimSuffix(d.Mount, `\`))
	if out, err := cmd.CombinedOutput(); err != nil {
		return ejectError(out)
	}
	return nil
}
