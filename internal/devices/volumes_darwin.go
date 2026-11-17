package devices

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func removableVolumes(ctx context.Context) ([]volume, error) {
	entries, err := os.ReadDir("/Volumes")
	if err != nil {
		return nil, err
	}
	var out []volume
	for _, e := range entries {
		mount := filepath.Join("/Volumes", e.Name())
		// The startup disk appears here as a symlink to "/".
		if target, err := os.Readlink(mount); err == nil && target == "/" {
			continue
		}
		info, err := exec.CommandContext(ctx, "diskutil", "info", mount).Output()
		if err != nil || !isRemovable(string(info)) {
			continue
		}
		out = append(out, volume{mount: mount, name: e.Name()})
	}
	return out, nil
}

// isRemovable reads `diskutil info`. Built-in SD readers report Internal but Removable;
// disk images (.dmg) are ejectable too and are skipped.
func isRemovable(info string) bool {
	field := func(key string) string {
		for _, line := range strings.Split(info, "\n") {
			k, v, ok := strings.Cut(strings.TrimSpace(line), ":")
			if ok && strings.TrimSpace(k) == key {
				return strings.TrimSpace(v)
			}
		}
		return ""
	}
	if field("Protocol") == "Disk Image" || field("Virtual") == "Yes" {
		return false
	}
	return field("Removable Media") == "Removable" || field("Device Location") == "External" || field("Ejectable") == "Yes"
}

func Eject(ctx context.Context, d Device) error {
	out, err := exec.CommandContext(ctx, "diskutil", "eject", d.Mount).CombinedOutput()
	if err != nil {
		return ejectError(out)
	}
	return nil
}
