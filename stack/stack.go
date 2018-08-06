package stack

import (
	"sync"
)

// Stack is a Last-In-First-Out (LIFO) queue
type Stack struct {
	top  *element
	size int
	lock *sync.RWMutex
}

// NewStack returns a new LIFO stack
func NewStack() *Stack {
	return &Stack{
		lock: new(sync.RWMutex),
	}
}

type element struct {
	value interface{} // All types satisfy the empty interface, so we can store anything here.
	next  *element
}

// Len returns the stack's length
func (s *Stack) Len() int {
	s.lock.RLock()
	defer s.lock.RUnlock()
	return s.size
}

// Push a new element onto the stack
func (s *Stack) Push(value interface{}) {
	s.lock.Lock()
	defer s.lock.Unlock()
	s.top = &element{value, s.top}
	s.size++
}

// Pop Removes the top element from the stack and return its value
// If the stack is empty, return nil
func (s *Stack) Pop() (value interface{}) {
	s.lock.Lock()
	defer s.lock.Unlock()
	if s.size > 0 {
		value, s.top = s.top.value, s.top.next
		s.size--
		return
	}
	return nil
}

// Clear removes all entries from the stack
func (s *Stack) Clear() {
	s.lock.Lock()
	defer s.lock.Unlock()
	s.top = nil
	s.size = 0
}
