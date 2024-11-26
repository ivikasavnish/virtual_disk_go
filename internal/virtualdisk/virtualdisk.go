package virtualdisk

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/vikasavn/virtual_disk_go/internal/cache"
	"github.com/vikasavn/virtual_disk_go/internal/events"
	"github.com/vikasavn/virtual_disk_go/internal/mmap"
	"github.com/vikasavn/virtual_disk_go/internal/s3store"
)

// StorageType represents the type of storage for files and directories
type StorageType string

const (
	StoragePersistent StorageType = "persistent" // Stored on disk
	StorageTemp       StorageType = "temp"       // Stored in temp directory, deleted on close
	StorageMemory     StorageType = "memory"     // Stored in memory only
)

// Config represents the configuration for VirtualDisk
type Config struct {
	DataPartition string
	BufferSize    int64
	UseS3         bool
	S3Config      *S3Config
	EnableTemp    bool
	EnableMemory  bool
	CacheSize     int64
	TempTTL       time.Duration
}

// VirtualDisk represents the virtual disk system
type VirtualDisk struct {
	dataPartition string
	tempDir       string
	bufferSize    int64
	buffer        map[string]*BufferEntry
	mu            sync.RWMutex
	s3store       *s3store.S3Store
	mmapFiles     map[string]*mmap.MappedFile
	enableTemp    bool
	enableMemory  bool
	eventBus      *events.EventBus
	cache         *cache.Cache
	tempTTL       time.Duration
}

// BufferEntry represents a file in the memory buffer
type BufferEntry struct {
	Data     []byte
	Modified time.Time
	Type     StorageType
}

// NewVirtualDisk creates a new virtual disk instance
func NewVirtualDisk(config Config) (*VirtualDisk, error) {
	if err := os.MkdirAll(config.DataPartition, 0755); err != nil {
		return nil, fmt.Errorf("failed to create data partition: %w", err)
	}

	vd := &VirtualDisk{
		dataPartition: config.DataPartition,
		bufferSize:    config.BufferSize,
		buffer:        make(map[string]*BufferEntry),
		mmapFiles:     make(map[string]*mmap.MappedFile),
		enableTemp:    config.EnableTemp,
		enableMemory:  config.EnableMemory,
		eventBus:      events.NewEventBus(),
		tempTTL:       config.TempTTL,
	}

	// Initialize cache
	if config.CacheSize > 0 {
		vd.cache = cache.NewCache(config.CacheSize, func(key string, value []byte) {
			// When items are evicted from cache, write them to disk if needed
			if strings.HasPrefix(key, "temp/") || strings.HasPrefix(key, "mem/") {
				return // Don't persist temporary or memory-only files
			}
			fullPath := filepath.Join(vd.dataPartition, key)
			if err := ioutil.WriteFile(fullPath, value, 0644); err != nil {
				// Log error but don't fail
				fmt.Printf("failed to write cached file to disk: %v\n", err)
			}
		})
	}

	// Create temporary directory if enabled
	if config.EnableTemp {
		tempDir, err := ioutil.TempDir("", "virtualdisk_temp_")
		if err != nil {
			return nil, fmt.Errorf("failed to create temp directory: %w", err)
		}
		vd.tempDir = tempDir

		// Start temp file cleanup goroutine
		if config.TempTTL > 0 {
			go vd.cleanupTempFiles()
		}
	}

	if config.UseS3 && config.S3Config != nil {
		s3store, err := s3store.NewS3Store(
			config.S3Config.Endpoint,
			config.S3Config.Region,
			config.S3Config.BucketName,
			config.S3Config.Prefix,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize S3 store: %w", err)
		}
		vd.s3store = s3store
	}

	return vd, nil
}

// cleanupTempFiles periodically removes expired temporary files
func (vd *VirtualDisk) cleanupTempFiles() {
	ticker := time.NewTicker(vd.tempTTL / 2)
	defer ticker.Stop()

	for range ticker.C {
		vd.mu.Lock()
		now := time.Now()
		for path, entry := range vd.buffer {
			if entry.Type == StorageTemp && now.Sub(entry.Modified) > vd.tempTTL {
				delete(vd.buffer, path)
				// Also remove from disk
				fullPath := vd.getFilePath(path, StorageTemp)
				os.Remove(fullPath) // Ignore errors
			}
		}
		vd.mu.Unlock()
	}
}

