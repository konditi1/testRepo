// internal/services/file_services
package services

import (
	"context"
	"evalhub/internal/cache"
	"evalhub/internal/events"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/cloudinary/cloudinary-go/v2"
	"github.com/cloudinary/cloudinary-go/v2/api/uploader"
	"go.uber.org/zap"
)

// fileService implements FileService with enterprise file management
type fileService struct {
	cloudinary *cloudinary.Cloudinary
	cache      cache.Cache
	events     events.EventBus
	logger     *zap.Logger
	config     *FileServiceConfig
}

// FileServiceConfig holds file service configuration
type FileServiceConfig struct {
	MaxImageSize      int64         `json:"max_image_size"`    // 5MB default
	MaxDocumentSize   int64         `json:"max_document_size"` // 10MB default
	AllowedImageTypes []string      `json:"allowed_image_types"`
	AllowedDocTypes   []string      `json:"allowed_doc_types"`
	UploadTimeout     time.Duration `json:"upload_timeout"`
	EnableCompression bool          `json:"enable_compression"`
	Quality           int           `json:"quality"` // Image quality 1-100
}

// NewFileService creates a new enterprise file service
func NewFileService(
	cloudinary *cloudinary.Cloudinary,
	cache cache.Cache,
	events events.EventBus,
	logger *zap.Logger,
	config *FileServiceConfig,
) FileService {
	if config == nil {
		config = DefaultFileConfig()
	}

	return &fileService{
		cloudinary: cloudinary,
		cache:      cache,
		events:     events,
		logger:     logger,
		config:     config,
	}
}

// DefaultFileConfig returns default file service configuration
func DefaultFileConfig() *FileServiceConfig {
	return &FileServiceConfig{
		MaxImageSize:    5 * 1024 * 1024,  // 5MB
		MaxDocumentSize: 10 * 1024 * 1024, // 10MB
		AllowedImageTypes: []string{
			"image/jpeg", "image/jpg", "image/png",
			"image/gif", "image/webp",
		},
		AllowedDocTypes: []string{
			"application/pdf", "application/msword",
			"application/vnd.openxmlformats-officedocument.wordprocessingml.document",
			"text/plain",
		},
		UploadTimeout:     2 * time.Minute,
		EnableCompression: true,
		Quality:           85,
	}
}

// ===============================
// IMAGE UPLOAD OPERATIONS
// ===============================

// UploadImage uploads an image with optimization and validation
func (s *fileService) UploadImage(ctx context.Context, req *FileUploadRequest) (*FileUploadResult, error) {
	// Validate request
	if err := s.validateImageUpload(req); err != nil {
		return nil, NewValidationError("image validation failed", err)
	}

	// Create upload context with timeout
	uploadCtx, cancel := context.WithTimeout(ctx, s.config.UploadTimeout)
	defer cancel()

	// Generate folder path
	folder := s.generateUploadFolder(req.Folder, req.UserID)

	// Prepare upload parameters
	uploadParams := uploader.UploadParams{
		Folder:         folder,
		ResourceType:   "image",
		Format:         "auto", // Auto-optimize format
		Transformation: fmt.Sprintf("q_%d/%s", s.config.Quality, s.buildImageTransformation(req)),
		UseFilename:    BoolPtr(false),
		UniqueFilename: BoolPtr(true),
		Tags:           []string{"evalhub", "user_upload"},
	}

	// Upload to Cloudinary
	result, err := s.cloudinary.Upload.Upload(uploadCtx, req.File, uploadParams)
	if err != nil {
		s.logger.Error("Failed to upload image to Cloudinary",
			zap.Error(err),
			zap.Int64("user_id", req.UserID),
			zap.String("filename", req.Filename),
		)
		return nil, NewInternalError("failed to upload image")
	}

	// Create result
	uploadResult := &FileUploadResult{
		URL:      result.SecureURL,
		PublicID: result.PublicID,
		Size:     int64(result.Bytes),
		Format:   result.Format,
		Width:    result.Width,
		Height:   result.Height,
		Type:     "image",
	}

	// Publish upload event
	if err := s.events.Publish(ctx, events.NewFileUploadedEvent(
		"image",
		uploadResult.Size,
		uploadResult.URL,
		uploadResult.PublicID,
		&req.UserID,
	)); err != nil {
		s.logger.Warn("Failed to publish file upload event", zap.Error(err))
	}

	s.logger.Info("Image uploaded successfully",
		zap.Int64("user_id", req.UserID),
		zap.String("public_id", uploadResult.PublicID),
		zap.String("url", uploadResult.URL),
		zap.Int64("size", uploadResult.Size),
	)

	return uploadResult, nil
}

