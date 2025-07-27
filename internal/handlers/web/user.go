// file: internal/handlers/web/user.go
package web

import (
	"context"
	"evalhub/internal/models"
	"evalhub/internal/services"
)

var userService services.UserService // TODO: Wire up in main or router

func GetUsernameByID(userID int) (string, error) {
	ctx := context.Background()
	user, err := userService.GetUserByID(ctx, int64(userID))
	if err != nil {
		return "", err
	}
	return user.Username, nil
}

func GetUserByGitHubID(githubID int) (*models.User, error) {
	ctx := context.Background()
	return userService.GetUserByGitHubID(ctx, int64(githubID))
}

// getUsername retrieves the username for a given user ID
func getUsername(userID int) string {
	ctx := context.Background()
	user, err := userService.GetUserByID(ctx, int64(userID))
	if err != nil {
		return ""
	}
	return user.Username
}

func GetAllUsers(currentUserID int) ([]models.User, error) {
	ctx := context.Background()
	// Use ListUsers with default pagination parameters
	resp, err := userService.ListUsers(ctx, &services.ListUsersRequest{
		Pagination: models.PaginationParams{
			Limit:  100, // Number of items per page
			Offset: 0,   // Start from the first item
		},
	})
	if err != nil {
		return nil, err
	}
	// Convert from []*models.User to []models.User
	var users []models.User
	for _, u := range resp.Data {
		users = append(users, *u)
	}
	return users, nil
}
