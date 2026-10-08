package account

import "testing"

func TestNewNamedCredentialStoreRejectsBlankName(t *testing.T) {
	for _, name := range []string{"", "   ", "\t\n"} {
		store, err := NewNamedCredentialStore(name)
		if err == nil {
			t.Fatalf("NewNamedCredentialStore(%q) error = nil, want an error", name)
		}
		if store != nil {
			t.Fatalf("NewNamedCredentialStore(%q) returned a store together with an error", name)
		}
	}
}
