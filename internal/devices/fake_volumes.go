//go:build lbfake

package devices

import (
	"context"
	"os"
	"path/filepath"
	"strings"
)

// Test-only build (go build -tags lbfake): LB_FAKE_VOLUMES lists folders, separated by
// the OS path-list separator, that are reported as mounted removable volumes.
func init() {
	listVolumes = func(context.Context) ([]volume, error) {
		var out []volume
		for _, p := range filepath.SplitList(os.Getenv("LB_FAKE_VOLUMES")) {
			if p = strings.TrimSpace(p); p != "" {
				out = append(out, volume{mount: p, name: filepath.Base(p)})
			}
		}
		return out, nil
	}
}
