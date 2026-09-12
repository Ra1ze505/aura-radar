package aura

import "sync"

// InFlight tracks one in-progress judge call per chat (TZ §14).
type InFlight struct {
	mu    sync.Mutex
	chats map[int64]struct{}
}

func NewInFlight() *InFlight {
	return &InFlight{chats: make(map[int64]struct{})}
}

func (f *InFlight) TryLock(chatID int64) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.chats[chatID]; ok {
		return false
	}
	f.chats[chatID] = struct{}{}
	return true
}

func (f *InFlight) Unlock(chatID int64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.chats, chatID)
}

func (f *InFlight) Busy(chatID int64) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, ok := f.chats[chatID]
	return ok
}
