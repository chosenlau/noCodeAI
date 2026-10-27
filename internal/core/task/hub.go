package task

import "sync"

type eventHub struct {
	mu          sync.Mutex
	nextID      uint64
	subscribers map[int64]map[uint64]chan struct{}
}

func newEventHub() *eventHub {
	return &eventHub{subscribers: make(map[int64]map[uint64]chan struct{})}
}

func (h *eventHub) subscribe(runID int64) (<-chan struct{}, func()) {
	h.mu.Lock()
	h.nextID++
	id := h.nextID
	ch := make(chan struct{}, 1)
	if h.subscribers[runID] == nil {
		h.subscribers[runID] = make(map[uint64]chan struct{})
	}
	h.subscribers[runID][id] = ch
	h.mu.Unlock()

	return ch, func() {
		h.mu.Lock()
		if subscribers := h.subscribers[runID]; subscribers != nil {
			delete(subscribers, id)
			if len(subscribers) == 0 {
				delete(h.subscribers, runID)
			}
		}
		h.mu.Unlock()
	}
}

func (h *eventHub) notify(runID int64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, ch := range h.subscribers[runID] {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}
