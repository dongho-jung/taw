package memoization

import (
	"testing"
)

func TestNewMemoCache(t *testing.T) {
	cache := NewMemoCache[HString, string](10)
	if cache == nil {
		t.Fatal("expected non-nil cache")
	}
	if cache.Capacity() != 10 {
		t.Errorf("expected capacity 10, got %d", cache.Capacity())
	}
	if cache.Size() != 0 {
		t.Errorf("expected size 0, got %d", cache.Size())
	}
}

func TestMemoCache_SetAndGet(t *testing.T) {
	cache := NewMemoCache[HString, string](10)

	// Set a value
	cache.Set(HString("key1"), "value1")

	// Get the value
	val, found := cache.Get(HString("key1"))
	if !found {
		t.Error("expected to find key1")
	}
	if val != "value1" {
		t.Errorf("expected value1, got %s", val)
	}

	// Get non-existent key
	_, found = cache.Get(HString("nonexistent"))
	if found {
		t.Error("expected not to find nonexistent key")
	}
}

func TestMemoCache_Update(t *testing.T) {
	cache := NewMemoCache[HString, string](10)

	// Set initial value
	cache.Set(HString("key1"), "value1")

	// Update the value
	cache.Set(HString("key1"), "value2")

	// Verify update
	val, found := cache.Get(HString("key1"))
	if !found {
		t.Error("expected to find key1")
	}
	if val != "value2" {
		t.Errorf("expected value2, got %s", val)
	}

	// Size should still be 1
	if cache.Size() != 1 {
		t.Errorf("expected size 1, got %d", cache.Size())
	}
}

func TestMemoCache_LRUEviction(t *testing.T) {
	cache := NewMemoCache[HString, string](3)

	// Fill the cache
	cache.Set(HString("key1"), "value1")
	cache.Set(HString("key2"), "value2")
	cache.Set(HString("key3"), "value3")

	if cache.Size() != 3 {
		t.Errorf("expected size 3, got %d", cache.Size())
	}

	// Add one more item, should evict key1 (LRU)
	cache.Set(HString("key4"), "value4")

	if cache.Size() != 3 {
		t.Errorf("expected size 3 after eviction, got %d", cache.Size())
	}

	// key1 should be evicted
	_, found := cache.Get(HString("key1"))
	if found {
		t.Error("expected key1 to be evicted")
	}

	// key2, key3, key4 should still exist
	_, found = cache.Get(HString("key2"))
	if !found {
		t.Error("expected key2 to still exist")
	}
	_, found = cache.Get(HString("key3"))
	if !found {
		t.Error("expected key3 to still exist")
	}
	_, found = cache.Get(HString("key4"))
	if !found {
		t.Error("expected key4 to exist")
	}
}

func TestMemoCache_LRUAccess(t *testing.T) {
	cache := NewMemoCache[HString, string](3)

	// Fill the cache
	cache.Set(HString("key1"), "value1")
	cache.Set(HString("key2"), "value2")
	cache.Set(HString("key3"), "value3")

	// Access key1 to make it recently used
	cache.Get(HString("key1"))

	// Add key4, should evict key2 (now LRU)
	cache.Set(HString("key4"), "value4")

	// key2 should be evicted
	_, found := cache.Get(HString("key2"))
	if found {
		t.Error("expected key2 to be evicted")
	}

	// key1 should still exist (was accessed recently)
	_, found = cache.Get(HString("key1"))
	if !found {
		t.Error("expected key1 to still exist")
	}
}

func TestHString_Hash(t *testing.T) {
	h1 := HString("test")
	h2 := HString("test")
	h3 := HString("different")

	// Same string should produce same hash
	if h1.Hash() != h2.Hash() {
		t.Error("expected same hash for same string")
	}

	// Different strings should produce different hashes
	if h1.Hash() == h3.Hash() {
		t.Error("expected different hash for different strings")
	}

	// Hash should be non-empty (FNV-64 produces up to 16 hex characters)
	if len(h1.Hash()) == 0 {
		t.Error("expected non-empty hash")
	}
}

func TestHInt_Hash(t *testing.T) {
	h1 := HInt(42)
	h2 := HInt(42)
	h3 := HInt(100)

	// Same int should produce same hash
	if h1.Hash() != h2.Hash() {
		t.Error("expected same hash for same int")
	}

	// Different ints should produce different hashes
	if h1.Hash() == h3.Hash() {
		t.Error("expected different hash for different ints")
	}

	// Hash should be non-empty (FNV-64 produces up to 16 hex characters)
	if len(h1.Hash()) == 0 {
		t.Error("expected non-empty hash")
	}
}

func TestMemoCache_Concurrent(t *testing.T) {
	cache := NewMemoCache[HInt, int](100)

	// Test concurrent access
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func(n int) {
			for j := 0; j < 100; j++ {
				cache.Set(HInt(n*100+j), n*100+j)
				cache.Get(HInt(n * 100))
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Should not panic and should have reasonable size
	if cache.Size() > 100 {
		t.Errorf("expected size <= 100, got %d", cache.Size())
	}
}
