package main

import (
	"path/filepath"
	"testing"
)

func TestPurchaseUsesRequestedItemAndPrice(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "economy_test.db")
	store, err := OpenStore(dbPath)
	if err != nil {
		t.Fatalf("failed to open store: %v", err)
	}
	defer store.Close()

	srv := NewServer("./web", "127.0.0.1", "8080", store)

	// Create user with 5000 starting dB
	d, err := srv.store.GetOrCreateDenizen("Shopper", "1", "avatar_shopper", "Shopping spree")
	if err != nil {
		t.Fatalf("failed to create denizen: %v", err)
	}

	initialBal, _, _ := srv.store.GetBalance(d.ScreenName)
	if initialBal != 5000 {
		t.Fatalf("expected initial balance 5000 dB, got %d", initialBal)
	}

	// 1. Purchase Item 13: 5001 Sofa (evidenced price: 625 dB, NOT hardcoded 25 dB!)
	sofaItem, err := srv.GetCatalogItem(13)
	if err != nil {
		t.Fatalf("expected item 13 to exist in catalogue: %v", err)
	}
	if sofaItem.Price != 625 {
		t.Fatalf("expected 5001 Sofa price 625 dB, got %d", sofaItem.Price)
	}

	res1, err := srv.Purchase(d.ScreenName, 13, map[string]any{"color": 1}, "")
	if err != nil {
		t.Fatalf("failed to purchase 5001 Sofa: %v", err)
	}
	if len(res1) == 0 {
		t.Fatalf("expected purchase result array, got empty")
	}

	balAfter1, _, _ := srv.store.GetBalance(d.ScreenName)
	expectedBal1 := 5000 - 625 // 4375 dB
	if balAfter1 != expectedBal1 {
		t.Fatalf("expected balance after sofa purchase %d dB, got %d dB (failed: was hardcoded 25 dB used?)", expectedBal1, balAfter1)
	}

	// 2. Purchase Item 26: Blank CD (evidenced price: 50 dB)
	res2, err := srv.Purchase(d.ScreenName, 26, nil, "")
	if err != nil {
		t.Fatalf("failed to purchase Blank CD: %v", err)
	}
	if len(res2) == 0 {
		t.Fatalf("expected purchase result for blank CD")
	}

	balAfter2, _, _ := srv.store.GetBalance(d.ScreenName)
	expectedBal2 := expectedBal1 - 50 // 4325 dB
	if balAfter2 != expectedBal2 {
		t.Fatalf("expected balance %d dB, got %d dB", expectedBal2, balAfter2)
	}

	// 3. Purchase Item 45: Coke Cooler (evidenced price: 200 dB, NOT 25 dB)
	res3, err := srv.Purchase(d.ScreenName, 45, nil, "")
	if err != nil {
		t.Fatalf("failed to purchase Coke Cooler: %v", err)
	}
	if len(res3) == 0 {
		t.Fatalf("expected purchase result for Coke Cooler")
	}

	balAfter3, _, _ := srv.store.GetBalance(d.ScreenName)
	expectedBal3 := expectedBal2 - 200 // 4125 dB
	if balAfter3 != expectedBal3 {
		t.Fatalf("expected balance %d dB, got %d dB", expectedBal3, balAfter3)
	}

	// Verify backpack contains all 3 purchased items with correct catalog IDs
	bp, err := srv.store.GetBackpack(d.ScreenName)
	if err != nil {
		t.Fatalf("failed to fetch backpack: %v", err)
	}
	if len(bp) != 3 {
		t.Fatalf("expected 3 items in backpack, got %d", len(bp))
	}

	foundSofa := false
	foundCD := false
	foundCooler := false
	for _, it := range bp {
		if it.CatalogItemID == 13 {
			foundSofa = true
		}
		if it.CatalogItemID == 26 {
			foundCD = true
		}
		if it.CatalogItemID == 45 {
			foundCooler = true
		}
	}
	if !foundSofa || !foundCD || !foundCooler {
		t.Fatalf("backpack missing items: sofa=%v, cd=%v, cooler=%v", foundSofa, foundCD, foundCooler)
	}
}