// UploadDocument uploads a document with validation
func (s *fileService) UploadDocument(ctx context.Context, req *FileUploadRequest) (*FileUploadResult, error) {
	// Validate request
	if err := s.validateDocumentUpload(req); err != nil {
		return nil, NewValidationError("document validation failed", err)
	}

	// Create upload context with timeout
	uploadCtx, cancel := context.WithTimeout(ctx, s.config.UploadTimeout)
	defer cancel()

	// Generate folder path
	folder := s.generateUploadFolder(req.Folder, req.UserID)

	// Prepare upload parameters
	uploadParams := uploader.UploadParams{
		Folder:         folder,
		ResourceType:   "raw", // For documents
		UseFilename:    BoolPtr(true),
		UniqueFilename: BoolPtr(true),
		Tags:           []string{"evalhub", "document", "user_upload"},
	}

	// Upload to Cloudinary
	result, err := s.cloudinary.Upload.Upload(uploadCtx, req.File, uploadParams)
	if err != nil {
		s.logger.Error("Failed to upload document to Cloudinary",
			zap.Error(err),
			zap.Int64("user_id", req.UserID),
			zap.String("filename", req.Filename),
		)
		return nil, NewInternalError("failed to upload document")
	}

	// Create result
	uploadResult := &FileUploadResult{
		URL:      result.SecureURL,
		PublicID: result.PublicID,
		Size:     int64(result.Bytes),
		Format:   result.Format,
		Type:     "document",
		Filename: req.Filename,
	}

	// Publish upload event
	if err := s.events.Publish(ctx, &events.FileUploadedEvent{
		BaseEvent: events.BaseEvent{
			EventID:   events.GenerateEventID(),
			EventType: "file.document_uploaded",
			Timestamp: time.Now(),
			UserID:    &req.UserID,
		},
		FileType: "document",
		FileSize: uploadResult.Size,
		URL:      uploadResult.URL,
		PublicID: uploadResult.PublicID,
		Filename: req.Filename}); err != nil {
		s.logger.Warn("Failed to publish file upload event", zap.Error(err))
	}

	s.logger.Info("Document uploaded successfully",
		zap.Int64("user_id", req.UserID),
		zap.String("public_id", uploadResult.PublicID),
		zap.String("filename", req.Filename),
		zap.Int64("size", uploadResult.Size),
	)

	return uploadResult, nil
}

// ===============================
// FILE MANAGEMENT OPERATIONS
// ===============================

// DeleteFile deletes a file from Cloudinary
func (s *fileService) DeleteFile(ctx context.Context, publicID string) error {
	if publicID == "" {
		return NewValidationError("public ID is required", nil)
	}

	// Create context with timeout
	deleteCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	// Delete from Cloudinary
	result, err := s.cloudinary.Upload.Destroy(deleteCtx, uploader.DestroyParams{
		PublicID: publicID,
	})

	if err != nil {
		s.logger.Error("Failed to delete file from Cloudinary",
			zap.Error(err),
			zap.String("public_id", publicID),
		)
		return NewInternalError("failed to delete file")
	}

	if result.Result != "ok" {
		s.logger.Warn("File deletion result was not OK",
			zap.String("public_id", publicID),
			zap.String("result", result.Result),
		)
		return NewInternalError("file deletion was not successful")
	}

	s.logger.Info("File deleted successfully",
		zap.String("public_id", publicID),
	)

	return nil
}

// GetFileInfo retrieves file information from Cloudinary
func (s *fileService) GetFileInfo(ctx context.Context, publicID string) (*FileInfo, error) {
	if publicID == "" {
		return nil, NewValidationError("public ID is required", nil)
	}

	// Try cache first
	cacheKey := fmt.Sprintf("file_info:%s", publicID)
	if cachedInfo, found := s.cache.Get(ctx, cacheKey); found {
		if fileInfo, ok := cachedInfo.(*FileInfo); ok {
			return fileInfo, nil
		}
	}

	// Get from Cloudinary API (this would require additional Cloudinary admin API setup)
	// For now, return basic info
	fileInfo := &FileInfo{
		PublicID: publicID,
		// Other fields would be populated from API call
	}

	// Cache the result
	if err := s.cache.Set(ctx, cacheKey, fileInfo, 30*time.Minute); err != nil {
		s.logger.Warn("Failed to cache file info", zap.Error(err))
	}

	return fileInfo, nil
}

// ===============================
// VALIDATION METHODS
// ===============================

