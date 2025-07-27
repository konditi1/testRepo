// file: internal/handlers/web/websocket.go
package web

import (
	"context"
	"evalhub/internal/database"
	"evalhub/internal/models"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // Adjust for production
	},
}

type Client struct {
	conn   *websocket.Conn
	userID int
	send   chan models.Message
}

var clients = make(map[int]*Client)
var clientsMu sync.Mutex

func WebSocketHandler(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(userIDKey).(int)
	if !ok {
		log.Printf("WebSocket: Unauthorized access attempt")
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade error: %v", err)
		return
	}
	// Set user online and update last_seen
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	_, err = database.DB.ExecContext(ctx, "UPDATE users SET is_online = TRUE, last_seen = CURRENT_TIMESTAMP WHERE id = $1", userID)
	if err != nil {
		log.Printf("Failed to set user online: %v", err)
	}
	client := &Client{
		conn:   conn,
		userID: userID,
		send:   make(chan models.Message),
	}
	clientsMu.Lock()
	clients[userID] = client
	clientsMu.Unlock()
	log.Printf("WebSocket: New client connected, userID=%d", userID)

	// Broadcast user online status to other clients
	broadcastUserStatus(userID, true)

	go client.writeMessages()
	client.readMessages()
}

func (c *Client) readMessages() {
	defer func() {
		clientsMu.Lock()
		delete(clients, c.userID)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		_, err := database.DB.ExecContext(ctx, "UPDATE users SET is_online = FALSE, last_seen = CURRENT_TIMESTAMP WHERE id = $1", c.userID)
		if err != nil {
			log.Printf("Failed to set user offline: %v", err)
		}
		clientsMu.Unlock()
		c.conn.Close()

		// Broadcast user offline status to other clients
		broadcastUserStatus(c.userID, false)

		log.Printf("WebSocket: Client disconnected, userID=%d", c.userID)
	}()
	for {
		var msg models.Message
		err := c.conn.ReadJSON(&msg)
		if err != nil {
			log.Printf("WebSocket: Error reading message from userID=%d: %v", c.userID, err)
			break
		}

		// Check if this is a special message type
		if msg.MessageType == "typing" {
			// Handle typing indicator
			clientsMu.Lock()
			if recipient, ok := clients[int(msg.RecipientID)]; ok {
				recipient.conn.WriteJSON(msg)
			}
			clientsMu.Unlock()
			continue
		}

		if msg.RecipientID == 0 || msg.Content == "" {
			log.Printf("WebSocket: Invalid message from userID=%d: recipient_id=%d, content='%s'", c.userID, msg.RecipientID, msg.Content)
			continue
		}
		msg.SenderID = int64(c.userID)
		msg.CreatedAt = time.Now()

		// Update last seen time when sending a message
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		_, err = database.DB.ExecContext(ctx, "UPDATE users SET last_seen = CURRENT_TIMESTAMP WHERE id = $1", c.userID)
		if err != nil {
			log.Printf("Failed to update last_seen: %v", err)
		}

		err = SaveMessage(&msg)
		if err != nil {
			log.Printf("WebSocket: Error saving message from userID=%d: %v", c.userID, err)
			continue
		}
		clientsMu.Lock()
		if sender, ok := clients[int(msg.SenderID)]; ok {
			sender.send <- msg
		}
		if recipient, ok := clients[int(msg.RecipientID)]; ok {
			recipient.send <- msg
		}
		clientsMu.Unlock()
	}
}

func (c *Client) writeMessages() {
	defer c.conn.Close()
	for msg := range c.send {
		err := c.conn.WriteJSON(msg)
		if err != nil {
			log.Printf("WebSocket: Error writing message to userID=%d: %v", c.userID, err)
			return
		}
	}
}

// Handle typing indicators
func handleTypingIndicator(msg models.Message) {
	clientsMu.Lock()
	defer clientsMu.Unlock()
	if recipient, ok := clients[int(msg.RecipientID)]; ok {
		recipient.conn.WriteJSON(msg)
	}
}

// New function to broadcast user status changes
func broadcastUserStatus(userID int, isOnline bool) {
	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Get user details
	var username string
	err := database.DB.QueryRowContext(ctx, "SELECT username FROM users WHERE id = $1", userID).Scan(&username)
	if err != nil {
		log.Printf("Failed to get username: %v", err)
		return
	}

	// Get the actual last_seen time from the database instead of using time.Now()
	var lastSeen time.Time
	err = database.DB.QueryRowContext(ctx, "SELECT last_seen FROM users WHERE id = $1", userID).Scan(&lastSeen)
	if err != nil {
		log.Printf("Failed to get last_seen: %v", err)
		// Fall back to current time if query fails
		lastSeen = time.Now()
	}

	// Create status update message
	statusMsg := struct {
		Type     string `json:"type"`
		UserID   int    `json:"user_id"`
		Username string `json:"username"`
		IsOnline bool   `json:"is_online"`
		LastSeen string `json:"last_seen"`
	}{
		Type:     "status_update",
		UserID:   userID,
		Username: username,
		IsOnline: isOnline,
		LastSeen: lastSeen.Format(time.RFC3339), // This preserves timezone info
	}

	// Broadcast to all clients
	clientsMu.Lock()
	defer clientsMu.Unlock()

	for _, client := range clients {
		// Don't send status updates to the user themselves
		if client.userID != userID {
			err := client.conn.WriteJSON(statusMsg)
			if err != nil {
				log.Printf("Failed to send status update: %v", err)
			}
		}
	}
}

func SaveMessage(msg *models.Message) error {
	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Ensure the timestamp is in UTC for storage
	if msg.CreatedAt.IsZero() {
		msg.CreatedAt = time.Now().UTC()
	} else {
		msg.CreatedAt = msg.CreatedAt.UTC()
	}
	query := `INSERT INTO messages (sender_id, recipient_id, content, created_at) VALUES ($1, $2, $3, $4) RETURNING id`
	err := database.DB.QueryRowContext(ctx, query, msg.SenderID, msg.RecipientID, msg.Content, msg.CreatedAt).Scan(&msg.ID)
	if err != nil {
		log.Printf("WebSocket: Failed to save message: sender_id=%d, recipient_id=%d, content='%s', error=%v", msg.SenderID, msg.RecipientID, msg.Content, err)
		return err
	}

	// Add notification for chat message
	go NotifyChatMessage(int(msg.SenderID), int(msg.RecipientID), msg.Content)

	return nil
}
