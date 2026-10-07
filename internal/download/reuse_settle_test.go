package download

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
)

// The engine publishes the result of a piece check a moment after the check
// itself returns, and the last pieces of a pass are still in that moment when
// the report is read. A fresh manager every round keeps the completion state
// empty, which is what makes the window show.
func TestInspectReuseOfIntactDataNeverReportsDamage(t *testing.T) {
	const rounds = 100
	for round := 0; round < rounds; round++ {
		m, hash, parent := reuseManager(t, gameFiles)

		report, err := m.InspectReuse(t.Context(), ReuseRequest{InfoHash: hash, Path: parent}, nil)
		if err != nil {
			t.Fatalf("round %d: InspectReuse: %v", round, err)
		}

		if report.BadPieces != 0 || report.OkPieces != report.TotalPieces {
			t.Fatalf("round %d: ok/bad/total pieces = %d/%d/%d on data nobody touched", round, report.OkPieces, report.BadPieces, report.TotalPieces)
		}
		if report.MatchedBytes != report.TotalBytes {
			t.Fatalf("round %d: matched %d of %d bytes on data nobody touched", round, report.MatchedBytes, report.TotalBytes)
		}
	}
}

func TestInspectReuseOfDamagedDataStillCountsEveryBadPiece(t *testing.T) {
	m, hash, parent := reuseManager(t, gameFiles)
	flipByte(t, filepath.Join(parent, "Game"), "d2.dat", 100)
	flipByte(t, filepath.Join(parent, "Game"), "d4.dat", 5000)

	report, err := m.InspectReuse(t.Context(), ReuseRequest{InfoHash: hash, Path: parent}, nil)
	if err != nil {
		t.Fatal(err)
	}

	if report.BadPieces != 2 || report.OkPieces != report.TotalPieces-2 {
		t.Fatalf("ok/bad/total pieces = %d/%d/%d, want exactly the two damaged pieces to be bad", report.OkPieces, report.BadPieces, report.TotalPieces)
	}
}

func TestSettlePiecesGivesUpWhenTheCallerDoes(t *testing.T) {
	mi, parent := buildTorrent(t, "Game", gameFiles)
	m := managerWithClient(t, 1)
	lt, err := m.client.addMetainfo(mi, parent, storageOpts{inPlace: true})
	if err != nil {
		t.Fatal(err)
	}
	defer lt.drop()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if err := lt.settlePieces(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("settlePieces = %v, want context.Canceled", err)
	}
}

func TestSettlePiecesReportsAClosedTorrent(t *testing.T) {
	mi, parent := buildTorrent(t, "Game", gameFiles)
	m := managerWithClient(t, 1)
	lt, err := m.client.addMetainfo(mi, parent, storageOpts{inPlace: true})
	if err != nil {
		t.Fatal(err)
	}
	lt.drop()

	if err := lt.settlePieces(t.Context()); !errors.Is(err, errNetworkDown) {
		t.Fatalf("settlePieces = %v, want errNetworkDown", err)
	}
}
