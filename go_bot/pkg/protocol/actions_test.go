package protocol

import "testing"

func TestAllCanonicalActionsAndAliases(t *testing.T) {
	seen := make(map[string]bool)
	for _, name := range Actions {
		if seen[name] {
			t.Fatalf("duplicate action %s", name)
		}
		seen[name] = true
		if action, ok := Canonical(name); !ok || action != name {
			t.Fatalf("canonical action not recognized: %s", name)
		}
	}
	for alias, want := range aliases {
		if !seen[want] {
			t.Fatalf("alias %s references unknown %s", alias, want)
		}
		if got, ok := Canonical(alias); !ok || got != want {
			t.Fatalf("alias mismatch: %s -> %s", alias, got)
		}
	}
	for _, name := range OneBotActions {
		if canonical, ok := Canonical(name); !ok || canonical == "" {
			t.Errorf("OneBot action not recognized: %s => %s", name, canonical)
		}
	}
	for _, name := range []string{"", "sendRaw", "send_raw", "definitelyUnknown"} {
		if _, ok := Canonical(name); ok {
			t.Fatalf("unsafe/unknown action accepted: %s", name)
		}
	}
}
