package services

import (
	"context"
	"evalhub/internal/events"
	"evalhub/internal/models"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
)

// eventService implements enterprise event-driven architecture
type eventService struct {
	bus                events.EventBus
	logger             *zap.Logger
	config             *EventConfig
	handlers           map[string][]EventHandler
	metrics            *EventMetrics
	processingQueue    chan *EventWrapper
	deadLetterQueue    chan *EventWrapper
	mu                 sync.RWMutex
	shutdown           chan struct{}
	wg                 sync.WaitGroup
}

// EventConfig holds event service configuration
type EventConfig struct {
	MaxRetries           int           `json:"max_retries"`
	RetryDelay           time.Duration `json:"retry_delay"`
	ProcessingTimeout    time.Duration `json:"processing_timeout"`
	QueueSize            int           `json:"queue_size"`
	WorkerCount          int           `json:"worker_count"`
	EnableDeadLetter     bool          `json:"enable_dead_letter"`
	EnableMetrics        bool          `json:"enable_metrics"`
	DeadLetterRetention  time.Duration `json:"dead_letter_retention"`
	BatchSize            int           `json:"batch_size"`
	FlushInterval        time.Duration `json:"flush_interval"`
}

// EventMetrics tracks event processing performance
type EventMetrics struct {
	EventsPublished    int64            `json:"events_published"`
	EventsProcessed    int64            `json:"events_processed"`
	EventsFailed       int64            `json:"events_failed"`
	EventsRetried      int64            `json:"events_retried"`
	AverageProcessTime time.Duration    `json:"average_process_time"`
	HandlerMetrics     map[string]int64 `json:"handler_metrics"`
	LastReset          time.Time        `json:"last_reset"`
	mu                 sync.RWMutex
}

// EventWrapper wraps events with metadata
type EventWrapper struct {
	Event       events.Event `json:"event"`
	AttemptCount int          `json:"attempt_count"`
	FirstAttempt time.Time    `json:"first_attempt"`
	LastAttempt  time.Time    `json:"last_attempt"`
	Error        error        `json:"error,omitempty"`
}

// NewEventService creates a new enterprise event service
func NewEventService(
	bus events.EventBus,
	logger *zap.Logger,
	config *EventConfig,
) EventService {
	if config == nil {
		config = DefaultEventConfig()
	}

	service := &eventService{
		bus:             bus,
		logger:          logger,
		config:          config,
		handlers:        make(map[string][]EventHandler),
		metrics:         &EventMetrics{
			HandlerMetrics: make(map[string]int64),
			LastReset:      time.Now(),
		},
		processingQueue: make(chan *EventWrapper, config.QueueSize),
		deadLetterQueue: make(chan *EventWrapper, config.QueueSize/10),
		shutdown:        make(chan struct{}),
	}

	// Start workers
	service.startWorkers()

	return service
}

// DefaultEventConfig returns default event configuration
func DefaultEventConfig() *EventConfig {
	return &EventConfig{
		MaxRetries:          3,
		RetryDelay:          5 * time.Second,
		ProcessingTimeout:   30 * time.Second,
		QueueSize:           1000,
		WorkerCount:         5,
		EnableDeadLetter:    true,
		EnableMetrics:       true,
		DeadLetterRetention: 24 * time.Hour,
		BatchSize:           10,
		FlushInterval:       5 * time.Second,
	}
}

// ===============================
// EVENT PUBLISHING
// ===============================

// PublishEvent publishes an event to the system
func (s *eventService) PublishEvent(ctx context.Context, event events.Event) error {
	if event == nil {
		return NewValidationError("event cannot be nil", nil)
	}

	// Validate event
	if err := s.validateEvent(event); err != nil {
		return NewValidationError("invalid event", err)
	}

	// Wrap event with metadata
	wrapper := &EventWrapper{
		Event:        event,
		AttemptCount: 0,
		FirstAttempt: time.Now(),
	}

	// Try to queue for processing
	select {
	case s.processingQueue <- wrapper:
		s.incrementMetric("events_published")
		s.logger.Debug("Event queued for processing",
			zap.String("event_type", event.GetEventType()),
			zap.String("event_id", event.GetEventID()),
		)
		return nil
	case <-ctx.Done():
		return ctx.Err()
	default:
		// Queue is full, try dead letter queue
		if s.config.EnableDeadLetter {
			select {
			case s.deadLetterQueue <- wrapper:
				s.logger.Warn("Event queued to dead letter (processing queue full)",
					zap.String("event_type", event.GetEventType()),
				)
				return nil
			default:
				return NewInternalError("all event queues are full")
			}
		}
		return NewInternalError("event processing queue is full")
	}
}

