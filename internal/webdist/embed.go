//go:build agentx_embed_web

package webdist

import (
	"embed"
	"io/fs"
)

//go:embed dist
var embedded embed.FS

func FS() fs.FS {
	dist, err := fs.Sub(embedded, "dist")
	if err != nil {
		return nil
	}
	return dist
}