// validateImageUpload validates image upload requirements
func (s *fileService) validateImageUpload(req *FileUploadRequest) error {
	// Check file size
	if req.Size > s.config.MaxImageSize {
		return fmt.Errorf("image too large (max %d bytes)", s.config.MaxImageSize)
	}

	// Check content type
	if !s.isAllowedImageType(req.ContentType) {
		return fmt.Errorf("unsupported image type: %s", req.ContentType)
	}

	// Validate filename
	if err := s.validateFilename(req.Filename); err != nil {
		return err
	}

	return nil
}

// validateDocumentUpload validates document upload requirements
func (s *fileService) validateDocumentUpload(req *FileUploadRequest) error {
	// Check file size
	if req.Size > s.config.MaxDocumentSize {
		return fmt.Errorf("document too large (max %d bytes)", s.config.MaxDocumentSize)
	}

	// Check content type
	if !s.isAllowedDocumentType(req.ContentType) {
		return fmt.Errorf("unsupported document type: %s", req.ContentType)
	}

	// Validate filename
	if err := s.validateFilename(req.Filename); err != nil {
		return err
	}

	return nil
}

// validateFilename validates filename for security
func (s *fileService) validateFilename(filename string) error {
	if filename == "" {
		return fmt.Errorf("filename is required")
	}

	// Check for dangerous characters
	dangerousChars := []string{"../", "..\\", "<", ">", ":", "\"", "|", "?", "*"}
	for _, char := range dangerousChars {
		if strings.Contains(filename, char) {
			return fmt.Errorf("filename contains invalid characters")
		}
	}

	// Check file extension
	ext := strings.ToLower(filepath.Ext(filename))
	if ext == "" {
		return fmt.Errorf("file must have an extension")
	}

	return nil
}

// isAllowedImageType checks if content type is allowed for images
func (s *fileService) isAllowedImageType(contentType string) bool {
	for _, allowedType := range s.config.AllowedImageTypes {
		if contentType == allowedType {
			return true
		}
	}
	return false
}

// isAllowedDocumentType checks if content type is allowed for documents
func (s *fileService) isAllowedDocumentType(contentType string) bool {
	for _, allowedType := range s.config.AllowedDocTypes {
		if contentType == allowedType {
			return true
		}
	}
	return false
}

// ===============================
// HELPER METHODS
// ===============================

// generateUploadFolder creates a structured folder path
func (s *fileService) generateUploadFolder(baseFolder string, userID int64) string {
	if baseFolder == "" {
		baseFolder = "uploads"
	}

	// Create hierarchical folder structure: evalhub/uploads/2024/01/user_123
	now := time.Now()
	return fmt.Sprintf("evalhub/%s/%d/%02d/user_%d",
		baseFolder, now.Year(), now.Month(), userID)
}

// buildImageTransformation creates Cloudinary transformation parameters
func (s *fileService) buildImageTransformation(req *FileUploadRequest) string {
	transformations := []string{}

	// Auto-optimize format and quality
	if s.config.EnableCompression {
		transformations = append(transformations, "f_auto", "q_auto")
	}

	// Limit maximum dimensions for performance
	transformations = append(transformations, "w_2048", "h_2048", "c_limit")

	// Progressive loading for better UX
	transformations = append(transformations, "fl_progressive")

	return strings.Join(transformations, ",")
}

// ===============================
// BATCH OPERATIONS
// ===============================

// DeleteMultipleFiles deletes multiple files in batch
func (s *fileService) DeleteMultipleFiles(ctx context.Context, publicIDs []string) (*BatchDeleteResult, error) {
	if len(publicIDs) == 0 {
		return &BatchDeleteResult{}, nil
	}

	result := &BatchDeleteResult{
		Total:   len(publicIDs),
		Deleted: 0,
		Failed:  0,
		Errors:  []string{},
	}

	// Delete files concurrently with a limit
	const maxConcurrent = 5
	semaphore := make(chan struct{}, maxConcurrent)
	errors := make(chan error, len(publicIDs))
	success := make(chan bool, len(publicIDs))

	for _, publicID := range publicIDs {
		go func(id string) {
			semaphore <- struct{}{}        // Acquire
			defer func() { <-semaphore }() // Release

			if err := s.DeleteFile(ctx, id); err != nil {
				errors <- fmt.Errorf("failed to delete %s: %w", id, err)
				success <- false
			} else {
				success <- true
			}
		}(publicID)
	}

	// Collect results
	for i := 0; i < len(publicIDs); i++ {
		select {
		case err := <-errors:
			result.Failed++
			result.Errors = append(result.Errors, err.Error())
		case <-success:
			result.Deleted++
		case <-ctx.Done():
			result.Failed += len(publicIDs) - i
			result.Errors = append(result.Errors, "operation cancelled")
			return result, ctx.Err()
		}
	}

	s.logger.Info("Batch file deletion completed",
		zap.Int("total", result.Total),
		zap.Int("deleted", result.Deleted),
		zap.Int("failed", result.Failed),
	)

	return result, nil
}

