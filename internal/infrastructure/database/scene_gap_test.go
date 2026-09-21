package database

import (
	"context"
	"fmt"
	"sort"
	"testing"
	"time"

	"agro-sentinel-worker/internal/domain"
)

// TestListAllBySceneName_OnlyFromFirstGapOnward pins down which rows a run must
// touch, using eight scenes A..H in time order across five productions:
//
//	P1  complete A..H                  → never taken
//	P2  has A,B                        → C onward
//	P3  has A..D and also G,H          → E,F as work; G,H regenerated
//	P4  has A..F                       → G onward
//	P5  has nothing                    → all of A..H
//
// The rule being verified: a COMPLETED scene is only revisited when it sits at
// or after its own production's first gap, because params carry the historical
// chain forward. Scenes before the gap keep their chain intact and must not be
// regenerated.
func TestListAllBySceneName_OnlyFromFirstGapOnward(t *testing.T) {
	db := testDB(t)
	prodRepo := NewProductionRepo(db)
	repo := NewSceneRepo(db)
	ctx := context.Background()

	names := []string{"A", "B", "C", "D", "E", "F", "G", "H"}
	base := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)

	// completedUpTo lists, per production, which scenes already exist as
	// COMPLETED. Everything else for that production is PENDING.
	layout := map[string]map[string]bool{
		"P1": setOf("A", "B", "C", "D", "E", "F", "G", "H"),
		"P2": setOf("A", "B"),
		"P3": setOf("A", "B", "C", "D", "G", "H"),
		"P4": setOf("A", "B", "C", "D", "E", "F"),
		"P5": setOf(),
	}

	// Scene names are unique per run so repeated runs against the same test
	// database never collide.
	suffix := fmt.Sprintf("_%d", randomID())
	sceneName := func(n string) string { return n + suffix }

	monitoringIDs := map[string]uint{}
	for _, label := range []string{"P1", "P2", "P3", "P4", "P5"} {
		_, monitoringID := setupProductionWithMonitoringID(t, ctx, prodRepo)
		monitoringIDs[label] = monitoringID

		for i, n := range names {
			fecha := base.AddDate(0, 0, i)
			status := domain.StatusPending
			completed := layout[label][n]
			if completed {
				status = domain.StatusCompleted
			}
			s := &domain.Scene{
				MonitoringProduccionID: monitoringID,
				SceneName:              sceneName(n),
				Fecha:                  &fecha,
				Status:                 status,
				TruthTifExists:         completed,
			}
			if err := repo.Upsert(ctx, s); err != nil {
				t.Fatalf("%s scene %s: %v", label, n, err)
			}
		}
	}

	labelOf := map[uint]string{}
	for label, id := range monitoringIDs {
		labelOf[id] = label
	}

	// want[scene] = productions that must take part, with the role each plays.
	want := map[string]map[string]string{
		"A": {"P5": "PENDING"},
		"B": {"P5": "PENDING"},
		"C": {"P2": "PENDING", "P5": "PENDING"},
		"D": {"P2": "PENDING", "P5": "PENDING"},
		"E": {"P2": "PENDING", "P3": "PENDING", "P5": "PENDING"},
		"F": {"P2": "PENDING", "P3": "PENDING", "P5": "PENDING"},
		"G": {"P2": "PENDING", "P3": "COMPLETED", "P4": "PENDING", "P5": "PENDING"},
		"H": {"P2": "PENDING", "P3": "COMPLETED", "P4": "PENDING", "P5": "PENDING"},
	}

	for _, n := range names {
		scenes, err := repo.ListAllBySceneName(ctx, sceneName(n))
		if err != nil {
			t.Fatalf("ListAllBySceneName(%s): %v", n, err)
		}

		got := map[string]string{}
		for _, s := range scenes {
			label, ok := labelOf[s.MonitoringProduccionID]
			if !ok {
				continue // a production from another test run
			}
			got[label] = s.Status
		}

		if len(got) != len(want[n]) {
			t.Errorf("scene %s: got %v, want %v", n, sortedKeys(got), sortedKeys(want[n]))
			continue
		}
		for label, status := range want[n] {
			gotStatus, present := got[label]
			if !present {
				t.Errorf("scene %s: %s missing (expected as %s)", n, label, status)
				continue
			}
			if gotStatus != status {
				t.Errorf("scene %s: %s has status %s, want %s", n, label, gotStatus, status)
			}
		}
		if _, leaked := got["P1"]; leaked {
			t.Errorf("scene %s: P1 is fully complete and must never be taken", n)
		}
	}
}

func setOf(vals ...string) map[string]bool {
	m := make(map[string]bool, len(vals))
	for _, v := range vals {
		m[v] = true
	}
	return m
}

func sortedKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