// PublishBatchEvents publishes multiple events efficiently
func (s *eventService) PublishBatchEvents(ctx context.Context, events []events.Event) error {
	if len(events) == 0 {
		return nil
	}

	// Validate all events first
	wrappers := make([]*EventWrapper, 0, len(events))
	for _, event := range events {
		if err := s.validateEvent(event); err != nil {
			s.logger.Warn("Skipping invalid event in batch",
				zap.Error(err),
				zap.String("event_type", event.GetEventType()),
			)
			continue
		}

		wrapper := &EventWrapper{
			Event:        event,
			AttemptCount: 0,
			FirstAttempt: time.Now(),
		}
		wrappers = append(wrappers, wrapper)
	}

	// Queue all valid events
	published := 0
BatchLoop:
	for _, wrapper := range wrappers {
		select {
		case s.processingQueue <- wrapper:
			published++
		case <-ctx.Done():
			break BatchLoop
		default:
			// Try dead letter queue
			if s.config.EnableDeadLetter {
				select {
				case s.deadLetterQueue <- wrapper:
					published++
				default:
					continue
				}
			}
		}
	}

	s.metrics.mu.Lock()
	s.metrics.EventsPublished += int64(published)
	s.metrics.mu.Unlock()

	s.logger.Info("Batch events published",
		zap.Int("total", len(events)),
		zap.Int("published", published),
		zap.Int("failed", len(events)-published),
	)

	return nil
}

// ===============================
// EVENT HANDLING
// ===============================

// GetEventHistory retrieves historical events of a specific type
func (s *eventService) GetEventHistory(ctx context.Context, eventType string, limit int) ([]events.Event, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// In a real implementation, this would query your event store/database
	// For now, we'll return an empty slice as a placeholder
	return []events.Event{}, nil
}