// ===============================
// FILE ANALYSIS OPERATIONS
// ===============================

// AnalyzeFile analyzes file content and metadata
func (s *fileService) AnalyzeFile(ctx context.Context, req *FileUploadRequest) (*FileAnalysis, error) {
	analysis := &FileAnalysis{
		Filename:    req.Filename,
		Size:        req.Size,
		ContentType: req.ContentType,
		IsValid:     true,
		Issues:      []string{},
	}

	// Basic validation
	if req.Size == 0 {
		analysis.IsValid = false
		analysis.Issues = append(analysis.Issues, "file is empty")
	}

	// Check for malicious content (basic checks)
	if s.containsSuspiciousContent(req.Filename) {
		analysis.IsValid = false
		analysis.Issues = append(analysis.Issues, "filename contains suspicious patterns")
	}

	// Analyze content type vs file extension
	expectedType := s.getExpectedContentType(req.Filename)
	if expectedType != "" && expectedType != req.ContentType {
		analysis.Issues = append(analysis.Issues,
			fmt.Sprintf("content type mismatch: expected %s, got %s", expectedType, req.ContentType))
	}

	// File-specific analysis would go here
	// (virus scanning, content analysis, etc.)

	return analysis, nil
}

// containsSuspiciousContent checks for suspicious patterns in filename
func (s *fileService) containsSuspiciousContent(filename string) bool {
	suspicious := []string{
		"script", "eval", "exec", "cmd", "powershell",
		".exe", ".bat", ".sh", ".ps1", ".vbs",
	}

	lower := strings.ToLower(filename)
	for _, pattern := range suspicious {
		if strings.Contains(lower, pattern) {
			return true
		}
	}
	return false
}

// getExpectedContentType returns expected content type for file extension
func (s *fileService) getExpectedContentType(filename string) string {
	ext := strings.ToLower(filepath.Ext(filename))

	typeMap := map[string]string{
		".jpg":  "image/jpeg",
		".jpeg": "image/jpeg",
		".png":  "image/png",
		".gif":  "image/gif",
		".pdf":  "application/pdf",
		".doc":  "application/msword",
		".docx": "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
		".txt":  "text/plain",
	}

	return typeMap[ext]
}

// ===============================
// IMAGE PROCESSING
// ===============================

// ProcessImageVariants generates multiple variants of an image with different sizes and formats
func (s *fileService) ProcessImageVariants(ctx context.Context, req *ProcessImageVariantsRequest) (*ImageVariantsResult, error) {
	if req == nil {
		return nil, NewValidationError("request cannot be nil", nil)
	}

	if req.PublicID == "" {
		return nil, NewValidationError("public ID is required", nil)
	}

	if len(req.Variants) == 0 {
		return nil, NewValidationError("at least one variant configuration is required", nil)
	}

	result := &ImageVariantsResult{
		PublicID: req.PublicID,
		Variants: make(map[string]FileUploadResult),
	}

	// Process each variant configuration
	for _, variant := range req.Variants {
		if variant.Name == "" {
			variant.Name = fmt.Sprintf("variant_%dx%d", variant.Width, variant.Height)
		}

		// Create transformation string
		transformation := fmt.Sprintf("c_%s,w_%d,h_%d", variant.Crop, variant.Width, variant.Height)
		if variant.Crop == "" {
			transformation = fmt.Sprintf("w_%d,h_%d,c_scale", variant.Width, variant.Height)
		}

		// Apply additional optimization
		transformation += ",f_auto,q_auto:good"

		// Generate the transformed image URL with transformations
		transformedURL, err := s.cloudinary.Image(req.PublicID + "/" + transformation)
		if err != nil {
			s.logger.Error("Failed to generate transformed image URL",
				zap.Error(err),
				zap.String("public_id", req.PublicID),
				zap.String("transformation", transformation),
			)
			continue
		}

		urlStr, err := transformedURL.String()
		if err != nil {
			s.logger.Error("Failed to convert URL to string",
				zap.Error(err),
				zap.String("public_id", req.PublicID),
				zap.String("transformation", transformation),
			)
			continue
		}

		// Store the result
		result.Variants[variant.Name] = FileUploadResult{
			PublicID: req.PublicID,
			URL:      urlStr,
			Width:    variant.Width,
			Height:   variant.Height,
			Format:   "auto",
		}
	}

	// Publish an event for the image processing
	if err := s.events.Publish(ctx, events.NewImageProcessedEvent(
		req.PublicID,
		len(result.Variants),
		&req.UserID,
	)); err != nil {
		s.logger.Warn("Failed to publish image processed event", zap.Error(err))
	}

	s.logger.Info("Generated image variants",
		zap.String("public_id", req.PublicID),
		zap.Int("variant_count", len(result.Variants)),
	)

	return result, nil
}

