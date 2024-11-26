package main

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"
)

type FileInfo struct {
	Path string `json:"path"`
	Size int64  `json:"size"`
}

type StatsInfo struct {
	TotalSize int64 `json:"totalSize"`
	UsedSpace int64 `json:"usedSpace"`
	FreeSpace int64 `json:"freeSpace"`
	FileCount int   `json:"fileCount"`
}

type Response struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data"`
	Error   string      `json:"error,omitempty"`
}

func main() {
	// Set up data directory
	dataDir := os.Getenv("DATA_PARTITION")
	if dataDir == "" {
		dataDir = filepath.Join(".", "data")
	}
	log.Infof("Using data directory: %s", dataDir)

	// Create storage directories if they don't exist
	for _, dir := range []string{"disk", "temp", "memory", "mmap"} {
		if err := os.MkdirAll(filepath.Join(dataDir, dir), 0755); err != nil {
			log.Fatalf("Failed to create directory: %v", err)
		}
	}

	// Set up router
	router := gin.Default()

	// Configure CORS
	config := cors.DefaultConfig()
	config.AllowOrigins = []string{"http://localhost:3000"}
	config.AllowMethods = []string{"GET", "POST", "DELETE", "OPTIONS"}
	router.Use(cors.New(config))

	// API endpoints
	api := router.Group("/api")
	{
		api.GET("/stats", func(c *gin.Context) {
			storageType := c.Query("type")
			if storageType == "" {
				storageType = "disk"
			}

			storageDir := filepath.Join(dataDir, storageType)
			var stats StatsInfo

			// Get storage statistics
			var totalSize, usedSpace int64
			var fileCount int
			err := filepath.Walk(storageDir, func(path string, info os.FileInfo, err error) error {
				if err != nil {
					return err
				}
				if !info.IsDir() {
					totalSize += info.Size()
					fileCount++
				}
				return nil
			})

			if err != nil {
				c.JSON(http.StatusInternalServerError, Response{
					Success: false,
					Error:   err.Error(),
				})
				return
			}

			usedSpace = totalSize
			freeSpace := int64(1024 * 1024 * 1024) // 1GB limit for demonstration

			stats = StatsInfo{
				TotalSize: freeSpace,
				UsedSpace: usedSpace,
				FreeSpace: freeSpace - usedSpace,
				FileCount: fileCount,
			}

			c.JSON(http.StatusOK, Response{
				Success: true,
				Data:    stats,
			})
		})

		api.GET("/list", func(c *gin.Context) {
			storageType := c.Query("type")
			if storageType == "" {
				storageType = "disk"
			}

			storageDir := filepath.Join(dataDir, storageType)
			var files []FileInfo

			err := filepath.Walk(storageDir, func(path string, info os.FileInfo, err error) error {
				if err != nil {
					return err
				}
				if !info.IsDir() {
					relPath, err := filepath.Rel(storageDir, path)
					if err != nil {
						return err
					}
					files = append(files, FileInfo{
						Path: relPath,
						Size: info.Size(),
					})
				}
				return nil
			})

			if err != nil {
				c.JSON(http.StatusInternalServerError, Response{
					Success: false,
					Error:   err.Error(),
				})
				return
			}

			c.JSON(http.StatusOK, Response{
				Success: true,
				Data:    files,
			})
		})

		api.POST("/files", func(c *gin.Context) {
			storageType := c.Query("type")
			if storageType == "" {
				storageType = "disk"
			}

			filePath := c.Query("path")
			if filePath == "" {
				c.JSON(http.StatusBadRequest, Response{
					Success: false,
					Error:   "path is required",
				})
				return
			}

			file, err := c.FormFile("file")
			if err != nil {
				c.JSON(http.StatusBadRequest, Response{
					Success: false,
					Error:   err.Error(),
				})
				return
			}

			targetPath := filepath.Join(dataDir, storageType, filePath)
			if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
				c.JSON(http.StatusInternalServerError, Response{
					Success: false,
					Error:   err.Error(),
				})
				return
			}

			if err := c.SaveUploadedFile(file, targetPath); err != nil {
				c.JSON(http.StatusInternalServerError, Response{
					Success: false,
					Error:   err.Error(),
				})
				return
			}

			c.JSON(http.StatusOK, Response{
				Success: true,
			})
		})

		api.DELETE("/files", func(c *gin.Context) {
			storageType := c.Query("type")
			if storageType == "" {
				storageType = "disk"
			}

			filePath := c.Query("path")
			if filePath == "" {
				c.JSON(http.StatusBadRequest, Response{
					Success: false,
					Error:   "path is required",
				})
				return
			}

			targetPath := filepath.Join(dataDir, storageType, filePath)
			if err := os.Remove(targetPath); err != nil {
				c.JSON(http.StatusInternalServerError, Response{
					Success: false,
					Error:   err.Error(),
				})
				return
			}

			c.JSON(http.StatusOK, Response{
				Success: true,
			})
		})

		api.GET("/explore", func(c *gin.Context) {
			path := c.Query("path")
			if path == "" {
				path = "/"
			}

			// Check if path exists and is a directory
			info, err := os.Stat(path)
			if err != nil || !info.IsDir() {
				c.JSON(http.StatusBadRequest, gin.H{
					"success": false,
					"error":   "Invalid directory path",
				})
				return
			}

			// Read directory contents
			entries, err := os.ReadDir(path)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{
					"success": false,
					"error":   "Failed to read directory",
				})
				return
			}

			// Convert entries to response format
			items := make([]gin.H, 0)
			for _, entry := range entries {
				info, err := entry.Info()
				if err != nil {
					continue
				}

				items = append(items, gin.H{
					"name":      entry.Name(),
					"path":      filepath.Join(path, entry.Name()),
					"isDir":     entry.IsDir(),
					"size":      info.Size(),
					"modified":  info.ModTime(),
					"isHidden":  strings.HasPrefix(entry.Name(), "."),
				})
			}

			c.JSON(http.StatusOK, gin.H{
				"success": true,
				"data": gin.H{
					"path":     path,
					"items":    items,
					"parent":   filepath.Dir(path),
				},
			})
		})

		api.POST("/mount", func(c *gin.Context) {
			path := c.Query("path")
			if path == "" {
				c.JSON(http.StatusBadRequest, gin.H{
					"success": false,
					"error":   "Path is required",
				})
				return
			}

			// Verify directory exists
			info, err := os.Stat(path)
			if err != nil || !info.IsDir() {
				c.JSON(http.StatusBadRequest, gin.H{
					"success": false,
					"error":   "Invalid directory path",
				})
				return
			}

			// Create virtual disk for this path if it doesn't exist
			// Note: This is a placeholder - implement actual mounting logic based on your needs
			c.JSON(http.StatusOK, gin.H{
				"success": true,
				"message": "Directory " + path + " mounted successfully",
			})
		})
	}

	// Serve static files
	router.Static("/static", "./frontend/build/static")
	router.StaticFile("/bundle.js", "./frontend/build/bundle.js")
	router.StaticFile("/", "./frontend/build/index.html")
	router.NoRoute(func(c *gin.Context) {
		c.File("./frontend/build/index.html")
	})

	// Start server
	port := os.Getenv("PORT")
	if port == "" {
		port = "3002"
	}

	log.Infof("Starting server on port %s", port)
	if err := router.Run(":" + port); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
