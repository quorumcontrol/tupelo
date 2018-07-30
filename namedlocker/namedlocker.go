package namedlocker

import (
	"sync"
)

// NamedLocker implements locks by name with lazy object creation.
// Mutexes are implemented as RWMutex
type NamedLocker struct {
	locks *sync.Map
}

// NewNamedLocker creates a new named locker
func NewNamedLocker() *NamedLocker {
	return &NamedLocker{
		locks: new(sync.Map),
	}
}

// Lock locks the named lock for write access
func (nl *NamedLocker) Lock(name string) {
	mut, _ := nl.locks.LoadOrStore(name, new(sync.RWMutex))
	mut.(*sync.RWMutex).Lock()
}

// Unlock unlocks the named lock for write access, panics on non existence
func (nl *NamedLocker) Unlock(name string) {
	mut, _ := nl.locks.Load(name)
	mut.(*sync.RWMutex).Unlock()
}

// RLock locks the named lock for read access
func (nl *NamedLocker) RLock(name string) {
	mut, _ := nl.locks.LoadOrStore(name, new(sync.RWMutex))
	mut.(*sync.RWMutex).RLock()
}

// RUnlock unlocks the named lock for read access, panics on non existence
func (nl *NamedLocker) RUnlock(name string) {
	mut, _ := nl.locks.Load(name)
	mut.(*sync.RWMutex).RUnlock()
}

// Delete deletes the named lock, panics on non existence
func (nl *NamedLocker) Delete(name string) {
	mut, _ := nl.locks.Load(name)
	mut.(*sync.RWMutex).Lock()
	defer mut.(*sync.RWMutex).Unlock()
	nl.locks.Delete(name)
}