// RegisterHandler registers an event handler for specific event types
func (s *eventService) RegisterHandler(eventType string, handler EventHandler) error {
	if eventType == "" {
		return NewValidationError("event type cannot be empty", nil)
	}
	if handler == nil {
		return NewValidationError("handler cannot be nil", nil)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.handlers[eventType] == nil {
		s.handlers[eventType] = make([]EventHandler, 0)
	}

	s.handlers[eventType] = append(s.handlers[eventType], handler)

	s.logger.Info("Event handler registered",
		zap.String("event_type", eventType),
		zap.String("handler_id", handler.GetHandlerID()),
		zap.String("handler_type", fmt.Sprintf("%T", handler)),
		zap.Int("total_handlers", len(s.handlers[eventType])),
	)

	return nil
}

// UnregisterHandler removes an event handler by its ID
func (s *eventService) UnregisterHandler(eventType string, handlerID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	handlers, exists := s.handlers[eventType]
	if !exists {
		return NewNotFoundError("no handlers found for event type")
	}

	// Find and remove the specific handler by ID
	for i, h := range handlers {
		if h.GetHandlerID() == handlerID {
			s.handlers[eventType] = append(handlers[:i], handlers[i+1:]...)
			s.logger.Info("Event handler unregistered",
				zap.String("event_type", eventType),
				zap.String("handler_id", handlerID))

			// Remove the event type entry if no handlers left
			if len(s.handlers[eventType]) == 0 {
				delete(s.handlers, eventType)
			}

			return nil
		}
	}

	return NewNotFoundError("handler not found for event type")
}

// ===============================
// SPECIALIZED EVENT PUBLISHERS
// ===============================

func (s *eventService) PublishUserEvent(ctx context.Context, userID int64, eventType string, data map[string]interface{}) error {
    var event events.Event
    
    // Create the appropriate event type based on eventType
    switch eventType {
    case "user_created":
        // If you need to extract specific fields from data, you can do it here
        email, _ := data["email"].(string)
        username, _ := data["username"].(string)
        event = events.NewUserCreatedEvent(userID, email, username)
    case "user_updated":
        event = &events.UserUpdatedEvent{
            BaseEvent: events.BaseEvent{
                EventID:   events.GenerateEventID(),
                EventType: eventType,
                Timestamp: time.Now(),
                UserID:    &userID,
            },
            UpdatedAt: time.Now(),
        }
    // Add more cases as needed
    default:
        // For generic user events, you can use BaseEvent directly
        event = &events.BaseEvent{
            EventID:   events.GenerateEventID(),
            EventType: eventType,
            Timestamp: time.Now(),
            UserID:    &userID,
            Metadata:  data,
        }
    }
    
    return s.PublishEvent(ctx, event)
}

// PublishPostEvent publishes post-related events
func (s *eventService) PublishPostEvent(ctx context.Context, postID, userID int64, eventType string, data map[string]interface{}) error {
	// Extract title and category from data map with type assertion and default values
	title, _ := data["title"].(string)
	category, _ := data["category"].(string)

	event := events.NewPostCreatedEvent(postID, userID, title, category)

	return s.PublishEvent(ctx, event)
}

// PublishCommentEvent publishes comment-related events
func (s *eventService) PublishCommentEvent(ctx context.Context, commentID, userID int64, eventType string, data map[string]interface{}) error {
	// Extract common comment data
	postID, _ := data["post_id"].(int64)
	questionID, _ := data["question_id"].(int64)

	var event events.Event
	switch eventType {
	case "comment.created":
		event = &events.CommentCreatedEvent{
			BaseEvent: events.BaseEvent{
				EventID:   events.GenerateEventID(),
				EventType: eventType,
				Timestamp: time.Now(),
				UserID:    &userID,
				Metadata:  data,
			},
			CommentID:  commentID,
			PostID:     &postID,
			QuestionID: &questionID,
			CreatedAt:  time.Now(),
		}
	case "comment.updated":
		event = &events.CommentUpdatedEvent{
			BaseEvent: events.BaseEvent{
				EventID:   events.GenerateEventID(),
				EventType: eventType,
				Timestamp: time.Now(),
				UserID:    &userID,
				Metadata:  data,
			},
			CommentID: commentID,
			UpdatedAt: time.Now(),
		}
	case "comment.deleted":
		event = &events.CommentDeletedEvent{
			BaseEvent: events.BaseEvent{
				EventID:   events.GenerateEventID(),
				EventType: eventType,
				Timestamp: time.Now(),
				UserID:    &userID,
				Metadata:  data,
			},
			CommentID: commentID,
			DeletedAt: time.Now(),
		}
	default:
		// Fallback to a basic event for unknown types
		event = &events.BaseEvent{
			EventID:   events.GenerateEventID(),
			EventType: eventType,
			Timestamp: time.Now(),
			UserID:    &userID,
			Metadata:  data,
		}
	}

	return s.PublishEvent(ctx, event)
}

// PublishNotificationEvent publishes notification events
func (s *eventService) PublishNotificationEvent(ctx context.Context, userID int64, notificationType string, data map[string]interface{}) error {
	// Create a base event for notifications since there's no specific notification event type
	event := &events.BaseEvent{
		EventID:   events.GenerateEventID(),
		EventType: "notification." + notificationType,
		Timestamp: time.Now(),
		UserID:    &userID,
		Metadata:  data,
	}

	return s.PublishEvent(ctx, event)
}

// ===============================
// DOMAIN-SPECIFIC PUBLISHERS
// ===============================

// PublishUserRegistered publishes user registration event
func (s *eventService) PublishUserRegistered(ctx context.Context, user *models.User) error {
	return s.PublishUserEvent(ctx, user.ID, "user.registered", map[string]interface{}{
		"username": user.Username,
		"email":    user.Email,
	})
}

// PublishPostCreated publishes post creation event
func (s *eventService) PublishPostCreated(ctx context.Context, post *models.Post) error {
	return s.PublishPostEvent(ctx, int64(post.ID), int64(post.UserID), "post.created", map[string]interface{}{
		"title":    post.Title,
		"category": post.Category,
	})
}

// PublishPostLiked publishes post like event
func (s *eventService) PublishPostLiked(ctx context.Context, postID, userID int64) error {
	return s.PublishPostEvent(ctx, postID, userID, "post.liked", map[string]interface{}{
		"post_id": postID,
	})
}

// PublishCommentCreated publishes comment creation event
func (s *eventService) PublishCommentCreated(ctx context.Context, comment *models.Comment) error {
	return s.PublishCommentEvent(ctx, comment.ID, comment.UserID, "comment.created", map[string]interface{}{
		"post_id": comment.PostID,
		"content": s.truncateContent(comment.Content, 100),
	})
}

// ===============================
// EVENT PROCESSING
// ===============================

// startWorkers starts background workers to process events
func (s *eventService) startWorkers() {
	// Start processing workers
	for i := 0; i < s.config.WorkerCount; i++ {
		s.wg.Add(1)
		go s.processWorker(i)
	}

	// Start dead letter processor
	if s.config.EnableDeadLetter {
		s.wg.Add(1)
		go s.deadLetterWorker()
	}

	// Start metrics collector
	if s.config.EnableMetrics {
		s.wg.Add(1)
		go s.metricsWorker()
	}
}

// processWorker processes events from the queue
func (s *eventService) processWorker(workerID int) {
	defer s.wg.Done()

	s.logger.Info("Event processing worker started",
		zap.Int("worker_id", workerID),
	)

	for {
		select {
		case wrapper := <-s.processingQueue:
			s.processEvent(wrapper)
		case <-s.shutdown:
			s.logger.Info("Event processing worker shutting down",
				zap.Int("worker_id", workerID),
			)
			return
		}
	}
}

// processEvent processes a single event
func (s *eventService) processEvent(wrapper *EventWrapper) {
	start := time.Now()
	wrapper.AttemptCount++
	wrapper.LastAttempt = time.Now()

	ctx, cancel := context.WithTimeout(context.Background(), s.config.ProcessingTimeout)
	defer cancel()

	eventType := wrapper.Event.GetEventType()

	// Get handlers for this event type
	s.mu.RLock()
	handlers := make([]EventHandler, len(s.handlers[eventType]))
	copy(handlers, s.handlers[eventType])
	s.mu.RUnlock()

	if len(handlers) == 0 {
		s.logger.Debug("No handlers registered for event type",
			zap.String("event_type", eventType),
		)
		s.incrementMetric("events_processed")
		return
	}

	// Process with each handler
	var lastError error
	successCount := 0
	for _, handler := range handlers {
		handlerID := handler.GetHandlerID()
		s.logger.Debug("Processing event with handler",
			zap.String("event_type", eventType),
			zap.String("event_id", wrapper.Event.GetEventID()),
			zap.String("handler_id", handlerID),
		)

		err := handler.Handle(ctx, wrapper.Event)
		if err != nil {
			lastError = err
			s.logger.Error("Event handler failed",
				zap.String("event_type", eventType),
				zap.String("event_id", wrapper.Event.GetEventID()),
				zap.String("handler_id", handlerID),
				zap.Error(err),
			)
			s.incrementHandlerMetric(eventType, "failed")
		} else {
			successCount++
			s.incrementHandlerMetric(eventType, "success")
		}
	}

	duration := time.Since(start)

	if lastError != nil && successCount == 0 {
		// All handlers failed
		s.handleEventFailure(wrapper, lastError)
	} else {
		// At least one handler succeeded
		s.incrementMetric("events_processed")
		s.updateProcessingTime(duration)
		s.logger.Debug("Event processed successfully",
			zap.String("event_type", eventType),
			zap.String("event_id", wrapper.Event.GetEventID()),
			zap.Int("successful_handlers", successCount),
			zap.Int("total_handlers", len(handlers)),
		)
	}
}

// safeHandleEvent safely calls an event handler with recovery
func (s *eventService) safeHandleEvent(ctx context.Context, handler EventHandler, event events.Event) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("handler panicked: %v", r)
			s.logger.Error("Event handler panicked",
				zap.String("event_type", event.GetEventType()),
				zap.Any("panic", r),
			)
		}
	}()

	return handler.Handle(ctx, event)
}

