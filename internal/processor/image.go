package processor

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"path/filepath"
	"strings"

	"github.com/nfnt/resize"
	"github.com/vikasavn/virtual_disk_go/internal/events"
)

// ImageProcessor handles image processing operations
type ImageProcessor struct {
	thumbnailSizes []uint
}

// NewImageProcessor creates a new image processor
func NewImageProcessor(thumbnailSizes []uint) *ImageProcessor {
	return &ImageProcessor{
		thumbnailSizes: thumbnailSizes,
	}
}

// ProcessImage handles image processing events
func (ip *ImageProcessor) ProcessImage(event events.Event) error {
	if event.Type != events.EventFileCreated && event.Type != events.EventFileModified {
		return nil
	}

	// Check if file is an image
	ext := strings.ToLower(filepath.Ext(event.Path))
	if ext != ".jpg" && ext != ".jpeg" && ext != ".png" {
		return nil
	}

	// Get image data from metadata
	data, ok := event.Metadata["data"].([]byte)
	if !ok {
		return fmt.Errorf("no image data in metadata")
	}

	// Decode image
	img, format, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("failed to decode image: %w", err)
	}

	// Generate thumbnails
	thumbnails := make(map[string][]byte)
	for _, size := range ip.thumbnailSizes {
		thumb := resize.Thumbnail(size, size, img, resize.Lanczos3)
		var buf bytes.Buffer

		switch format {
		case "jpeg":
			if err := jpeg.Encode(&buf, thumb, nil); err != nil {
				return fmt.Errorf("failed to encode JPEG thumbnail: %w", err)
			}
		case "png":
			if err := png.Encode(&buf, thumb); err != nil {
				return fmt.Errorf("failed to encode PNG thumbnail: %w", err)
			}
		}

		thumbPath := fmt.Sprintf("%s_%dx%d%s", 
			strings.TrimSuffix(event.Path, ext),
			size, size,
			ext,
		)
		thumbnails[thumbPath] = buf.Bytes()
	}

	// Add thumbnails to metadata for storage
	event.Metadata["thumbnails"] = thumbnails
	return nil
}

// GetThumbnailPath returns the path for a thumbnail of the given size
func GetThumbnailPath(originalPath string, size uint) string {
	ext := filepath.Ext(originalPath)
	return fmt.Sprintf("%s_%dx%d%s",
		strings.TrimSuffix(originalPath, ext),
		size, size,
		ext,
	)
}
