package main

import (
	"fmt"
	"path/filepath"
	"testing"
)

type dummyConn struct{}

func (d *dummyConn) WriteBinary(p []byte) error { return nil }
func (d *dummyConn) Close() error               { return nil }

func BenchmarkRoomIsWalkable(b *testing.B) {
	room := &Room{
		ID:           "bench_room",
		Layout:       5,
		walkableGrid: make(map[[2]int]bool),
	}
	for x := 0; x < 30; x++ {
		for z := 0; z < 30; z++ {
			if (x+z)%2 == 0 {
				room.walkableGrid[[2]int{x, z}] = true
			}
		}
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		x, z := 10, 10
		for pb.Next() {
			_ = room.IsWalkable(x, z)
			x = (x + 1) % 30
		}
	})
}

func BenchmarkRoomBroadcast50Players(b *testing.B) {
	room := &Room{
		ID:      "bench_room",
		Players: make(map[string]*Player),
	}
	for i := 0; i < 50; i++ {
		p := &Player{
			ID:   fmt.Sprintf("player_%d", i),
			Conn: &dummyConn{},
		}
		room.Players[p.ID] = p
	}

	packet := `<AV_MV id="player_0" x="12" z="14" dir="2"/>`
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		room.Broadcast(packet, "")
	}
}

func BenchmarkRoomBroadcast100Players(b *testing.B) {
	room := &Room{
		ID:      "bench_room",
		Players: make(map[string]*Player),
	}
	for i := 0; i < 100; i++ {
		p := &Player{
			ID:   fmt.Sprintf("player_%d", i),
			Conn: &dummyConn{},
		}
		room.Players[p.ID] = p
	}

	packet := `<CH sender="player_0" text="Welcome to Coke Studios London I!"/>`
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		room.Broadcast(packet, "")
	}
}

func BenchmarkPathMapLoadCached(b *testing.B) {
	repoRoot := findRepoRoot()
	webRoot := filepath.Join(repoRoot, "sources", "gocoke", "web")

	room := &Room{
		ID:     "$SYSTEM$1",
		Name:   "London I",
		Layout: 5,
	}
	room.LoadPathMap(webRoot)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		room.LoadPathMap(webRoot)
	}
}

func BenchmarkDatabaseDenizenRegistration(b *testing.B) {
	dbPath := filepath.Join(b.TempDir(), "bench.db")
	store, err := OpenStore(dbPath)
	if err != nil {
		b.Fatalf("failed to open store: %v", err)
	}
	defer store.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		name := fmt.Sprintf("Denizen_%d", i)
		_, err := store.RegisterDenizen(name, "password123", "1", "avatar_string", "Coke Music")
		if err != nil {
			b.Fatalf("registration failed: %v", err)
		}
	}
}

func BenchmarkCatalogueLookup(b *testing.B) {
	repoRoot := findRepoRoot()
	webRoot := filepath.Join(repoRoot, "sources", "gocoke", "web")
	srv := &Server{webRoot: webRoot}
	srv.InitCatalog()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = srv.GetCatalogItem(101)
		_, _ = srv.GetCatalogItem(9999)
	}
}

func BenchmarkDatabaseAtomicPurchase(b *testing.B) {
	dbPath := filepath.Join(b.TempDir(), "bench_purchase.db")
	store, err := OpenStore(dbPath)
	if err != nil {
		b.Fatalf("failed to open store: %v", err)
	}
	defer store.Close()

	name := "Buyer_Bench"
	_, err = store.RegisterDenizen(name, "pass", "1", "def", "Mission")
	if err != nil {
		b.Fatalf("failed to register buyer: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		idem := fmt.Sprintf("idem_%d", i)
		_, _, err := store.PurchaseItemAtomic(name, 101, 1, map[string]any{"color": "red"}, idem)
		if err != nil {
			b.Fatalf("purchase failed: %v", err)
		}
	}
}
