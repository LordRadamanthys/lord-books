package lordbooks

import (
	"embed"
	"io/fs"
)

//go:embed web/static
var webFiles embed.FS

// WebFS devolve a interface web embutida no binário.
func WebFS() fs.FS {
	sub, err := fs.Sub(webFiles, "web/static")
	if err != nil {
		panic(err)
	}
	return sub
}
