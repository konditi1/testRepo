// file: internal/handlers/web/handlers_test,go
package web

import (
	"context"
	"evalhub/internal/config"
	"evalhub/internal/database"
	"net/http"
	"net/http/httptest"
	"testing"
	"text/template"
	"time"

	"go.uber.org/zap"
)

func setupTestDB(t *testing.T) {
	// Create a test database configuration
	cfg := &config.DatabaseConfig{
		URL:                "postgres://postgres:postgres@localhost:5432/evalhub_test?sslmode=disable",
		MaxOpenConns:       10,
		MaxIdleConns:       5,
		ConnMaxLifetime:    time.Hour,
		ConnMaxIdleTime:    30 * time.Minute,
		SlowQueryThreshold: 500 * time.Millisecond,
	}

	// Initialize the database manager
	logger, _ := zap.NewDevelopment()
	dbManager, err := database.NewManager(cfg, logger)
	if err != nil {
		t.Fatalf("Failed to create database manager: %v", err)
	}

	// Execute a test query to verify the database is working
	_, err = database.DB.DB().ExecContext(context.Background(), "SELECT 1")
	if err != nil {
		t.Fatalf("Failed to execute test query: %v", err)
	}

	// Set the global DB instance
	database.DB = dbManager

	// Initialize templates
	templates = template.New("test")
	templates.New("login").Parse(`{{define "login"}}Login Template{{end}}`)
	templates.New("signup").Parse(`{{define "signup"}}Signup Template{{end}}`)
	templates.New("dashboard").Parse(`{{define "dashboard"}}
		{{range .Posts}}
			<div>{{.Title}} - {{.Content}} - {{.Username}}</div>
		{{end}}
	{{end}}`)
	templates.New("post-list").Parse(`{{define "post-list"}}Post List Template{{end}}`)
	templates.New("create-post").Parse(`{{define "create-post"}}Create Post Template{{end}}`)
	templates.New("edit-post").Parse(`{{define "edit-post"}}Edit Post Template{{end}}`)
	templates.New("400").Parse(`{{define "400"}}400 Error{{end}}`)
	templates.New("401").Parse(`{{define "401"}}401 Error{{end}}`)
	templates.New("403").Parse(`{{define "403"}}403 Error{{end}}`)
	templates.New("404").Parse(`{{define "404"}}404 Error{{end}}`)
	templates.New("405").Parse(`{{define "405"}}405 Error{{end}}`)
	templates.New("500").Parse(`{{define "500"}}500 Error{{end}}`)

	// Create tables with complete schema
	_, err = database.DB.DB().ExecContext(context.Background(), `
		CREATE TABLE users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			email TEXT UNIQUE NOT NULL,
			username TEXT UNIQUE NOT NULL,
			password TEXT NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);

		CREATE TABLE sessions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL,
			session_token TEXT UNIQUE NOT NULL,
			expires_at DATETIME NOT NULL,
			last_activity DATETIME DEFAULT CURRENT_TIMESTAMP,
			user_role TEXT DEFAULT 'user',
			FOREIGN KEY (user_id) REFERENCES users (id)
		);

		CREATE TABLE posts (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL,
			title TEXT NOT NULL,
			content TEXT NOT NULL,
			category TEXT NOT NULL,
			image_url TEXT DEFAULT '',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (user_id) REFERENCES users (id)
		);

		CREATE TABLE comments (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			post_id INTEGER NOT NULL,
			user_id INTEGER NOT NULL,
			username TEXT NOT NULL,
			content TEXT NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (post_id) REFERENCES posts(id),
			FOREIGN KEY (user_id) REFERENCES users(id)
		);

		CREATE TABLE post_reactions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL,
			post_id INTEGER NOT NULL,
			reaction TEXT CHECK(reaction IN ('like', 'dislike')) NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (user_id) REFERENCES users (id),
			FOREIGN KEY (post_id) REFERENCES posts (id),
			UNIQUE(user_id, post_id)
		);

		CREATE TABLE categories (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT UNIQUE NOT NULL
		);

		CREATE TABLE comment_reactions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			comment_id INTEGER NOT NULL,
			user_id INTEGER NOT NULL,
			reaction TEXT CHECK(reaction IN ('like', 'dislike')) NOT NULL,
			FOREIGN KEY (comment_id) REFERENCES comments(id),
			FOREIGN KEY (user_id) REFERENCES users(id),
			UNIQUE(user_id, comment_id)
		);

		CREATE TABLE reactions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL,
			post_id INTEGER NOT NULL,
			reaction_type TEXT NOT NULL,
			FOREIGN KEY (user_id) REFERENCES users (id),
			FOREIGN KEY (post_id) REFERENCES posts (id)
		);

		-- Insert test user (only once)
		INSERT INTO users (id, email, username, password) 
		VALUES (1, 'test@example.com', 'testuser', 'password');
	`)
	if err != nil {
		t.Fatalf("Failed to create tables: %v", err)
	}
}

func TestHomeHandler(t *testing.T) {
	setupTestDB(t)
	defer database.DB.Close()

	tests := []struct {
		name           string
		path           string
		authenticated  bool
		expectedStatus int
	}{
		{
			name:           "Valid Home Page Request",
			path:           "/",
			authenticated:  false,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Invalid Path",
			path:           "/invalid",
			authenticated:  false,
			expectedStatus: http.StatusNotFound,
		},
		{
			name:           "Authenticated User",
			path:           "/",
			authenticated:  true,
			expectedStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", tt.path, nil)

			if tt.authenticated {
				ctx := context.WithValue(req.Context(), userIDKey, 1)
				req = req.WithContext(ctx)
			}

			w := httptest.NewRecorder()
			HomeHandler(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("HomeHandler() status = %v, want %v", w.Code, tt.expectedStatus)
			}
		})
	}
}

func TestDashboardHandler(t *testing.T) {
	setupTestDB(t)
	defer database.DB.Close()

	tests := []struct {
		name           string
		userID         int
		expectedStatus int
	}{
		{
			name:           "Valid Dashboard Request",
			userID:         1,
			expectedStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/dashboard", nil)
			ctx := context.WithValue(req.Context(), userIDKey, tt.userID)
			req = req.WithContext(ctx)

			w := httptest.NewRecorder()
			DashboardHandler(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("DashboardHandler() status = %v, want %v", w.Code, tt.expectedStatus)
			}
		})
	}
}

func TestListPostsHandler(t *testing.T) {
	setupTestDB(t)
	defer database.DB.Close()

	tests := []struct {
		name           string
		expectedStatus int
	}{
		{
			name:           "List All Posts",
			expectedStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/posts", nil)
			w := httptest.NewRecorder()
			ListPostsHandler(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("ListPostsHandler() status = %v, want %v", w.Code, tt.expectedStatus)
			}
		})
	}
}

func TestInitTemplates(t *testing.T) {
	tests := []struct {
		name        string
		baseDir     string
		expectError bool
	}{
		{
			name:        "Valid Templates Directory",
			baseDir:     "..",
			expectError: false,
		},
		{
			name:        "Invalid Templates Directory",
			baseDir:     "/nonexistent",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := InitTemplates(tt.baseDir)
			if (err != nil) != tt.expectError {
				t.Errorf("InitTemplates() error = %v, expectError %v", err, tt.expectError)
			}
		})
	}
}

func TestMain(m *testing.M) {
	// Run tests
	m.Run()
}
