//go:build !agentx_embed_web

package webdist

import "io/fs"

func FS() fs.FS {
	return nil
}