func TestPurchaseInsufficientFunds(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "insufficient_funds_test.db")
	store, err := OpenStore(dbPath)
	if err != nil {
		t.Fatalf("failed to open store: %v", err)
	}
	defer store.Close()

	srv := NewServer("./web", "127.0.0.1", "8080", store)

	d, _ := srv.store.GetOrCreateDenizen("BrokePlayer", "1", "avatar_broke", "No funds")
	// Set balance to exactly 50 dB
	_, _ = srv.store.db.Exec("UPDATE denizens SET decibels = 50 WHERE id = ?", d.ID)

	bal, _, _ := srv.store.GetBalance(d.ScreenName)
	if bal != 50 {
		t.Fatalf("expected balance 50 dB, got %d", bal)
	}

	// Attempt to purchase 5001 Sofa (price: 625 dB)
	_, err = srv.Purchase(d.ScreenName, 13, nil, "")
	if err == nil {
		t.Fatalf("expected purchase to fail due to insufficient funds, but succeeded")
	}
	if err != ErrInsufficientFunds {
		t.Fatalf("expected ErrInsufficientFunds, got: %v", err)
	}

	// Balance must remain unchanged
	balAfter, _, _ := srv.store.GetBalance(d.ScreenName)
	if balAfter != 50 {
		t.Fatalf("expected balance to remain 50 dB, got %d", balAfter)
	}

	// Backpack must remain empty
	bp, err := srv.store.GetBackpack(d.ScreenName)
	if err != nil {
		t.Fatalf("failed to fetch backpack: %v", err)
	}
	if len(bp) != 0 {
		t.Fatalf("expected 0 items in backpack, got %d", len(bp))
	}
}

func TestPurchaseRetryDoesNotDoubleCharge(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "idempotency_test.db")
	store, err := OpenStore(dbPath)
	if err != nil {
		t.Fatalf("failed to open store: %v", err)
	}
	defer store.Close()

	srv := NewServer("./web", "127.0.0.1", "8080", store)

	d, _ := srv.store.GetOrCreateDenizen("RetryShopper", "1", "avatar_retry", "Testing retry")
	_, _ = srv.store.db.Exec("UPDATE denizens SET decibels = 1000 WHERE id = ?", d.ID)

	reqID := "client-tx-uuid-9988-7766"

	// First attempt
	res1, err := srv.Purchase(d.ScreenName, 13, map[string]any{"color": "red"}, reqID)
	if err != nil {
		t.Fatalf("first purchase attempt failed: %v", err)
	}
	bal1, _, _ := srv.store.GetBalance(d.ScreenName)
	if bal1 != 375 { // 1000 - 625 = 375
		t.Fatalf("expected balance 375 dB, got %d", bal1)
	}

	posID1 := res1[0].(map[string]any)["id"].(float64)

	// Second attempt with exact same idempotency key (simulated network retry)
	res2, err := srv.Purchase(d.ScreenName, 13, map[string]any{"color": "red"}, reqID)
	if err != nil {
		t.Fatalf("retry purchase attempt failed: %v", err)
	}
	bal2, _, _ := srv.store.GetBalance(d.ScreenName)
	if bal2 != 375 {
		t.Fatalf("expected balance to remain 375 dB on retry, got %d dB (DOUBLE CHARGE BUG!)", bal2)
	}

	posID2 := res2[0].(map[string]any)["id"].(float64)
	if posID1 != posID2 {
		t.Fatalf("expected retry to return identical possession ID %v, got %v", posID1, posID2)
	}

	// Verify only 1 item in backpack
	bp, _ := srv.store.GetBackpack(d.ScreenName)
	if len(bp) != 1 {
		t.Fatalf("expected exactly 1 item in backpack, got %d", len(bp))
	}
}

func TestPrePurchaseReturnsEvidencedPriceAndBalance(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "prepurchase_test.db")
	store, err := OpenStore(dbPath)
	if err != nil {
		t.Fatalf("failed to open store: %v", err)
	}
	defer store.Close()

	srv := NewServer("./web", "127.0.0.1", "8080", store)

	d, _ := srv.store.GetOrCreateDenizen("PreBuyer", "1", "avatar_pre", "")
	_, _ = srv.store.db.Exec("UPDATE denizens SET decibels = 1200 WHERE id = ?", d.ID)

	pre, err := srv.PrePurchase(d.ScreenName, 13, "[#color:2]")
	if err != nil {
		t.Fatalf("PrePurchase failed: %v", err)
	}

	if pre["balance"].(float64) != 1200 {
		t.Errorf("expected balance 1200, got %v", pre["balance"])
	}
	if pre["price"].(float64) != 625 {
		t.Errorf("expected price 625, got %v", pre["price"])
	}
	if pre["catalogItemId"].(float64) != 13 {
		t.Errorf("expected catalogItemId 13, got %v", pre["catalogItemId"])
	}
}
