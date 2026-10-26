package runtime

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
)

// EnvSignature identifies the agent environment a persistent process was
// launched with. Environment variables only apply at process start, so the
// persistent runtimes fold this into their pool signature to restart an idle
// process when the agent's env changes.
func (r StartSessionRequest) EnvSignature() string {
	if len(r.Env) == 0 {
		return ""
	}
	keys := make([]string, 0, len(r.Env))
	for key := range r.Env {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	hash := sha256.New()
	for _, key := range keys {
		hash.Write([]byte(key))
		hash.Write([]byte{0})
		hash.Write([]byte(r.Env[key]))
		hash.Write([]byte{0})
	}
	return hex.EncodeToString(hash.Sum(nil))
}