// handleEventFailure handles failed event processing
func (s *eventService) handleEventFailure(wrapper *EventWrapper, err error) {
	wrapper.Error = err

	if wrapper.AttemptCount < s.config.MaxRetries {
		// Retry after delay
		s.incrementMetric("events_retried")
		go func() {
			time.Sleep(s.config.RetryDelay * time.Duration(wrapper.AttemptCount))
			select {
			case s.processingQueue <- wrapper:
				s.logger.Debug("Event requeued for retry",
					zap.String("event_type", wrapper.Event.GetEventType()),
					zap.Int("attempt", wrapper.AttemptCount),
				)
			case s.deadLetterQueue <- wrapper:
				s.logger.Warn("Event moved to dead letter (retry queue full)",
					zap.String("event_type", wrapper.Event.GetEventType()),
				)
			default:
				s.logger.Error("Failed to requeue event (all queues full)",
					zap.String("event_type", wrapper.Event.GetEventType()),
				)
			}
		}()
	} else {
		// Max retries exceeded, move to dead letter
		s.incrementMetric("events_failed")
		if s.config.EnableDeadLetter {
			select {
			case s.deadLetterQueue <- wrapper:
				s.logger.Error("Event moved to dead letter queue (max retries exceeded)",
					zap.String("event_type", wrapper.Event.GetEventType()),
					zap.Int("attempts", wrapper.AttemptCount),
					zap.Error(err),
				)
			default:
				s.logger.Error("Failed to move event to dead letter queue (queue full)",
					zap.String("event_type", wrapper.Event.GetEventType()),
				)
			}
		}
	}
}

