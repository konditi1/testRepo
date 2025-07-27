package web

import (
	"context"
	"evalhub/internal/database"
	"fmt"
	"net/http"
	"strings"
)

// AuthMiddleware ensures that only logged-in users can access the route.
func AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("session_token")
		if err != nil || cookie.Value == "" {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		var userID int
		query := `SELECT user_id FROM sessions WHERE session_token = $1`
		err = database.DB.QueryRowContext(context.Background(), query, cookie.Value).Scan(&userID)
		if err != nil {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		ctx := context.WithValue(r.Context(), userIDKey, userID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// OwnershipMiddleware ensures that the logged-in user owns the requested resource.
func OwnershipMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		val := r.Context().Value(userIDKey)
		userID, ok := val.(int)
		if !ok {
			RenderErrorPage(w, http.StatusUnauthorized, fmt.Errorf("user ID not found in context"))
			return
		}

		resourceID := r.URL.Query().Get("id")
		isOwner, err := VerifyResourceOwnership(userID, resourceID, r.URL.Path)
		if err != nil || !isOwner {
			RenderErrorPage(w, http.StatusForbidden, fmt.Errorf("unauthorized access: %v", err))
			return
		}

		next.ServeHTTP(w, r)
	})
}

// GuestMiddleware skips login-required pages if user is already logged in.
func GuestMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("session_token")
		if err != nil || cookie.Value == "" {
			// No session token, proceed as guest
			next.ServeHTTP(w, r)
			return
		}

		var userID int
		query := `SELECT user_id FROM sessions WHERE session_token = $1`
		err = database.DB.QueryRowContext(r.Context(), query, cookie.Value).Scan(&userID)
		if err != nil {
			// Invalid session token, proceed as guest
			next.ServeHTTP(w, r)
			return
		}

		// User is already logged in, redirect them away from guest-only pages
		http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
	})
}

func AdminMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID := r.Context().Value(userIDKey)
		if userID == nil {
			RenderErrorPage(w, http.StatusUnauthorized, fmt.Errorf("authentication required"))
			return
		}

		user, err := getUserByID(userID.(int))
		if err != nil || user.Role != "admin" {
			RenderErrorPage(w, http.StatusForbidden, fmt.Errorf("admin access required"))
			return
		}

		next.ServeHTTP(w, r)
	})
}

func VerifyResourceOwnership(userID int, resourceID string, resourceType string) (bool, error) {
	var ownerID int
	var query string

	switch {
	case strings.Contains(resourceType, "post"):
		query = "SELECT user_id FROM posts WHERE id = $1"
	case strings.Contains(resourceType, "comment"):
		query = "SELECT user_id FROM comments WHERE id = $1"
	default:
		return false, fmt.Errorf("invalid resource type")
	}

	err := database.DB.QueryRowContext(context.Background(), query, resourceID).Scan(&ownerID)
	if err != nil {
		return false, err
	}

	return ownerID == userID, nil
}
