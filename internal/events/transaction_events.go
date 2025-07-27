package events

import "time"

// TransactionStartedEvent is emitted when a new transaction is started
type TransactionStartedEvent struct {
	BaseEvent
	TransactionID string `json:"transaction_id"`
}

// NewTransactionStartedEvent creates a new transaction started event
func NewTransactionStartedEvent(transactionID string, userID *int64) *TransactionStartedEvent {
	return &TransactionStartedEvent{
		BaseEvent: BaseEvent{
			EventID:   GenerateEventID(),
			EventType: "transaction.started",
			Timestamp: time.Now(),
			UserID:    userID,
		},
		TransactionID: transactionID,
	}
}


// TransactionCommittedEvent is emitted when a transaction is successfully committed
type TransactionCommittedEvent struct {
	BaseEvent
	TransactionID  string        `json:"transaction_id"`
	Duration      time.Duration `json:"duration"`
	OperationCount int           `json:"operation_count"`
}

// NewTransactionCommittedEvent creates a new transaction committed event
func NewTransactionCommittedEvent(transactionID string, duration time.Duration, operationCount int, userID *int64) *TransactionCommittedEvent {
	return &TransactionCommittedEvent{
		BaseEvent: BaseEvent{
			EventID:   GenerateEventID(),
			EventType: "transaction.committed",
			Timestamp: time.Now(),
			UserID:    userID,
		},
		TransactionID:  transactionID,
		Duration:      duration,
		OperationCount: operationCount,
	}
}

// TransactionRolledBackEvent is emitted when a transaction is rolled back
type TransactionRolledBackEvent struct {
	BaseEvent
	TransactionID  string        `json:"transaction_id"`
	Duration      time.Duration `json:"duration"`
	OperationCount int           `json:"operation_count"`
}

// NewTransactionRolledBackEvent creates a new transaction rolled back event
func NewTransactionRolledBackEvent(transactionID string, duration time.Duration, operationCount int, userID *int64) *TransactionRolledBackEvent {
	return &TransactionRolledBackEvent{
		BaseEvent: BaseEvent{
			EventID:   GenerateEventID(),
			EventType: "transaction.rolled_back",
			Timestamp: time.Now(),
			UserID:    userID,
		},
		TransactionID:  transactionID,
		Duration:      duration,
		OperationCount: operationCount,
	}
}
