package utils

import (
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/cloudinary/cloudinary-go/v2"
	"github.com/cloudinary/cloudinary-go/v2/api/uploader"
	"go.uber.org/zap"
	"golang.org/x/exp/slices"
)

// Config holds configuration settings for CloudinaryService.
type Config struct {
	MaxFileSize     int64               // Maximum allowed file size in bytes
	UploadTimeout   time.Duration       // Timeout for upload operations
	DeleteTimeout   time.Duration       // Timeout for delete operations
	MaxRetries      int                 // Maximum retry attempts for uploads
	AllowedTypes    map[string]bool     // Allowed MIME types
	ValidExtensions map[string][]string // Valid extensions per MIME type
}

// DefaultConfig provides default configuration values.
func DefaultConfig() Config {
	return Config{
		MaxFileSize:   10 * 1024 * 1024, // 10MB
		UploadTimeout: 30 * time.Second,
		DeleteTimeout: 10 * time.Second,
		MaxRetries:    3,
		AllowedTypes: map[string]bool{
			"image/jpeg":         true,
			"image/png":          true,
			"image/gif":          true,
			"image/webp":         true,
			"image/svg+xml":      true,
			"application/pdf":    true,
			"application/msword": true,
			"application/vnd.openxmlformats-officedocument.wordprocessingml.document": true,
			"application/vnd.ms-excel": true,
			"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet":         true,
			"application/vnd.ms-powerpoint":                                             true,
			"application/vnd.openxmlformats-officedocument.presentationml.presentation": true,
			"text/plain":       true,
			"text/csv":         true,
			"application/json": true,
			"application/zip":  true,
			"video/mp4":        true,
			"video/webm":       true,
			"audio/mpeg":       true,
			"audio/wav":        true,
			"audio/webm":       true,
		},
		ValidExtensions: map[string][]string{
			"image/jpeg":         {".jpg", ".jpeg"},
			"image/png":          {".png"},
			"image/gif":          {".gif"},
			"image/webp":         {".webp"},
			"image/svg+xml":      {".svg"},
			"application/pdf":    {".pdf"},
			"application/msword": {".doc"},
			"application/vnd.openxmlformats-officedocument.wordprocessingml.document": {".docx"},
			"application/vnd.ms-excel": {".xls"},
			"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet":         {".xlsx"},
			"application/vnd.ms-powerpoint":                                             {".ppt"},
			"application/vnd.openxmlformats-officedocument.presentationml.presentation": {".pptx"},
			"text/plain":       {".txt"},
			"text/csv":         {".csv"},
			"application/json": {".json"},
			"application/zip":  {".zip"},
			"video/mp4":        {".mp4"},
			"video/webm":       {".webm"},
			"audio/mpeg":       {".mp3"},
			"audio/wav":        {".wav"},
			"audio/webm":       {".weba"},
		},
	}
}

// FileStorage defines the interface for file storage operations.
type FileStorage interface {
	UploadFile(ctx context.Context, file *multipart.FileHeader, folder string) (*UploadResult, error)
	DeleteFile(ctx context.Context, id string) error
	ValidateFile(ctx context.Context, file *multipart.FileHeader) error
}

// CloudinaryService wraps the Cloudinary client and configuration.
type CloudinaryService struct {
	Client *cloudinary.Cloudinary
	Config Config
	Logger *zap.Logger
}

// UploadResult contains the result of a file upload.
type UploadResult struct {
	URL      string
	PublicID string
	Format   string
	Size     int
}

// Custom errors for specific failure cases.
var (
	ErrFileTooLarge       = fmt.Errorf("file size exceeds limit")
	ErrInvalidContentType = fmt.Errorf("invalid content type")
	ErrInvalidExtension   = fmt.Errorf("invalid file extension")
	ErrUnableToOpenFile   = fmt.Errorf("unable to open file")
	ErrUnableToReadFile   = fmt.Errorf("unable to read file")
	ErrUnableToResetFile  = fmt.Errorf("unable to reset file position")
	ErrCloudinaryInit     = fmt.Errorf("failed to initialize Cloudinary")
	ErrMissingCredentials = fmt.Errorf("cloudinary credentials are missing")
	ErrUploadFailed       = fmt.Errorf("failed to upload file")
	ErrDeleteFailed       = fmt.Errorf("failed to delete file")
)

// Singleton pattern implementation.
var (
	cloudinaryInstance *CloudinaryService
	cloudinaryOnce     sync.Once
	initErr            error
)

// GetCloudinaryService returns a singleton instance of CloudinaryService.
func GetCloudinaryService() (*CloudinaryService, error) {
	cloudinaryOnce.Do(func() {
		cloudinaryInstance, initErr = initializeCloudinary()
	})
	return cloudinaryInstance, initErr
}

// ptrBool returns a pointer to a bool.
func ptrBool(b bool) *bool {
	return &b
}

