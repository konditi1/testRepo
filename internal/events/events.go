package events

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
)

// ===============================
// EVENT INTERFACE
// ===============================

// Event represents a domain event
type Event interface {
	GetEventID() string
	GetEventType() string
	GetTimestamp() time.Time
	GetUserID() *int64
	GetMetadata() map[string]interface{}
}

// BaseEvent provides common event functionality
type BaseEvent struct {
	EventID   string                 `json:"event_id"`
	EventType string                 `json:"event_type"`
	Timestamp time.Time              `json:"timestamp"`
	UserID    *int64                 `json:"user_id,omitempty"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// GetEventID returns the event ID
func (e *BaseEvent) GetEventID() string {
	return e.EventID
}

// GetEventType returns the event type
func (e *BaseEvent) GetEventType() string {
	return e.EventType
}

// GetTimestamp returns the event timestamp
func (e *BaseEvent) GetTimestamp() time.Time {
	return e.Timestamp
}

// GetUserID returns the user ID associated with the event
func (e *BaseEvent) GetUserID() *int64 {
	return e.UserID
}

// GetMetadata returns the event metadata
func (e *BaseEvent) GetMetadata() map[string]interface{} {
	return e.Metadata
}

// ===============================
// EVENT BUS INTERFACE
// ===============================

// EventBus defines the event publishing and subscription interface
type EventBus interface {
	// Publishing
	Publish(ctx context.Context, event Event) error
	PublishAsync(ctx context.Context, event Event) error
	PublishBatch(ctx context.Context, events []Event) error

	// Subscription
	Subscribe(eventType string, handler EventHandler) error
	SubscribePattern(pattern string, handler EventHandler) error
	Unsubscribe(eventType string, handler EventHandler) error

	// Management
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	Health() error
	Stats() *EventBusStats
}

// EventHandler represents an event handler function
type EventHandler interface {
	Handle(ctx context.Context, event Event) error
	GetHandlerID() string
}

// EventHandlerFunc is a function type that implements EventHandler
type EventHandlerFunc struct {
	ID   string
	Func func(ctx context.Context, event Event) error
}

// Handle implements EventHandler
func (f EventHandlerFunc) Handle(ctx context.Context, event Event) error {
	return f.Func(ctx, event)
}

// GetHandlerID implements EventHandler
func (f EventHandlerFunc) GetHandlerID() string {
	return f.ID
}

// EventBusStats represents event bus statistics
type EventBusStats struct {
	EventsPublished    int64         `json:"events_published"`
	EventsProcessed    int64         `json:"events_processed"`
	EventsFailed       int64         `json:"events_failed"`
	HandlersCount      int           `json:"handlers_count"`
	QueueDepth         int           `json:"queue_depth"`
	AverageProcessTime time.Duration `json:"average_process_time"`
	Uptime             time.Duration `json:"uptime"`
}

// ===============================
// IN-MEMORY EVENT BUS
// ===============================

// inMemoryEventBus implements EventBus using in-memory channels
type inMemoryEventBus struct {
	mu                 sync.RWMutex
	handlers           map[string][]EventHandler
	patternHandlers    map[string][]EventHandler
	eventQueue         chan eventMessage
	workerPool         chan struct{}
	logger             *zap.Logger
	stats              *EventBusStats
	startTime          time.Time
	ctx                context.Context
	cancel             context.CancelFunc
	wg                 sync.WaitGroup
	bufferSize         int
	workerCount        int
	processingTimes    []time.Duration
	maxProcessingTimes int
}

// eventMessage wraps an event with context
type eventMessage struct {
	ctx       context.Context
	event     Event
	timestamp time.Time
}

// EventBusConfig holds configuration for the event bus
type EventBusConfig struct {
	BufferSize     int           `json:"buffer_size" yaml:"buffer_size"`
	WorkerCount    int           `json:"worker_count" yaml:"worker_count"`
	HandlerTimeout time.Duration `json:"handler_timeout" yaml:"handler_timeout"`
	RetryAttempts  int           `json:"retry_attempts" yaml:"retry_attempts"`
	RetryDelay     time.Duration `json:"retry_delay" yaml:"retry_delay"`
	EnableMetrics  bool          `json:"enable_metrics" yaml:"enable_metrics"`
	EnableTracing  bool          `json:"enable_tracing" yaml:"enable_tracing"`
}

// DefaultEventBusConfig returns default configuration
func DefaultEventBusConfig() *EventBusConfig {
	return &EventBusConfig{
		BufferSize:     1000,
		WorkerCount:    5,
		HandlerTimeout: 30 * time.Second,
		RetryAttempts:  3,
		RetryDelay:     time.Second,
		EnableMetrics:  true,
		EnableTracing:  false,
	}
}

// NewInMemoryEventBus creates a new in-memory event bus
func NewInMemoryEventBus(config *EventBusConfig, logger *zap.Logger) EventBus {
	if config == nil {
		config = DefaultEventBusConfig()
	}

	if logger == nil {
		logger = zap.NewNop()
	}

	ctx, cancel := context.WithCancel(context.Background())

	bus := &inMemoryEventBus{
		handlers:           make(map[string][]EventHandler),
		patternHandlers:    make(map[string][]EventHandler),
		eventQueue:         make(chan eventMessage, config.BufferSize),
		workerPool:         make(chan struct{}, config.WorkerCount),
		logger:             logger,
		stats:              &EventBusStats{},
		startTime:          time.Now(),
		ctx:                ctx,
		cancel:             cancel,
		bufferSize:         config.BufferSize,
		workerCount:        config.WorkerCount,
		processingTimes:    make([]time.Duration, 0, 100),
		maxProcessingTimes: 100,
	}

	return bus
}

// Publish publishes an event synchronously
func (b *inMemoryEventBus) Publish(ctx context.Context, event Event) error {
	if event == nil {
		return fmt.Errorf("event cannot be nil")
	}

	b.logger.Debug("Publishing event",
		zap.String("event_id", event.GetEventID()),
		zap.String("event_type", event.GetEventType()),
	)

	// Process immediately in synchronous mode
	if err := b.processEvent(ctx, event); err != nil {
		b.logger.Error("Failed to process event",
			zap.String("event_id", event.GetEventID()),
			zap.String("event_type", event.GetEventType()),
			zap.Error(err),
		)
		b.stats.EventsFailed++
		return err
	}

	b.stats.EventsPublished++
	return nil
}

// PublishAsync publishes an event asynchronously
func (b *inMemoryEventBus) PublishAsync(ctx context.Context, event Event) error {
	if event == nil {
		return fmt.Errorf("event cannot be nil")
	}

	select {
	case b.eventQueue <- eventMessage{ctx: ctx, event: event, timestamp: time.Now()}:
		b.stats.EventsPublished++
		return nil
	case <-ctx.Done():
		return ctx.Err()
	default:
		return fmt.Errorf("event queue is full")
	}
}

// PublishBatch publishes multiple events
func (b *inMemoryEventBus) PublishBatch(ctx context.Context, events []Event) error {
	var errors []error

	for _, event := range events {
		if err := b.PublishAsync(ctx, event); err != nil {
			errors = append(errors, err)
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("failed to publish %d out of %d events", len(errors), len(events))
	}

	return nil
}

// Subscribe subscribes to events of a specific type
func (b *inMemoryEventBus) Subscribe(eventType string, handler EventHandler) error {
	if eventType == "" {
		return fmt.Errorf("event type cannot be empty")
	}
	if handler == nil {
		return fmt.Errorf("handler cannot be nil")
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	b.handlers[eventType] = append(b.handlers[eventType], handler)
	b.stats.HandlersCount++

	b.logger.Info("Handler subscribed",
		zap.String("event_type", eventType),
		zap.String("handler_id", handler.GetHandlerID()),
	)

	return nil
}

// SubscribePattern subscribes to events matching a pattern
func (b *inMemoryEventBus) SubscribePattern(pattern string, handler EventHandler) error {
	if pattern == "" {
		return fmt.Errorf("pattern cannot be empty")
	}
	if handler == nil {
		return fmt.Errorf("handler cannot be nil")
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	b.patternHandlers[pattern] = append(b.patternHandlers[pattern], handler)
	b.stats.HandlersCount++

	b.logger.Info("Pattern handler subscribed",
		zap.String("pattern", pattern),
		zap.String("handler_id", handler.GetHandlerID()),
	)

	return nil
}

// Unsubscribe removes a handler for a specific event type
func (b *inMemoryEventBus) Unsubscribe(eventType string, handler EventHandler) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	handlers := b.handlers[eventType]
	for i, h := range handlers {
		if h.GetHandlerID() == handler.GetHandlerID() {
			// Remove handler from slice
			b.handlers[eventType] = append(handlers[:i], handlers[i+1:]...)
			b.stats.HandlersCount--

			b.logger.Info("Handler unsubscribed",
				zap.String("event_type", eventType),
				zap.String("handler_id", handler.GetHandlerID()),
			)
			return nil
		}
	}

	return fmt.Errorf("handler not found")
}

// Start starts the event bus workers
func (b *inMemoryEventBus) Start(ctx context.Context) error {
	b.logger.Info("Starting event bus", zap.Int("worker_count", b.workerCount))

	// Start worker goroutines
	for i := 0; i < b.workerCount; i++ {
		b.wg.Add(1)
		go b.worker(i)
	}

	return nil
}

// Stop stops the event bus
func (b *inMemoryEventBus) Stop(ctx context.Context) error {
	b.logger.Info("Stopping event bus")

	// Cancel context to stop workers
	b.cancel()

	// Wait for workers to finish with timeout
	done := make(chan struct{})
	go func() {
		b.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		b.logger.Info("Event bus stopped successfully")
	case <-ctx.Done():
		b.logger.Warn("Event bus stop timeout")
		return ctx.Err()
	}

	return nil
}

// Health checks the health of the event bus
func (b *inMemoryEventBus) Health() error {
	select {
	case <-b.ctx.Done():
		return fmt.Errorf("event bus is stopped")
	default:
	}

	// Check queue depth
	queueDepth := len(b.eventQueue)
	if queueDepth > b.bufferSize*80/100 { // 80% threshold
		return fmt.Errorf("event queue is %d%% full", queueDepth*100/b.bufferSize)
	}

	return nil
}

// Stats returns event bus statistics
func (b *inMemoryEventBus) Stats() *EventBusStats {
	b.mu.RLock()
	defer b.mu.RUnlock()

	stats := *b.stats // Copy stats
	stats.QueueDepth = len(b.eventQueue)
	stats.Uptime = time.Since(b.startTime)

	// Calculate average processing time
	if len(b.processingTimes) > 0 {
		var total time.Duration
		for _, t := range b.processingTimes {
			total += t
		}
		stats.AverageProcessTime = total / time.Duration(len(b.processingTimes))
	}

	return &stats
}

// worker processes events from the queue
func (b *inMemoryEventBus) worker(workerID int) {
	defer b.wg.Done()

	b.logger.Debug("Event bus worker started", zap.Int("worker_id", workerID))

	for {
		select {
		case msg := <-b.eventQueue:
			start := time.Now()

			if err := b.processEvent(msg.ctx, msg.event); err != nil {
				b.logger.Error("Failed to process event",
					zap.Int("worker_id", workerID),
					zap.String("event_id", msg.event.GetEventID()),
					zap.String("event_type", msg.event.GetEventType()),
					zap.Error(err),
				)
				b.stats.EventsFailed++
			} else {
				b.stats.EventsProcessed++
			}

			// Record processing time
			processingTime := time.Since(start)
			b.recordProcessingTime(processingTime)

		case <-b.ctx.Done():
			b.logger.Debug("Event bus worker stopped", zap.Int("worker_id", workerID))
			return
		}
	}
}

// processEvent processes a single event
func (b *inMemoryEventBus) processEvent(ctx context.Context, event Event) error {
	b.mu.RLock()
	defer b.mu.RUnlock()

	eventType := event.GetEventType()
	var allHandlers []EventHandler

	// Get direct handlers
	if handlers, exists := b.handlers[eventType]; exists {
		allHandlers = append(allHandlers, handlers...)
	}

	// Get pattern handlers
	for pattern, handlers := range b.patternHandlers {
		if matchesPattern(eventType, pattern) {
			allHandlers = append(allHandlers, handlers...)
		}
	}

	if len(allHandlers) == 0 {
		b.logger.Debug("No handlers found for event",
			zap.String("event_type", eventType),
			zap.String("event_id", event.GetEventID()),
		)
		return nil
	}

	// Process handlers
	var errors []error
	for _, handler := range allHandlers {
		if err := b.executeHandler(ctx, handler, event); err != nil {
			errors = append(errors, err)
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("failed to execute %d out of %d handlers", len(errors), len(allHandlers))
	}

	return nil
}

// executeHandler executes a single handler with timeout and recovery
func (b *inMemoryEventBus) executeHandler(ctx context.Context, handler EventHandler, event Event) error {
	defer func() {
		if r := recover(); r != nil {
			b.logger.Error("Handler panicked",
				zap.String("handler_id", handler.GetHandlerID()),
				zap.String("event_type", event.GetEventType()),
				zap.Any("panic", r),
			)
		}
	}()

	// Create timeout context
	handlerCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	return handler.Handle(handlerCtx, event)
}

// recordProcessingTime records processing time for statistics
func (b *inMemoryEventBus) recordProcessingTime(duration time.Duration) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.processingTimes = append(b.processingTimes, duration)

	// Keep only the last N processing times
	if len(b.processingTimes) > b.maxProcessingTimes {
		b.processingTimes = b.processingTimes[1:]
	}
}

// matchesPattern checks if an event type matches a pattern
func matchesPattern(eventType, pattern string) bool {
	// Simple wildcard matching
	if pattern == "*" {
		return true
	}

	// Prefix matching
	if len(pattern) > 0 && pattern[len(pattern)-1] == '*' {
		prefix := pattern[:len(pattern)-1]
		return len(eventType) >= len(prefix) && eventType[:len(prefix)] == prefix
	}

	// Exact match
	return eventType == pattern
}

// ===============================
// DOMAIN EVENTS
// ===============================

// CommentReactionEvent represents a comment reaction event
type CommentReactionEvent struct {
	BaseEvent
	CommentID    int64  `json:"comment_id"`
	ReactionType string `json:"reaction_type"`
}

// CommentNotificationEvent represents a comment notification event
type CommentNotificationEvent struct {
	BaseEvent
	CommentID      int64   `json:"comment_id"`
	CommenterID    int64   `json:"commenter_id"`
	PostID         *int64  `json:"post_id,omitempty"`
	QuestionID     *int64  `json:"question_id,omitempty"`
	CommentPreview string  `json:"comment_preview"`
}

// UserMentionedEvent represents a user mention event
type UserMentionedEvent struct {
	BaseEvent
	MentionedByUserID int64  `json:"mentioned_by_user_id"`
	CommentID         int64  `json:"comment_id"`
	PostID            *int64 `json:"post_id,omitempty"`
	QuestionID        *int64 `json:"question_id,omitempty"`
}




// User Events
type UserCreatedEvent struct {
	BaseEvent
	UserID    int64     `json:"user_id"`
	Email     string    `json:"email"`
	Username  string    `json:"username"`
	CreatedAt time.Time `json:"created_at"`
}

type UserUpdatedEvent struct {
	BaseEvent
	UserID    int64     `json:"user_id"`
	UpdatedAt time.Time `json:"updated_at"`
	Changes   []string  `json:"changes"`
}

type UserDeactivatedEvent struct {
	BaseEvent
	UserID        int64     `json:"user_id"`
	Username      string    `json:"username"`
	Email         string    `json:"email"`
	Reason        string    `json:"reason"`
	DeactivatedAt time.Time `json:"deactivated_at"`
}

type UserOnlineStatusChangedEvent struct {
	BaseEvent
	UserID    int64     `json:"user_id"`
	Online    bool      `json:"online"`
	ChangedAt time.Time `json:"changed_at"`
}

// Content Report Events
type ContentReportedEvent struct {
	BaseEvent
	ContentType string    `json:"content_type"`
	ContentID   int64     `json:"content_id"`
	Reason      string    `json:"reason"`
	ReportedAt  time.Time `json:"reported_at"`
}

// Post Events
type PostCreatedEvent struct {
	BaseEvent
	PostID    int64     `json:"post_id"`
	Title     string    `json:"title"`
	Category  string    `json:"category"`
	CreatedAt time.Time `json:"created_at"`
}

type PostUpdatedEvent struct {
	BaseEvent
	PostID    int64     `json:"post_id"`
	UpdatedAt time.Time `json:"updated_at"`
	Changes   []string  `json:"changes"`
}

type PostDeletedEvent struct {
	BaseEvent
	PostID    int64     `json:"post_id"`
	DeletedAt time.Time `json:"deleted_at"`
}

type PostReactionEvent struct {
	BaseEvent
	PostID       int64     `json:"post_id"`
	ReactionType string    `json:"reaction_type"`
	ReactedAt    time.Time `json:"reacted_at"`
}

type PostSharedEvent struct {
	BaseEvent
	PostID   int64  `json:"post_id"`
	Platform string `json:"platform"` // e.g., "twitter", "facebook", "email", etc.
}

// PostViewedEvent is emitted when a post is viewed
type PostViewedEvent struct {
	BaseEvent
	PostID    int64     `json:"post_id"`
	ViewerID  *int64    `json:"viewer_id,omitempty"`
	ViewedAt  time.Time `json:"viewed_at"`
	IPAddress string    `json:"ip_address,omitempty"`
}

// Comment Events
type CommentCreatedEvent struct {
	BaseEvent
	CommentID  int64    `json:"comment_id"`
	PostID     *int64   `json:"post_id,omitempty"`
	QuestionID *int64   `json:"question_id,omitempty"`
	DocumentID *int64   `json:"document_id,omitempty"`
	Content    string   `json:"content"`
	Mentions   []string `json:"mentions,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

type CommentUpdatedEvent struct {
	BaseEvent
	CommentID int64     `json:"comment_id"`
	UpdatedAt time.Time `json:"updated_at"`
	Content   string   `json:"content"`
	Mentions  []string `json:"mentions,omitempty"`
}

type CommentDeletedEvent struct {
	BaseEvent
	CommentID int64     `json:"comment_id"`
	DeletedAt time.Time `json:"deleted_at"`
	
}

// Auth Events
type UserLoggedInEvent struct {
	BaseEvent
	LoginAt   time.Time `json:"login_at"`
	IPAddress string    `json:"ip_address"`
	UserAgent string    `json:"user_agent"`
}

type UserLoggedOutEvent struct {
	BaseEvent
	LogoutAt time.Time `json:"logout_at"`
}

type PasswordChangedEvent struct {
	BaseEvent
	ChangedAt time.Time `json:"changed_at"`
	IPAddress string    `json:"ip_address"`
}


// ContentModeratedEvent is emitted when content is moderated by an admin or moderator
type ContentModeratedEvent struct {
	BaseEvent
	ContentType string    `json:"content_type"` // e.g., "post", "comment"
	ContentID   int64     `json:"content_id"`   // ID of the moderated content
	Action      string    `json:"action"`       // e.g., "approved", "rejected", "deleted"
	Reason      string    `json:"reason"`       // Reason for moderation
	ModeratedAt time.Time `json:"moderated_at"` // When the moderation occurred
}

// ===============================
// FILE EVENTS
// ===============================

// FileUploadedEvent is emitted when a file is successfully uploaded
type FileUploadedEvent struct {
	BaseEvent
	FileType string `json:"file_type"`
	FileSize int64  `json:"file_size"`
	URL      string `json:"url"`
	PublicID string `json:"public_id"`
	Filename string `json:"filename,omitempty"`
}

// NewFileUploadedEvent creates a new file uploaded event
func NewFileUploadedEvent(fileType string, fileSize int64, url, publicID string, userID *int64) *FileUploadedEvent {
	return &FileUploadedEvent{
		BaseEvent: BaseEvent{
			EventID:   GenerateEventID(),
			EventType: "file.uploaded",
			Timestamp: time.Now(),
			UserID:    userID,
		},
		FileType: fileType,
		FileSize: fileSize,
		URL:      url,
		PublicID:  publicID,
	}
}

// ImageProcessedEvent is emitted when an image has been processed (resized, optimized, etc.)
type ImageProcessedEvent struct {
	BaseEvent
	PublicID string `json:"public_id"`
	Variants int    `json:"variants"`
}

// NewImageProcessedEvent creates a new image processed event
func NewImageProcessedEvent(publicID string, variants int, userID *int64) *ImageProcessedEvent {
	return &ImageProcessedEvent{
		BaseEvent: BaseEvent{
			EventID:   GenerateEventID(),
			EventType: "image.processed",
			Timestamp: time.Now(),
			UserID:    userID,
		},
		PublicID: publicID,
		Variants: variants,
	}
}

// ===============================
// EVENT FACTORY FUNCTIONS
// ===============================

// NewUserCreatedEvent creates a new user created event
func NewUserCreatedEvent(userID int64, email, username string) *UserCreatedEvent {
	return &UserCreatedEvent{
		BaseEvent: BaseEvent{
			EventID:   GenerateEventID(),
			EventType: "user.created",
			Timestamp: time.Now(),
			UserID:    &userID,
		},
		UserID:    userID,
		Email:     email,
		Username:  username,
		CreatedAt: time.Now(),
	}
}

// NewPostCreatedEvent creates a new post created event
func NewPostCreatedEvent(postID, userID int64, title, category string) *PostCreatedEvent {
	return &PostCreatedEvent{
		BaseEvent: BaseEvent{
			EventID:   GenerateEventID(),
			EventType: "post.created",
			Timestamp: time.Now(),
			UserID:    &userID,
		},
		PostID:    postID,
		Title:     title,
		Category:  category,
		CreatedAt: time.Now(),
	}
}

// NewPostSharedEvent creates a new post shared event
func NewPostSharedEvent(postID, userID int64, platform string) *PostSharedEvent {
	return &PostSharedEvent{
		BaseEvent: BaseEvent{
			EventID:   GenerateEventID(),
			EventType: "post.shared",
			Timestamp: time.Now(),
			UserID:    &userID,
		},
		PostID:   postID,
		Platform: platform,
	}
}

// NewContentReportedEvent creates a new content reported event
func NewContentReportedEvent(contentType string, contentID int64, reason string, reporterID *int64) *ContentReportedEvent {
	return &ContentReportedEvent{
		BaseEvent: BaseEvent{
			EventID:   GenerateEventID(),
			EventType: "content.reported",
			Timestamp: time.Now(),
			UserID:    reporterID,
		},
		ContentType: contentType,
		ContentID:   contentID,
		Reason:      reason,
		ReportedAt:  time.Now(),
	}
}

// NewContentModeratedEvent creates a new ContentModeratedEvent
func NewContentModeratedEvent(contentType string, contentID int64, action, reason string, moderatorID *int64) *ContentModeratedEvent {
	return &ContentModeratedEvent{
		BaseEvent: BaseEvent{
			EventID:   GenerateEventID(),
			EventType: "content.moderated",
			Timestamp: time.Now(),
			UserID:    moderatorID,
		},
		ContentType: contentType,
		ContentID:   contentID,
		Action:      action,
		Reason:      reason,
		ModeratedAt: time.Now(),
	}
}

// NewPostViewedEvent creates a new PostViewedEvent
func NewPostViewedEvent(postID int64, viewerID *int64, ipAddress string) *PostViewedEvent {
	return &PostViewedEvent{
		BaseEvent: BaseEvent{
			EventID:   GenerateEventID(),
			EventType: "post.viewed",
			Timestamp: time.Now(),
			UserID:    viewerID,
		},
		PostID:    postID,
		ViewerID:   viewerID,
		ViewedAt:   time.Now(),
		IPAddress:  ipAddress,
	}
}
// ===============================
// UTILITY FUNCTIONS
// ===============================

// generateEventID generates a unique event ID
func GenerateEventID() string {
	return fmt.Sprintf("evt_%d_%d", time.Now().UnixNano(), time.Now().Nanosecond())
}

// NewEventBus creates a new event bus instance
func NewEventBus(config *EventBusConfig, logger *zap.Logger) EventBus {
	return NewInMemoryEventBus(config, logger)
}

// ===============================
// EVENT HANDLER HELPERS
// ===============================

// NewEventHandlerFunc creates an EventHandler from a function
func NewEventHandlerFunc(id string, fn func(ctx context.Context, event Event) error) EventHandler {
	return EventHandlerFunc{
		ID:   id,
		Func: fn,
	}
}

// TypedEventHandler is a generic handler for specific event types
type TypedEventHandler[T Event] struct {
	ID      string
	Handler func(ctx context.Context, event T) error
}

// Handle implements EventHandler
func (h TypedEventHandler[T]) Handle(ctx context.Context, event Event) error {
	if typedEvent, ok := event.(T); ok {
		return h.Handler(ctx, typedEvent)
	}
	return fmt.Errorf("event type mismatch: expected %T, got %T", *new(T), event)
}

// GetHandlerID implements EventHandler
func (h TypedEventHandler[T]) GetHandlerID() string {
	return h.ID
}

// NewTypedEventHandler creates a typed event handler
func NewTypedEventHandler[T Event](id string, handler func(ctx context.Context, event T) error) EventHandler {
	return TypedEventHandler[T]{
		ID:      id,
		Handler: handler,
	}
}