// deadLetterWorker processes dead letter queue
func (s *eventService) deadLetterWorker() {
	defer s.wg.Done()

	ticker := time.NewTicker(s.config.FlushInterval)
	defer ticker.Stop()

	deadEvents := make([]*EventWrapper, 0, s.config.BatchSize)

	for {
		select {
		case wrapper := <-s.deadLetterQueue:
			deadEvents = append(deadEvents, wrapper)
			if len(deadEvents) >= s.config.BatchSize {
				s.processDeadLetterBatch(deadEvents)
				deadEvents = deadEvents[:0]
			}

		case <-ticker.C:
			if len(deadEvents) > 0 {
				s.processDeadLetterBatch(deadEvents)
				deadEvents = deadEvents[:0]
			}

		case <-s.shutdown:
			// Process remaining events
			if len(deadEvents) > 0 {
				s.processDeadLetterBatch(deadEvents)
			}
			return
		}
	}
}

// processDeadLetterBatch processes a batch of dead letter events
func (s *eventService) processDeadLetterBatch(events []*EventWrapper) {
	s.logger.Info("Processing dead letter batch",
		zap.Int("count", len(events)),
	)

	// Here you would typically:
	// 1. Store events to persistent storage for investigation
	// 2. Send alerts to monitoring systems
	// 3. Create administrative reports

	for _, wrapper := range events {
		s.logger.Error("Dead letter event",
			zap.String("event_type", wrapper.Event.GetEventType()),
			zap.String("event_id", wrapper.Event.GetEventID()),
			zap.Int("attempts", wrapper.AttemptCount),
			zap.Duration("total_time", wrapper.LastAttempt.Sub(wrapper.FirstAttempt)),
			zap.Error(wrapper.Error),
		)
	}
}

// ===============================
// METRICS AND MONITORING
// ===============================

