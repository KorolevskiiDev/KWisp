package logstore

import (
	"testing"

	"github.com/KorolevskiiDev/KWisp/internal/domain"
	"github.com/KorolevskiiDev/KWisp/internal/repository/jsonl"
)

func TestStoreAppendAndRecent(t *testing.T) {
	dir := t.TempDir()
	store, err := jsonl.NewRepository(dir, 10)
	if err != nil {
		t.Fatalf("NewRepository: %v", err)
	}

	st, err := store.GetOrCreate("app_a")
	if err != nil {
		t.Fatalf("GetOrCreate: %v", err)
	}
	records := []domain.Record{
		{TsUnixMs: 1, Level: "info", Msg: "m", Application: "app-a"},
		{TsUnixMs: 2, Level: "info", Msg: "m", Application: "app-a"},
		{TsUnixMs: 3, Level: "info", Msg: "m", Application: "app-a"},
	}
	if err := store.Append(st, records); err != nil {
		t.Fatalf("Append: %v", err)
	}

	recs, err := store.Recent(st, 0)
	if err != nil {
		t.Fatalf("Recent: %v", err)
	}
	if len(recs) != 3 {
		t.Fatalf("Recent: got %d records, want 3", len(recs))
	}
	for i, r := range recs {
		if r.TsUnixMs != int64(i+1) {
			t.Errorf("record %d: ts = %d, want %d (oldest first)", i, r.TsUnixMs, i+1)
		}
	}

	// Recent(n) must return the NEWEST n, oldest first: with 8 records it
	// must yield 6 and 7, not 0 and 1 (regression: the window start was
	// once computed from the total count, returning the oldest records).
	for i := 4; i <= 8; i++ {
		if err := store.Append(st, []domain.Record{{TsUnixMs: int64(i)}}); err != nil {
			t.Fatalf("Append %d: %v", i, err)
		}
	}
	latest, err := store.Recent(st, 2)
	if err != nil {
		t.Fatalf("Recent(2): %v", err)
	}
	if len(latest) != 2 || latest[0].TsUnixMs != 7 || latest[1].TsUnixMs != 8 {
		t.Errorf("Recent(2) = %v, want [7 8] (newest two, oldest first)", latest)
	}
}

func TestStoreRingEviction(t *testing.T) {
	const capacity = 5
	dir := t.TempDir()
	store, err := jsonl.NewRepository(dir, capacity)
	if err != nil {
		t.Fatalf("NewRepository: %v", err)
	}
	st, err := store.GetOrCreate("app_a")
	if err != nil {
		t.Fatalf("GetOrCreate: %v", err)
	}

	for i := 0; i < capacity+3; i++ {
		if err := store.Append(st, []domain.Record{{TsUnixMs: int64(i)}}); err != nil {
			t.Fatalf("Append %d: %v", i, err)
		}
	}

	recs, err := store.Recent(st, 0)
	if err != nil {
		t.Fatalf("Recent: %v", err)
	}
	if len(recs) != capacity {
		t.Fatalf("Recent: got %d records, want %d (ring capacity)", len(recs), capacity)
	}
	if recs[0].TsUnixMs != 3 {
		t.Errorf("oldest kept ts = %d, want 3 (evicted first 3)", recs[0].TsUnixMs)
	}
	if recs[len(recs)-1].TsUnixMs != capacity+2 {
		t.Errorf("newest ts = %d, want %d", recs[len(recs)-1].TsUnixMs, capacity+2)
	}
}

func TestStoreSurvivesRestart(t *testing.T) {
	dir := t.TempDir()

	store1, err := jsonl.NewRepository(dir, 10)
	if err != nil {
		t.Fatalf("NewRepository: %v", err)
	}
	st1, err := store1.GetOrCreate("app_a")
	if err != nil {
		t.Fatalf("GetOrCreate: %v", err)
	}
	if err := store1.Append(st1, []domain.Record{{TsUnixMs: 42, Level: "error", Msg: "boom", Application: "app-a"}}); err != nil {
		t.Fatalf("Append: %v", err)
	}

	// "Restart": a fresh store over the same directory.
	store2, err := jsonl.NewRepository(dir, 10)
	if err != nil {
		t.Fatalf("NewRepository (restart): %v", err)
	}
	st2, err := store2.Get("app_a")
	if err != nil {
		t.Fatal("stream not rebuilt after restart")
	}
	if st2.Key != st1.Key {
		t.Errorf("api key changed across restart")
	}

	recs, err := store2.Recent(st2, 0)
	if err != nil {
		t.Fatalf("Recent: %v", err)
	}
	if len(recs) != 1 || recs[0].TsUnixMs != 42 || recs[0].Msg != "boom" {
		t.Errorf("records after restart = %+v, want the appended record", recs)
	}
}