// WriteFile writes data to a file in the virtual disk
func (vd *VirtualDisk) WriteFile(path string, data []byte) error {
	vd.mu.Lock()
	defer vd.mu.Unlock()

	storageType := vd.getStorageType(path)

	// Prepare event metadata
	metadata := map[string]interface{}{
		"data": data,
		"size": len(data),
	}

	// For memory storage, just store in buffer
	if storageType == StorageMemory {
		vd.buffer[path] = &BufferEntry{
			Data:     data,
			Modified: time.Now(),
			Type:     StorageMemory,
		}
		// Cache the data
		if vd.cache != nil {
			vd.cache.Put(path, data, int64(len(data)))
		}
		// Publish event
		vd.eventBus.Publish(events.Event{
			Type:      events.EventFileCreated,
			Path:      path,
			Timestamp: time.Now(),
			Metadata:  metadata,
		})
		return nil
	}

	// Get the actual file path
	fullPath := vd.getFilePath(path, storageType)
	dir := filepath.Dir(fullPath)

	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Write file
	if err := ioutil.WriteFile(fullPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	// Cache the data
	if vd.cache != nil {
		vd.cache.Put(path, data, int64(len(data)))
	}

	// Write to S3 if configured and not temporary
	if vd.s3store != nil && storageType == StoragePersistent {
		if err := vd.s3store.WriteFile(path, data); err != nil {
			return fmt.Errorf("failed to write to S3: %w", err)
		}
	}

	// Publish event
	vd.eventBus.Publish(events.Event{
		Type:      events.EventFileCreated,
		Path:      path,
		Timestamp: time.Now(),
		Metadata:  metadata,
	})

	return nil
}

// ReadFile reads data from a file in the virtual disk
func (vd *VirtualDisk) ReadFile(path string) ([]byte, error) {
	vd.mu.RLock()
	defer vd.mu.RUnlock()

	// Try cache first
	if vd.cache != nil {
		if data, ok := vd.cache.Get(path); ok {
			// Publish access event
			vd.eventBus.Publish(events.Event{
				Type:      events.EventFileAccessed,
				Path:      path,
				Timestamp: time.Now(),
			})
			return data, nil
		}
	}

	// Check memory buffer
	if entry, ok := vd.buffer[path]; ok {
		// Cache the data for future use
		if vd.cache != nil {
			vd.cache.Put(path, entry.Data, int64(len(entry.Data)))
		}
		// Publish access event
		vd.eventBus.Publish(events.Event{
			Type:      events.EventFileAccessed,
			Path:      path,
			Timestamp: time.Now(),
		})
		return entry.Data, nil
	}

	storageType := vd.getStorageType(path)
	fullPath := vd.getFilePath(path, storageType)

	data, err := ioutil.ReadFile(fullPath)
	if err != nil {
		// Try S3 if configured and not temporary
		if vd.s3store != nil && storageType == StoragePersistent {
			data, err = vd.s3store.ReadFile(path)
			if err != nil {
				return nil, fmt.Errorf("failed to read file: %w", err)
			}
		} else {
			return nil, fmt.Errorf("failed to read file: %w", err)
		}
	}

	// Cache the data for future use
	if vd.cache != nil {
		vd.cache.Put(path, data, int64(len(data)))
	}

	// Publish access event
	vd.eventBus.Publish(events.Event{
		Type:      events.EventFileAccessed,
		Path:      path,
		Timestamp: time.Now(),
	})

	return data, nil
}

// Subscribe registers an event handler
func (vd *VirtualDisk) Subscribe(eventType events.EventType, handler events.Handler) {
	vd.eventBus.Subscribe(eventType, handler)
}

// getStorageType determines the storage type based on the path prefix
func (vd *VirtualDisk) getStorageType(path string) StorageType {
	if strings.HasPrefix(path, "temp/") && vd.enableTemp {
		return StorageTemp
	}
	if strings.HasPrefix(path, "mem/") && vd.enableMemory {
		return StorageMemory
	}
	return StoragePersistent
}

// getFilePath returns the actual file path based on storage type
func (vd *VirtualDisk) getFilePath(path string, storageType StorageType) string {
	switch storageType {
	case StorageTemp:
		return filepath.Join(vd.tempDir, strings.TrimPrefix(path, "temp/"))
	case StorageMemory:
		return path
	default:
		return filepath.Join(vd.dataPartition, path)
	}
}

// DeleteFile deletes a file from the virtual disk
func (vd *VirtualDisk) DeleteFile(path string) error {
	vd.mu.Lock()
	defer vd.mu.Unlock()

	storageType := vd.getStorageType(path)
	fullPath := vd.getFilePath(path, storageType)

	// Remove from buffer if present
	if _, ok := vd.buffer[path]; ok {
		delete(vd.buffer, path)
	}

	// Remove from disk
	if err := os.Remove(fullPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete file: %w", err)
	}

	// Delete from S3 if configured
	if vd.s3store != nil {
		if err := vd.s3store.DeleteFile(path); err != nil {
			return fmt.Errorf("failed to delete from S3: %w", err)
		}
	}

	return nil
}

// ListFiles lists all files in the virtual disk with an optional prefix
func (vd *VirtualDisk) ListFiles(prefix string) ([]string, error) {
	vd.mu.RLock()
	defer vd.mu.RUnlock()

	files := make(map[string]struct{})

	// Add files from buffer
	for path := range vd.buffer {
		if strings.HasPrefix(path, prefix) {
			files[path] = struct{}{}
		}
	}

	// Add files from disk
	err := filepath.Walk(vd.dataPartition, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			relPath, err := filepath.Rel(vd.dataPartition, path)
			if err != nil {
				return err
			}
			if strings.HasPrefix(relPath, prefix) {
				files[relPath] = struct{}{}
			}
		}
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to list files: %w", err)
	}

	// Add files from S3 if configured
	if vd.s3store != nil {
		s3Files, err := vd.s3store.ListFiles(prefix)
		if err != nil {
			return nil, fmt.Errorf("failed to list S3 files: %w", err)
		}
		for _, path := range s3Files {
			files[path] = struct{}{}
		}
	}

	// Convert map to slice
	result := make([]string, 0, len(files))
	for path := range files {
		result = append(result, path)
	}

	return result, nil
}

