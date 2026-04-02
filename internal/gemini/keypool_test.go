package gemini

import (
	"testing"
	"time"
)

func TestNewKeyPool(t *testing.T) {
	pool := NewKeyPool([]string{"k1", "k2", "k3"}, 120*time.Second)
	if pool == nil {
		t.Fatal("pool is nil")
	}
	if len(pool.keys) != 3 {
		t.Errorf("keys len = %d, want 3", len(pool.keys))
	}
}

func TestGetKey_ReturnsAvailable(t *testing.T) {
	pool := NewKeyPool([]string{"k1", "k2"}, 120*time.Second)
	key, err := pool.GetKey()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if key != "k1" && key != "k2" {
		t.Errorf("unexpected key: %q", key)
	}
}

func TestGetKey_WeightedByUsage(t *testing.T) {
	pool := NewKeyPool([]string{"k1", "k2"}, 120*time.Second)
	for i := 0; i < 100; i++ {
		pool.Release("k1")
	}

	counts := map[string]int{}
	for i := 0; i < 1000; i++ {
		key, _ := pool.GetKey()
		counts[key]++
	}
	if counts["k2"] <= counts["k1"] {
		t.Errorf("k2 should be picked more often: k1=%d, k2=%d", counts["k1"], counts["k2"])
	}
}

func TestMarkCooldown_ExcludesKey(t *testing.T) {
	pool := NewKeyPool([]string{"k1", "k2"}, 2*time.Second)
	pool.MarkCooldown("k1")

	for i := 0; i < 50; i++ {
		key, err := pool.GetKey()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if key == "k1" {
			t.Fatal("got cooled down key k1")
		}
	}
}

func TestMarkCooldown_AllKeys_WaitsForRecovery(t *testing.T) {
	pool := NewKeyPool([]string{"k1"}, 100*time.Millisecond)
	pool.MarkCooldown("k1")

	key, err := pool.GetKey()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if key != "k1" {
		t.Errorf("expected k1 after cooldown, got %q", key)
	}
}

func TestRelease_IncrementsUsage(t *testing.T) {
	pool := NewKeyPool([]string{"k1"}, 120*time.Second)
	pool.Release("k1")
	pool.Release("k1")

	stats := pool.Stats()
	if stats["k1"] != 2 {
		t.Errorf("usage = %d, want 2", stats["k1"])
	}
}

func TestGetKey_EmptyPool(t *testing.T) {
	pool := NewKeyPool([]string{}, 120*time.Second)
	_, err := pool.GetKey()
	if err == nil {
		t.Fatal("expected error for empty pool")
	}
}