// initializeCloudinary creates a new CloudinaryService from environment variables.
func initializeCloudinary() (*CloudinaryService, error) {
	cloudName := os.Getenv("CLOUDINARY_CLOUD_NAME")
	apiKey := os.Getenv("CLOUDINARY_API_KEY")
	apiSecret := os.Getenv("CLOUDINARY_API_SECRET")

	if cloudName == "" || apiKey == "" || apiSecret == "" {
		return nil, ErrMissingCredentials
	}

	cld, err := cloudinary.NewFromParams(cloudName, apiKey, apiSecret)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrCloudinaryInit, err)
	}

	logger, err := zap.NewProduction()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize logger: %v", err)
	}

	config := DefaultConfig()
	// Override with environment variables if set
	if maxSize := os.Getenv("MAX_FILE_SIZE"); maxSize != "" {
		if size, err := strconv.ParseInt(maxSize, 10, 64); err == nil {
			config.MaxFileSize = size
		}
	}

	service := &CloudinaryService{
		Client: cld,
		Config: config,
		Logger: logger,
	}

	logger.Info("Cloudinary service initialized successfully")
	return service, nil
}

// New creates a new CloudinaryService (for backward compatibility).
func New() (*CloudinaryService, error) {
	return GetCloudinaryService()
}

// UploadFile handles different file types appropriately
func (c *CloudinaryService) UploadFile(ctx context.Context, file *multipart.FileHeader, folder string) (*UploadResult, error) {
    // Common setup code
    startTime := time.Now()
    c.Logger.Info("Starting file upload", zap.String("filename", file.Filename), zap.Int64("size", file.Size))
    
    // Create context with timeout
    ctx, cancel := context.WithTimeout(ctx, c.Config.UploadTimeout)
    defer cancel()
    
    // Open the uploaded file
    src, err := file.Open()
    if err != nil {
        c.Logger.Error("Failed to open file", zap.Error(err))
        return nil, fmt.Errorf("%w: %v", ErrUnableToOpenFile, err)
    }
    defer src.Close()
    
    // Detect content type
    buffer := make([]byte, 512)
    _, err = src.Read(buffer)
    if err != nil && err != io.EOF {
        c.Logger.Error("Failed to read file for content detection", zap.Error(err))
        return nil, fmt.Errorf("%w: %v", ErrUnableToReadFile, err)
    }
    
    // Reset file pointer
    _, err = src.Seek(0, io.SeekStart)
    if err != nil {
        c.Logger.Error("Failed to reset file position", zap.Error(err))
        return nil, fmt.Errorf("%w: %v", ErrUnableToResetFile, err)
    }
    
    contentType := http.DetectContentType(buffer)
    c.Logger.Info("Detected content type", 
        zap.String("filename", file.Filename),
        zap.String("content_type", contentType))
    
    // Configure upload parameters based on content type
    uploadParams := uploader.UploadParams{
        Folder:         folder,
        UseFilename:    ptrBool(true),
        UniqueFilename: ptrBool(true),
        ResourceType:   "auto", // Use auto for all file types
    }
    
    // Perform the upload with retries
    var result *uploader.UploadResult
    operation := func() error {
        var opErr error
        result, opErr = c.Client.Upload.Upload(ctx, src, uploadParams)
        return opErr
    }
    
    b := backoff.NewExponentialBackOff()
    b.MaxElapsedTime = c.Config.UploadTimeout / 2
    err = backoff.RetryNotify(
        operation,
        backoff.WithMaxRetries(b, uint64(c.Config.MaxRetries)),
        func(err error, d time.Duration) {
            c.Logger.Warn("Upload attempt failed",
                zap.String("filename", file.Filename),
                zap.Error(err),
                zap.Duration("backoff", d))
        },
    )
    
    if err != nil {
        c.Logger.Error("All upload attempts failed",
            zap.String("filename", file.Filename),
            zap.Int("attempts", c.Config.MaxRetries),
            zap.Error(err))
        return nil, fmt.Errorf("%w after %d attempts: %v", ErrUploadFailed, c.Config.MaxRetries, err)
    }
    
    // Use the URL as Cloudinary returns it (no modification)
    resultURL := result.SecureURL
    
    duration := time.Since(startTime)
    c.Logger.Info("File uploaded successfully",
        zap.String("filename", file.Filename),
        zap.Int64("size", file.Size),
        zap.Duration("duration", duration),
        zap.String("public_id", result.PublicID),
        zap.String("url", resultURL))
    
    return &UploadResult{
        URL:      resultURL,
        PublicID: result.PublicID,
        Format:   result.Format,
        Size:     result.Bytes,
    }, nil
}

// DeleteFile removes a file from Cloudinary by its public ID.
func (c *CloudinaryService) DeleteFile(ctx context.Context, publicID string) error {
	startTime := time.Now()
	c.Logger.Info("Starting file deletion", zap.String("public_id", publicID))

	ctx, cancel := context.WithTimeout(ctx, c.Config.DeleteTimeout)
	defer cancel()

	_, err := c.Client.Upload.Destroy(ctx, uploader.DestroyParams{
		PublicID: publicID,
	})

	if err != nil {
		c.Logger.Error("Failed to delete file",
			zap.String("public_id", publicID),
			zap.Error(err))
		return fmt.Errorf("%w: %v", ErrDeleteFailed, err)
	}

	c.Logger.Info("File deleted successfully",
		zap.String("public_id", publicID),
		zap.Duration("duration", time.Since(startTime)))
	return nil
}