// ListFilesAndDirs lists all files and directories in the virtual disk with an optional prefix
func (vd *VirtualDisk) ListFilesAndDirs(prefix string) ([]FileInfo, error) {
	vd.mu.RLock()
	defer vd.mu.RUnlock()

	items := make(map[string]FileInfo)

	// Add files from disk and directories
	err := filepath.Walk(vd.dataPartition, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(vd.dataPartition, path)
		if err != nil {
			return err
		}

		// Skip the root directory
		if relPath == "." {
			return nil
		}

		if strings.HasPrefix(relPath, prefix) {
			items[relPath] = FileInfo{
				Path:     relPath,
				IsDir:    info.IsDir(),
				Size:     info.Size(),
				Modified: info.ModTime().Format(time.RFC3339),
			}
		}
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to list items: %w", err)
	}

	// Add files from buffer
	for path, entry := range vd.buffer {
		if strings.HasPrefix(path, prefix) {
			items[path] = FileInfo{
				Path:     path,
				IsDir:    false,
				Size:     int64(len(entry.Data)),
				Modified: entry.Modified.Format(time.RFC3339),
			}
		}
	}

	// Convert map to slice
	result := make([]FileInfo, 0, len(items))
	for _, info := range items {
		result = append(result, info)
	}

	return result, nil
}

// FileInfo represents information about a file or directory
type FileInfo struct {
	Path      string `json:"path"`
	IsDir     bool   `json:"is_dir"`
	Size      int64  `json:"size,omitempty"`
	Modified  string `json:"modified"`
}

// Flush writes all buffered data to disk and S3
func (vd *VirtualDisk) Flush() error {
	for path := range vd.buffer {
		delete(vd.buffer, path)
	}
	return nil
}

// CreateDirectory creates a directory and all parent directories in the virtual disk
func (vd *VirtualDisk) CreateDirectory(path string) error {
	vd.mu.Lock()
	defer vd.mu.Unlock()

	// Clean and validate the path
	path = filepath.Clean(path)
	if path == "." || path == "/" {
		return nil // root directory always exists
	}

	storageType := vd.getStorageType(path)
	fullPath := vd.getFilePath(path, storageType)

	// Create the full path in the data partition
	if err := os.MkdirAll(fullPath, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	return nil
}

// Close flushes all data, closes memory mapped files, and closes the virtual disk
func (vd *VirtualDisk) Close() error {
	vd.mu.Lock()
	defer vd.mu.Unlock()

	// Clear memory buffer
	vd.buffer = make(map[string]*BufferEntry)

	// Close memory mapped files
	for _, file := range vd.mmapFiles {
		if err := file.Close(); err != nil {
			return fmt.Errorf("failed to close memory mapped file: %w", err)
		}
	}
	vd.mmapFiles = make(map[string]*mmap.MappedFile)

	// Remove temporary directory if it exists
	if vd.tempDir != "" {
		if err := os.RemoveAll(vd.tempDir); err != nil {
			return fmt.Errorf("failed to remove temp directory: %w", err)
		}
	}

	return vd.Flush()
}

// S3Config represents the S3 configuration
type S3Config struct {
	Endpoint   string
	Region     string
	BucketName string
	Prefix     string
}
