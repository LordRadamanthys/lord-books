package devices

import (
	"bufio"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// removableVolumes lists desktop automounts (udisks mounts removable media under
// /media/<user> or /run/media/<user>).
func removableVolumes(ctx context.Context) ([]volume, error) {
	f, err := os.Open("/proc/self/mounts")
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []volume
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) < 2 {
			continue
		}
		mount := unescapeMount(fields[1])
		if strings.HasPrefix(mount, "/media/") || strings.HasPrefix(mount, "/run/media/") {
			out = append(out, volume{mount: mount, name: filepath.Base(mount)})
		}
	}
	return out, sc.Err()
}

// /proc/mounts encodes spaces and a few other bytes as octal escapes.
func unescapeMount(s string) string {
	r := strings.NewReplacer(`\040`, " ", `\011`, "\t", `\012`, "\n", `\134`, `\`)
	return r.Replace(s)
}

func Eject(ctx context.Context, d Device) error {
	if p, err := exec.LookPath("gio"); err == nil {
		if out, err := exec.CommandContext(ctx, p, "mount", "-e", d.Mount).CombinedOutput(); err != nil {
			return ejectError(out)
		}
		return nil
	}
	if out, err := exec.CommandContext(ctx, "umount", d.Mount).CombinedOutput(); err != nil {
		return ejectError(out)
	}
	return nil
}
