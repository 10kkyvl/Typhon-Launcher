package metadata

import (
	"context"
	"errors"
	"testing"
	"typhon/internal/catalog"
)

type identityCatalogRemote struct{}

func (identityCatalogRemote) Browse(context.Context, catalog.GameQuery) (catalog.GamePage, error) {
	return catalog.GamePage{}, nil
}
func TestUnknownPersonalGameRequiresProviderConfirmation(t *testing.T) {
	cat, err := catalog.NewServiceAt(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	cat.SetRemoteCatalog(identityCatalogRemote{})
	svc := &Service{catalog: cat}
	_, err = svc.lookup(context.Background(), nil, catalog.Game{ID: "private", Title: "Prey"}, "", classUser)
	if !errors.Is(err, ErrAmbiguous) {
		t.Fatalf("unknown personal identity was looked up automatically: %v", err)
	}
}
