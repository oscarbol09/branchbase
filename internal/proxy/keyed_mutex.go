package proxy

import "sync"

// keyedMutex provides fine-grained mutual exclusion per string key.
// It tracks reference counts so that entries are automatically removed from
// the internal map when idle, preventing memory leaks.
type keyedMutex struct {
	mu    sync.Mutex
	locks map[string]*keyLockEntry
}

type keyLockEntry struct {
	mu       sync.Mutex
	refCount int
}

// newKeyedMutex creates a new keyedMutex.
func newKeyedMutex() *keyedMutex {
	return &keyedMutex{
		locks: make(map[string]*keyLockEntry),
	}
}

// Lock acquires the mutex associated with key and returns an unlock function.
// The caller must invoke the returned function when finished with the critical section.
// The unlock function is idempotent and safe to call multiple times.
func (km *keyedMutex) Lock(key string) func() {
	km.mu.Lock()
	entry, exists := km.locks[key]
	if !exists {
		entry = &keyLockEntry{}
		km.locks[key] = entry
	}
	entry.refCount++
	km.mu.Unlock()

	entry.mu.Lock()

	var once sync.Once
	return func() {
		once.Do(func() {
			entry.mu.Unlock()

			km.mu.Lock()
			entry.refCount--
			if entry.refCount <= 0 {
				delete(km.locks, key)
			}
			km.mu.Unlock()
		})
	}
}

// keyCount returns the number of currently tracked keys (for testing and observability).
func (km *keyedMutex) keyCount() int {
	km.mu.Lock()
	defer km.mu.Unlock()
	return len(km.locks)
}
