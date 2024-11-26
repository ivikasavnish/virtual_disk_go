package mmap

import (
	"fmt"
	"os"
	"sync"
	"syscall"
	"golang.org/x/sys/unix"
)

// MappedFile represents a memory-mapped file
type MappedFile struct {
	file     *os.File
	data     []byte
	size     int64
	mu       sync.RWMutex
	isClosed bool
}

// OpenFile opens or creates a memory-mapped file
func OpenFile(path string, size int64) (*MappedFile, error) {
	file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}

	// Get current file size
	fi, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, fmt.Errorf("failed to stat file: %w", err)
	}

	// Extend file if necessary
	if fi.Size() < size {
		if err := file.Truncate(size); err != nil {
			file.Close()
			return nil, fmt.Errorf("failed to truncate file: %w", err)
		}
	}

	// Memory map the file
	data, err := syscall.Mmap(int(file.Fd()), 0, int(size),
		syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_SHARED)
	if err != nil {
		file.Close()
		return nil, fmt.Errorf("failed to mmap: %w", err)
	}

	return &MappedFile{
		file: file,
		data: data,
		size: size,
	}, nil
}

// Write writes data to the memory-mapped file at the specified offset
func (mf *MappedFile) Write(offset int64, data []byte) error {
	mf.mu.Lock()
	defer mf.mu.Unlock()

	if mf.isClosed {
		return fmt.Errorf("file is closed")
	}

	if offset+int64(len(data)) > mf.size {
		return fmt.Errorf("write would exceed file size")
	}

	copy(mf.data[offset:], data)
	return unix.Msync(mf.data, unix.MS_SYNC)
}

// Read reads data from the memory-mapped file at the specified offset
func (mf *MappedFile) Read(offset, length int64) ([]byte, error) {
	mf.mu.RLock()
	defer mf.mu.RUnlock()

	if mf.isClosed {
		return nil, fmt.Errorf("file is closed")
	}

	if offset+length > mf.size {
		return nil, fmt.Errorf("read would exceed file size")
	}

	data := make([]byte, length)
	copy(data, mf.data[offset:offset+length])
	return data, nil
}

// Close closes the memory-mapped file
func (mf *MappedFile) Close() error {
	mf.mu.Lock()
	defer mf.mu.Unlock()

	if mf.isClosed {
		return nil
	}

	if err := syscall.Munmap(mf.data); err != nil {
		return fmt.Errorf("failed to unmap: %w", err)
	}

	if err := mf.file.Close(); err != nil {
		return fmt.Errorf("failed to close file: %w", err)
	}

	mf.isClosed = true
	return nil
}

// Sync synchronizes the memory-mapped file with storage
func (mf *MappedFile) Sync() error {
	mf.mu.RLock()
	defer mf.mu.RUnlock()

	if mf.isClosed {
		return fmt.Errorf("file is closed")
	}

	return unix.Msync(mf.data, unix.MS_SYNC)
}

// Size returns the size of the memory-mapped file
func (mf *MappedFile) Size() int64 {
	return mf.size
}