// ===============================
// URL GENERATION
// ===============================

// GenerateUploadURL generates a pre-signed URL for direct uploads to Cloudinary
// Note: Cloudinary doesn't support pre-signed URLs like S3. Instead, we'll return
// the necessary parameters for client-side uploads using the Cloudinary Upload Widget or API.
func (s *fileService) GenerateUploadURL(ctx context.Context, req *GenerateUploadURLRequest) (*UploadURLResult, error) {
	if req == nil {
		return nil, fmt.Errorf("request cannot be nil")
	}

	// Set default values if not provided
	if req.Folder == "" {
		req.Folder = "uploads"
	}
	if req.ResourceType == "" {
		req.ResourceType = "auto"
	}

	// Generate a unique public ID if not provided
	publicID := req.PublicID
	if publicID == "" {
		publicID = fmt.Sprintf("%s/%d", req.Folder, time.Now().UnixNano())
	}

	// For Cloudinary, we don't actually generate a pre-signed URL like S3
	// Instead, we'll return the necessary parameters for client-side upload
	// The actual upload will need to be handled by the client using Cloudinary's JS SDK or Upload Widget

	s.logger.Info("Generated upload parameters",
		zap.String("public_id", publicID),
		zap.String("folder", req.Folder),
		zap.String("resource_type", req.ResourceType),
	)

	// Return the result with the public ID and other metadata
	// The client will need to handle the actual file upload
	result := &UploadURLResult{
		PublicID: publicID,
		Fields: map[string]string{
			"folder":        req.Folder,
			"public_id":     publicID,
			"resource_type": req.ResourceType,
		},
		ExpiresAt: time.Now().Add(1 * time.Hour), // Set an expiration time
	}

	return result, nil
}

// GenerateSignedURL generates a signed URL for secure file access
func (s *fileService) GenerateSignedURL(ctx context.Context, publicID string, options *URLOptions) (string, error) {
	if publicID == "" {
		return "", NewValidationError("public ID is required", nil)
	}

	if options == nil {
		options = &URLOptions{
			ExpiresIn: 24 * time.Hour,
		}
	}

	// Generate expiration timestamp
	expiresAt := time.Now().Add(options.ExpiresIn).Unix()

	// Build transformation string
	var transformation string
	if options.Width > 0 || options.Height > 0 {
		transformation = fmt.Sprintf("w_%d,h_%d,c_limit", options.Width, options.Height)
	}

	// Generate signed URL with expiration
	// Note: In a real implementation, you would use Cloudinary's SDK to properly sign the URL
	// This is a simplified example showing where the expiration would be used
	url := fmt.Sprintf("https://res.cloudinary.com/evalhub/image/upload/%s/%s?expires=%d", 
		transformation, publicID, expiresAt)

	return url, nil
}

// ===============================
// CLEANUP OPERATIONS
// ===============================

// CleanupOrphanedFiles removes files that are no longer referenced
func (s *fileService) CleanupOrphanedFiles(ctx context.Context, olderThan time.Time) (*CleanupResult, error) {
	// This would implement cleanup logic to find and remove orphaned files
	// For now, return a placeholder result

	result := &CleanupResult{
		FilesProcessed: 0,
		FilesDeleted:   0,
		SpaceFreed:     0,
		Errors:         []string{},
	}

	s.logger.Info("File cleanup completed",
		zap.Int("processed", result.FilesProcessed),
		zap.Int("deleted", result.FilesDeleted),
		zap.Int64("space_freed", result.SpaceFreed),
	)

	return result, nil
}

// ===============================
// STATISTICS
// ===============================

// GetUploadStatistics returns file upload statistics
func (s *fileService) GetUploadStatistics(ctx context.Context, userID *int64, days int) (*UploadStatistics, error) {
	// This would query upload statistics from events or database
	// For now, return placeholder data

	stats := &UploadStatistics{
		TotalFiles:    0,
		TotalSize:     0,
		ImageCount:    0,
		DocumentCount: 0,
		AverageSize:   0,
	}

	return stats, nil
}

func BoolPtr(b bool) *bool {
	return &b
}
