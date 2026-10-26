package runtime

import "testing"

func TestEnvSignature(t *testing.T) {
	sig := func(env map[string]string) string { return StartSessionRequest{Env: env}.EnvSignature() }
	if sig(nil) != "" || sig(map[string]string{}) != "" {
		t.Fatal("an empty env has no signature")
	}
	base := sig(map[string]string{"A": "1", "B": "2"})
	if base != sig(map[string]string{"B": "2", "A": "1"}) {
		t.Fatal("signature must not depend on map order")
	}
	for name, env := range map[string]map[string]string{
		"changed value": {"A": "1", "B": "3"},
		"added key":     {"A": "1", "B": "2", "C": ""},
		"removed key":   {"A": "1"},
		"shifted bytes": {"A": "1\x00B", "B": "2"},
	} {
		if sig(env) == base {
			t.Errorf("%s: signature did not change", name)
		}
	}
}
