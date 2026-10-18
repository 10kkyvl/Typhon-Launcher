package updates

import (
	"context"
	"errors"
	"testing"

	"typhon/internal/library"
)

func externalUpdatesService(t *testing.T) *Service {
	t.Helper()
	svc, err := newServiceAt(t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	svc.library = &fakeLibrary{games: []library.Game{
		{ID: "plain", Title: "Plain", InstallDir: t.TempDir()},
		{
			ID:         "ext",
			Title:      "Steam Game",
			InstallDir: t.TempDir(),
			SourceID:   "src",
			ReleaseID:  "rel",
			External:   &library.External{Provider: library.ProviderSteam, AppID: "42", LaunchURI: "steam://rungameid/42"},
		},
	}}
	svc.releases = &fakeReleases{}
	return svc
}

func TestCheckAllSkipsExternalGames(t *testing.T) {
	svc := externalUpdatesService(t)
	svc.checkAll(context.Background())

	var ids []string
	for _, u := range svc.GetUpdates() {
		ids = append(ids, u.GameID)
	}
	if len(ids) != 1 || ids[0] != "plain" {
		t.Fatalf("tracked games = %v, want only the managed one", ids)
	}
}

func TestExternalGamesRefuseUpdateOperations(t *testing.T) {
	svc := externalUpdatesService(t)

	if _, err := svc.CheckGame("ext"); !errors.Is(err, errExternalGame) {
		t.Fatalf("CheckGame error = %v, want errExternalGame", err)
	}
	if err := svc.PreparePlan("ext"); !errors.Is(err, errExternalGame) {
		t.Fatalf("PreparePlan error = %v, want errExternalGame", err)
	}
	if _, err := svc.planInputFor(context.Background(), "ext"); !errors.Is(err, errExternalGame) {
		t.Fatalf("planInputFor error = %v, want errExternalGame", err)
	}
	if _, ok := svc.installedGame("ext"); ok {
		t.Fatal("installedGame returned an external game")
	}
	if _, err := svc.CheckGame("plain"); err != nil {
		t.Fatalf("CheckGame on a managed game: %v", err)
	}
}
