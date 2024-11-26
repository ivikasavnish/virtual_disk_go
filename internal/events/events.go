package events

import (
	"sync"
	"time"
)

// EventType represents the type of file system event
type EventType string

const (
	EventFileCreated  EventType = "file_created"
	EventFileModified EventType = "file_modified"
	EventFileDeleted  EventType = "file_deleted"
	EventFileAccessed EventType = "file_accessed"
)

// Event represents a file system event
type Event struct {
	Type      EventType
	Path      string
	Timestamp time.Time
	Metadata  map[string]interface{}
}

// Handler is a function that processes events
type Handler func(event Event) error

// EventBus manages event subscriptions and publishing
type EventBus struct {
	subscribers map[EventType][]Handler
	mu          sync.RWMutex
}

// NewEventBus creates a new event bus
func NewEventBus() *EventBus {
	return &EventBus{
		subscribers: make(map[EventType][]Handler),
	}
}

// Subscribe registers a handler for a specific event type
func (eb *EventBus) Subscribe(eventType EventType, handler Handler) {
	eb.mu.Lock()
	defer eb.mu.Unlock()

	if eb.subscribers[eventType] == nil {
		eb.subscribers[eventType] = make([]Handler, 0)
	}
	eb.subscribers[eventType] = append(eb.subscribers[eventType], handler)
}

// Publish sends an event to all subscribers
func (eb *EventBus) Publish(event Event) []error {
	eb.mu.RLock()
	defer eb.mu.RUnlock()

	var errors []error
	for _, handler := range eb.subscribers[event.Type] {
		if err := handler(event); err != nil {
			errors = append(errors, err)
		}
	}
	return errors
}