// GetMetrics returns comprehensive metrics about the event service's performance and state
func (s *eventService) GetMetrics() *EventServiceMetrics {
	s.metrics.mu.RLock()
	defer s.metrics.mu.RUnlock()

	// Calculate uptime and rates
	uptime := time.Since(s.metrics.LastReset)
	publishRate := float64(0)
	processRate := float64(0)

	// Avoid division by zero for rates
	if uptime > 0 {
		publishRate = float64(s.metrics.EventsPublished) / uptime.Seconds()
		processRate = float64(s.metrics.EventsProcessed) / uptime.Seconds()
	}

	// Safely copy handler metrics to prevent concurrent map access
	handlerMetrics := make(map[string]int64, len(s.metrics.HandlerMetrics))
	for k, v := range s.metrics.HandlerMetrics {
		handlerMetrics[k] = v
	}

	return &EventServiceMetrics{
		// Basic event counters
		EventsPublished: s.metrics.EventsPublished,
		EventsProcessed: s.metrics.EventsProcessed,
		EventsFailed:    s.metrics.EventsFailed,
		EventsRetried:   s.metrics.EventsRetried,

		// Performance metrics
		AverageProcessTime: s.metrics.AverageProcessTime,
		PublishRate:       publishRate,
		ProcessRate:       processRate,

		// Queue metrics
		QueueDepth:      len(s.processingQueue),
		DeadLetterDepth: len(s.deadLetterQueue),

		// Additional metrics
		HandlerMetrics: handlerMetrics,
		Uptime:         uptime,
	}
}

// metricsWorker collects and reports metrics
func (s *eventService) metricsWorker() {
	defer s.wg.Done()

	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			metrics := s.GetMetrics()
			s.logger.Info("Event service metrics",
				zap.Int64("published", metrics.EventsPublished),
				zap.Int64("processed", metrics.EventsProcessed),
				zap.Int64("failed", metrics.EventsFailed),
				zap.Float64("publish_rate", metrics.PublishRate),
				zap.Int("queue_depth", metrics.QueueDepth),
			)

		case <-s.shutdown:
			return
		}
	}
}

// ===============================
// HELPER METHODS
// ===============================

// validateEvent validates event structure
func (s *eventService) validateEvent(event events.Event) error {
	if event.GetEventID() == "" {
		return fmt.Errorf("event ID is required")
	}
	if event.GetEventType() == "" {
		return fmt.Errorf("event type is required")
	}
	if event.GetTimestamp().IsZero() {
		return fmt.Errorf("event timestamp is required")
	}
	return nil
}

// incrementMetric safely increments a metric
func (s *eventService) incrementMetric(metric string) {
	s.metrics.mu.Lock()
	defer s.metrics.mu.Unlock()

	switch metric {
	case "events_published":
		s.metrics.EventsPublished++
	case "events_processed":
		s.metrics.EventsProcessed++
	case "events_failed":
		s.metrics.EventsFailed++
	case "events_retried":
		s.metrics.EventsRetried++
	}
}

// incrementHandlerMetric tracks handler-specific metrics
func (s *eventService) incrementHandlerMetric(eventType, result string) {
	s.metrics.mu.Lock()
	defer s.metrics.mu.Unlock()

	key := fmt.Sprintf("%s.%s", eventType, result)
	s.metrics.HandlerMetrics[key]++
}

// updateProcessingTime updates average processing time
func (s *eventService) updateProcessingTime(duration time.Duration) {
	s.metrics.mu.Lock()
	defer s.metrics.mu.Unlock()

	// Simple moving average
	if s.metrics.AverageProcessTime == 0 {
		s.metrics.AverageProcessTime = duration
	} else {
		s.metrics.AverageProcessTime = (s.metrics.AverageProcessTime + duration) / 2
	}
}

// truncateContent safely truncates content for logging
func (s *eventService) truncateContent(content string, maxLen int) string {
	if len(content) <= maxLen {
		return content
	}
	return content[:maxLen] + "..."
}

// Shutdown gracefully stops the event service
func (s *eventService) Shutdown(ctx context.Context) error {
	s.logger.Info("Shutting down event service")

	close(s.shutdown)

	// Wait for workers to finish with timeout
	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		s.logger.Info("Event service shutdown completed")
		return nil
	case <-ctx.Done():
		s.logger.Warn("Event service shutdown timed out")
		return ctx.Err()
	}
}