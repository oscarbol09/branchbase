package proxy

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestKeyedMutexBasic(t *testing.T) {
	km := newKeyedMutex()
	if km.keyCount() != 0 {
		t.Fatalf("expected initial keyCount 0, got %d", km.keyCount())
	}

	unlock1 := km.Lock("branch-a")
	if km.keyCount() != 1 {
		t.Fatalf("expected keyCount 1 during lock, got %d", km.keyCount())
	}

	unlock1()
	if km.keyCount() != 0 {
		t.Fatalf("expected keyCount 0 after unlock, got %d", km.keyCount())
	}
}

func TestKeyedMutexDifferentKeys(t *testing.T) {
	km := newKeyedMutex()

	unlockA := km.Lock("branch-a")
	unlockB := km.Lock("branch-b")

	if km.keyCount() != 2 {
		t.Fatalf("expected keyCount 2 for distinct keys, got %d", km.keyCount())
	}

	unlockA()
	if km.keyCount() != 1 {
		t.Fatalf("expected keyCount 1 after unlocking A, got %d", km.keyCount())
	}

	unlockB()
	if km.keyCount() != 0 {
		t.Fatalf("expected keyCount 0 after unlocking B, got %d", km.keyCount())
	}
}

func TestKeyedMutexDoubleUnlock(t *testing.T) {
	km := newKeyedMutex()

	unlock := km.Lock("branch-x")
	unlock()
	// Calling unlock second time must not panic or cause negative refcount
	unlock()

	if km.keyCount() != 0 {
		t.Fatalf("expected keyCount 0, got %d", km.keyCount())
	}
}

func TestKeyedMutexMutualExclusion(t *testing.T) {
	km := newKeyedMutex()
	const n = 20
	var active int32
	var maxActive int32
	var wg sync.WaitGroup

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			unlock := km.Lock("shared-branch")
			defer unlock()

			cur := atomic.AddInt32(&active, 1)
			for {
				old := atomic.LoadInt32(&maxActive)
				if cur <= old || atomic.CompareAndSwapInt32(&maxActive, old, cur) {
					break
				}
			}

			time.Sleep(5 * time.Millisecond)
			atomic.AddInt32(&active, -1)
		}()
	}

	wg.Wait()

	if maxActive != 1 {
		t.Fatalf("expected mutual exclusion (maxActive == 1), got %d", maxActive)
	}

	if km.keyCount() != 0 {
		t.Fatalf("expected keyCount 0 after all unlocks, got %d", km.keyCount())
	}
}

func TestKeyedMutexStress(t *testing.T) {
	km := newKeyedMutex()
	const workers = 100
	const iterations = 50
	const numKeys = 5

	var counters [numKeys]int64
	var wg sync.WaitGroup
	wg.Add(workers)

	for w := 0; w < workers; w++ {
		go func(workerID int) {
			defer wg.Done()
			for it := 0; it < iterations; it++ {
				keyIndex := (workerID + it) % numKeys
				key := fmt.Sprintf("key-%d", keyIndex)

				unlock := km.Lock(key)
				// Critical section: increment counter
				counters[keyIndex]++
				unlock()
			}
		}(w)
	}

	wg.Wait()

	var total int64
	for _, count := range counters {
		total += count
	}

	expected := int64(workers * iterations)
	if total != expected {
		t.Fatalf("expected total %d increments, got %d", expected, total)
	}

	if km.keyCount() != 0 {
		t.Fatalf("expected keyCount 0 after stress test, got %d", km.keyCount())
	}
}