func TestStoreRecentFiltered(t *testing.T) {
	dir := t.TempDir()
	store, err := jsonl.NewRepository(dir, 10)
	if err != nil {
		t.Fatalf("NewRepository: %v", err)
	}
	st, err := store.GetOrCreate("app_a")
	if err != nil {
		t.Fatalf("GetOrCreate: %v", err)
	}

	instanceIDs := []string{"i1", "i2", "i1", "i2", "i1"}
	for i, id := range instanceIDs {
		if err := store.Append(st, []domain.Record{{TsUnixMs: int64(i + 1), InstanceID: id}}); err != nil {
			t.Fatalf("Append %d: %v", i, err)
		}
	}

	got, err := store.RecentFiltered(st, "i1", 0)
	if err != nil {
		t.Fatalf("RecentFiltered: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("i1 matches = %d, want 3", len(got))
	}
	for i, r := range got {
		if r.InstanceID != "i1" {
			t.Fatalf("match %d instance = %q, want i1", i, r.InstanceID)
		}
	}
	if got[0].TsUnixMs != 1 || got[2].TsUnixMs != 5 {
		t.Errorf("order = %v, want oldest-first [1 3 5]", []int64{got[0].TsUnixMs, got[1].TsUnixMs, got[2].TsUnixMs})
	}

	// Limited to the newest 2 matches (ts 3 and 5).
	got, err = store.RecentFiltered(st, "i1", 2)
	if err != nil {
		t.Fatalf("RecentFiltered(i1, 2): %v", err)
	}
	if len(got) != 2 || got[0].TsUnixMs != 3 || got[1].TsUnixMs != 5 {
		t.Errorf("RecentFiltered(i1, 2) = %v, want [3 5]", []int64{got[0].TsUnixMs, got[1].TsUnixMs})
	}

	got, err = store.RecentFiltered(st, "nope", 0)
	if err != nil {
		t.Fatalf("RecentFiltered(nope): %v", err)
	}
	if len(got) != 0 {
		t.Errorf("unknown instance matches = %d, want 0", len(got))
	}
}

func TestStoreSubscribe(t *testing.T) {
	dir := t.TempDir()
	store, err := jsonl.NewRepository(dir, 10)
	if err != nil {
		t.Fatalf("NewRepository: %v", err)
	}
	st, err := store.GetOrCreate("app_a")
	if err != nil {
		t.Fatalf("GetOrCreate: %v", err)
	}

	ch, unsubscribe, err := store.Subscribe(st)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	if err := store.Append(st, []domain.Record{{Msg: "hello"}}); err != nil {
		t.Fatalf("Append: %v", err)
	}
	if rec := <-ch; rec.Msg != "hello" {
		t.Errorf("received %q, want %q", rec.Msg, "hello")
	}

	unsubscribe()
	if _, ok := <-ch; ok {
		t.Error("subscriber channel not closed after unsubscribe")
	}
}

func TestStoreStreamIdempotent(t *testing.T) {
	dir := t.TempDir()
	store, err := jsonl.NewRepository(dir, 10)
	if err != nil {
		t.Fatalf("NewRepository: %v", err)
	}

	st1, err := store.GetOrCreate("app_a")
	if err != nil {
		t.Fatalf("GetOrCreate: %v", err)
	}
	st2, err := store.GetOrCreate("app_a")
	if err != nil {
		t.Fatalf("GetOrCreate (repeat): %v", err)
	}
	if st2.Name != st1.Name {
		t.Error("repeat GetOrCreate returned a different stream")
	}
	if st2.Key != st1.Key {
		t.Error("repeat GetOrCreate returned a different key")
	}
}
