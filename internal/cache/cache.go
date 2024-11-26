package cache

import (
	"container/list"
	"sync"
	"time"
)

// Entry represents a cache entry
type Entry struct {
	Key       string
	Value     []byte
	Size      int64
	Accessed  time.Time
	Modified  time.Time
	Reference int
}

// Cache implements an LRU cache with reference counting
type Cache struct {
	capacity    int64
	size        int64
	items       map[string]*list.Element
	lru         *list.List
	mu          sync.RWMutex
	evictNotify func(key string, value []byte)
}

// NewCache creates a new cache with the given capacity in bytes
func NewCache(capacity int64, evictNotify func(key string, value []byte)) *Cache {
	return &Cache{
		capacity:    capacity,
		items:       make(map[string]*list.Element),
		lru:         list.New(),
		evictNotify: evictNotify,
	}
}

// Get retrieves an item from the cache
func (c *Cache) Get(key string) ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if element, exists := c.items[key]; exists {
		entry := element.Value.(*Entry)
		entry.Accessed = time.Now()
		entry.Reference++
		c.lru.MoveToFront(element)
		return entry.Value, true
	}
	return nil, false
}

// Put adds an item to the cache
func (c *Cache) Put(key string, value []byte, size int64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// If entry exists, update it
	if element, exists := c.items[key]; exists {
		entry := element.Value.(*Entry)
		c.size -= entry.Size
		entry.Value = value
		entry.Size = size
		entry.Modified = time.Now()
		entry.Accessed = time.Now()
		entry.Reference++
		c.size += size
		c.lru.MoveToFront(element)
	} else {
		// Create new entry
		entry := &Entry{
			Key:       key,
			Value:     value,
			Size:      size,
			Modified:  time.Now(),
			Accessed:  time.Now(),
			Reference: 1,
		}
		element := c.lru.PushFront(entry)
		c.items[key] = element
		c.size += size
	}

	// Evict items if cache is full
	for c.size > c.capacity {
		c.evictOldest()
	}
}

// Release decrements the reference count of an item
func (c *Cache) Release(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if element, exists := c.items[key]; exists {
		entry := element.Value.(*Entry)
		entry.Reference--
		if entry.Reference <= 0 {
			c.lru.MoveToBack(element)
		}
	}
}

// evictOldest removes the least recently used item from the cache
func (c *Cache) evictOldest() {
	element := c.lru.Back()
	if element == nil {
		return
	}

	entry := element.Value.(*Entry)
	if entry.Reference > 0 {
		// Skip items that are still referenced
		return
	}

	c.lru.Remove(element)
	delete(c.items, entry.Key)
	c.size -= entry.Size

	if c.evictNotify != nil {
		c.evictNotify(entry.Key, entry.Value)
	}
}

// Clear removes all items from the cache
func (c *Cache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	for key, element := range c.items {
		entry := element.Value.(*Entry)
		if c.evictNotify != nil {
			c.evictNotify(entry.Key, entry.Value)
		}
		delete(c.items, key)
	}
	c.lru.Init()
	c.size = 0
}