// ValidateFile performs comprehensive validation on the file.
func (c *CloudinaryService) ValidateFile(ctx context.Context, file *multipart.FileHeader) error {
    select {
    case <-ctx.Done():
        return ctx.Err()
    default:
    }

    // Size validation
    if file.Size > c.Config.MaxFileSize {
        c.Logger.Warn("File size validation failed",
            zap.String("filename", file.Filename),
            zap.Int64("size", file.Size),
            zap.Int64("limit", c.Config.MaxFileSize))
        return fmt.Errorf("%w: %d bytes exceeds %d bytes", ErrFileTooLarge, file.Size, c.Config.MaxFileSize)
    }

    // Open file for content type detection
    src, err := file.Open()
    if err != nil {
        c.Logger.Error("Failed to open file for validation", zap.Error(err))
        return fmt.Errorf("%w: %v", ErrUnableToOpenFile, err)
    }
    defer src.Close()

    // Read buffer for MIME detection
    buffer := make([]byte, 512)
    n, err := src.Read(buffer)
    if err != nil && err != io.EOF {
        c.Logger.Error("Failed to read file for validation", zap.Error(err))
        return fmt.Errorf("%w: %v", ErrUnableToReadFile, err)
    }

    // Reset file pointer
    _, err = src.Seek(0, io.SeekStart)
    if err != nil {
        c.Logger.Error("Failed to reset file position", zap.Error(err))
        return fmt.Errorf("%w: %v", ErrUnableToResetFile, err)
    }

    // Detect content type
    contentType := http.DetectContentType(buffer[:n])
    c.Logger.Info("Detected content type",
        zap.String("filename", file.Filename),
        zap.String("content_type", contentType))

    // Extract the general type category
    typeCategory := strings.Split(contentType, "/")[0]

    // Validate content type
    if !c.Config.AllowedTypes[contentType] {
        c.Logger.Warn("Content type not allowed",
            zap.String("filename", file.Filename),
            zap.String("content_type", contentType))
        return fmt.Errorf("%w: %s", ErrInvalidContentType, contentType)
    }

    // Get file extension
    filename := file.Filename
    ext := ""
    for i := len(filename) - 1; i >= 0; i-- {
        if filename[i] == '.' {
            ext = strings.ToLower(filename[i:])
            break
        }
    }

    // Industry standard practice: For images, validate that the extension belongs to an image file
    // but don't strictly enforce a match between extension and detected content type
    if typeCategory == "image" {
        validImageExts := []string{".jpg", ".jpeg", ".png", ".gif", ".webp", ".svg", ".bmp", ".tiff", ".tif"}
        
        isValidImageExt := false
        for _, validExt := range validImageExts {
            if ext == validExt {
                isValidImageExt = true
                break
            }
        }
        
        if !isValidImageExt {
            c.Logger.Warn("Invalid image file extension",
                zap.String("filename", filename),
                zap.String("extension", ext),
                zap.String("content_type", contentType))
            return fmt.Errorf("%w: %s is not a valid image extension", ErrInvalidExtension, ext)
        }
        
        c.Logger.Info("Image file passed validation", 
            zap.String("filename", file.Filename),
            zap.String("detected_content_type", contentType),
            zap.String("extension", ext))
        return nil
    }

    // For documents and other file types, validate extension against content type
    if extensions, exists := c.Config.ValidExtensions[contentType]; exists {
        if !slices.Contains(extensions, ext) {
            c.Logger.Warn("File extension validation failed",
                zap.String("filename", filename),
                zap.String("extension", ext),
                zap.String("content_type", contentType))
            
            // Provide more helpful error message
            expectedExtensions := strings.Join(extensions, ", ")
            return fmt.Errorf("%w: file has extension %s but content is %s (expected: %s)", 
                ErrInvalidExtension, ext, contentType, expectedExtensions)
        }
    } else {
        c.Logger.Warn("No extension validation defined for content type",
            zap.String("filename", filename),
            zap.String("content_type", contentType))
    }

    c.Logger.Info("File passed validation", zap.String("filename", file.Filename))
    return nil
}

// GetFileTypeCategory categorizes file types into broader categories.
func GetFileTypeCategory(contentType string) string {
	switch {
	case strings.HasPrefix(contentType, "image/"):
		return "image"
	case strings.HasPrefix(contentType, "video/"):
		return "video"
	case strings.HasPrefix(contentType, "audio/"):
		return "audio"
	case contentType == "application/pdf" || strings.Contains(contentType, "wordprocessing"):
		return "document"
	case strings.Contains(contentType, "spreadsheet") || contentType == "text/csv":
		return "spreadsheet"
	case contentType == "text/plain" || contentType == "application/json":
		return "text"
	case contentType == "application/zip":
		return "archive"
	default:
		return "other"
	}
}
