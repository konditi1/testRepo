// Package docs contains comprehensive Swagger documentation
package docs

// This file contains all Swagger endpoint documentation
// Import this in your main.go with: _ "evalhub/docs"

// HealthCheck godoc
// @Summary Health check endpoint
// @Description Returns the health status of the API
// @Tags System
// @Produce json
// @Success 200 {object} HealthCheckResponse "API is healthy"
// @Router /health [get]
func _() {}

// GetAPIInfo godoc
// @Summary Get API information
// @Description Returns comprehensive API information and available endpoints
// @Tags System
// @Produce json
// @Success 200 {object} map[string]interface{} "API information"
// @Router /info [get]  
func _() {}

// Register godoc
// @Summary Register a new user
// @Description Register a new user with the provided information
// @Tags Authentication
// @Accept json
// @Produce json
// @Param registerRequest body RegisterRequest true "Registration details"
// @Success 201 {object} AuthResponse "User registered successfully"
// @Failure 400 {object} ErrorResponse "Invalid request format or validation error"
// @Failure 409 {object} ErrorResponse "User with email or username already exists"
// @Failure 500 {object} ErrorResponse "Internal server error"
// @Router /auth/register [post]
func _() {}

// Login godoc
// @Summary Authenticate a user
// @Description Authenticate a user with email/username and password
// @Tags Authentication
// @Accept json
// @Produce json
// @Param loginRequest body LoginRequest true "Login credentials"
// @Success 200 {object} AuthResponse "Authentication successful"
// @Failure 400 {object} ErrorResponse "Invalid request format"
// @Failure 401 {object} ErrorResponse "Invalid credentials"
// @Failure 403 {object} ErrorResponse "Account locked or not verified"
// @Failure 429 {object} ErrorResponse "Too many login attempts"
// @Router /auth/login [post]
func _() {}

// RefreshToken godoc
// @Summary Refresh authentication token
// @Description Get a new access token using a refresh token
// @Tags Authentication
// @Accept json
// @Produce json
// @Param refreshRequest body RefreshTokenRequest true "Refresh token"
// @Success 200 {object} AuthResponse "New tokens generated"
// @Failure 400 {object} ErrorResponse "Invalid refresh token"
// @Failure 401 {object} ErrorResponse "Invalid or expired refresh token"
// @Router /auth/refresh [post]
func _() {}

// Logout godoc
// @Summary Logout the current user
// @Description Invalidate the current session
// @Security BearerAuth
// @Security SessionAuth
// @Tags Authentication
// @Produce json
// @Success 200 {object} SuccessResponse "Successfully logged out"
// @Failure 401 {object} ErrorResponse "Unauthorized"
// @Router /auth/logout [post]
func _() {}

// GetProfile godoc
// @Summary Get current user's profile
// @Description Retrieves the authenticated user's profile information
// @Security BearerAuth
// @Security SessionAuth
// @Tags Users
// @Produce json
// @Success 200 {object} UserProfile "User profile retrieved successfully"
// @Failure 401 {object} ErrorResponse "Unauthorized"
// @Router /users/profile [get]
func _() {}

// UpdateProfile godoc
// @Summary Update current user's profile
// @Description Updates the authenticated user's profile information
// @Security BearerAuth
// @Security SessionAuth
// @Tags Users
// @Accept json
// @Produce json
// @Param updateRequest body UpdateProfileRequest true "Profile update data"
// @Success 200 {object} UserProfile "Profile updated successfully"
// @Failure 400 {object} ErrorResponse "Invalid request data"
// @Failure 401 {object} ErrorResponse "Unauthorized"
// @Router /users/profile/update [put]
func _() {}

// ListUsers godoc
// @Summary List users
// @Description Retrieves a paginated list of users with optional filtering
// @Security BearerAuth
// @Security SessionAuth
// @Tags Users
// @Produce json
// @Param page query int false "Page number" default(1)
// @Param limit query int false "Number of items per page" default(20)
// @Param role query string false "Filter by user role" Enums(expert, evaluator, admin)
// @Param status query string false "Filter by user status" Enums(active, inactive, suspended)
// @Success 200 {object} PaginationResponse "List of users"
// @Failure 401 {object} ErrorResponse "Unauthorized"
// @Router /users [get]
func _() {}

// CreatePost godoc
// @Summary Create a new post
// @Description Creates a new post with the provided information
// @Security BearerAuth
// @Security SessionAuth
// @Tags Posts
// @Accept json
// @Produce json
// @Param postRequest body CreatePostRequest true "Post creation data"
// @Success 201 {object} PostResponse "Post created successfully"
// @Failure 400 {object} ErrorResponse "Invalid request data"
// @Failure 401 {object} ErrorResponse "Unauthorized"
// @Router /posts [post]
func _() {}

// ListPosts godoc
// @Summary List posts
// @Description Retrieves a paginated list of posts with optional filtering
// @Security BearerAuth
// @Security SessionAuth
// @Tags Posts
// @Produce json
// @Param page query int false "Page number" default(1)
// @Param limit query int false "Number of items per page" default(20)
// @Param category query string false "Filter by category"
// @Param tag query string false "Filter by tag"
// @Param sort query string false "Sort order" Enums(recent, popular, trending) default(recent)
// @Success 200 {object} PaginationResponse "List of posts"
// @Failure 401 {object} ErrorResponse "Unauthorized"
// @Router /posts [get]
func _() {}

// GetPost godoc
// @Summary Get a specific post
// @Description Retrieves detailed information about a specific post
// @Security BearerAuth
// @Security SessionAuth
// @Tags Posts
// @Produce json
// @Param id path int true "Post ID"
// @Success 200 {object} PostResponse "Post details"
// @Failure 401 {object} ErrorResponse "Unauthorized"
// @Failure 404 {object} ErrorResponse "Post not found"
// @Router /posts/{id} [get]
func _() {}

// UpdatePost godoc
// @Summary Update a post
// @Description Updates a post (owner, moderator, or admin only)
// @Security BearerAuth
// @Security SessionAuth
// @Tags Posts
// @Accept json
// @Produce json
// @Param id path int true "Post ID"
// @Param updateRequest body UpdatePostRequest true "Post update data"
// @Success 200 {object} PostResponse "Post updated successfully"
// @Failure 400 {object} ErrorResponse "Invalid request data"
// @Failure 401 {object} ErrorResponse "Unauthorized"
// @Failure 403 {object} ErrorResponse "Forbidden - insufficient permissions"
// @Failure 404 {object} ErrorResponse "Post not found"
// @Router /posts/{id} [put]
func _() {}

// DeletePost godoc
// @Summary Delete a post
// @Description Deletes a post (owner, moderator, or admin only)
// @Security BearerAuth
// @Security SessionAuth
// @Tags Posts
// @Produce json
// @Param id path int true "Post ID"
// @Success 200 {object} SuccessResponse "Post deleted successfully"
// @Failure 401 {object} ErrorResponse "Unauthorized"
// @Failure 403 {object} ErrorResponse "Forbidden - insufficient permissions"
// @Failure 404 {object} ErrorResponse "Post not found"
// @Router /posts/{id} [delete]
func _() {}

// CreateComment godoc
// @Summary Create a new comment
// @Description Creates a new comment on a post, question, or document
// @Security BearerAuth
// @Security SessionAuth
// @Tags Comments
// @Accept json
// @Produce json
// @Param commentRequest body CreateCommentRequest true "Comment creation data"
// @Success 201 {object} CommentResponse "Comment created successfully"
// @Failure 400 {object} ErrorResponse "Invalid request data"
// @Failure 401 {object} ErrorResponse "Unauthorized"
// @Router /comments [post]
func _() {}

// GetComment godoc
// @Summary Get a specific comment
// @Description Retrieves detailed information about a specific comment
// @Security BearerAuth
// @Security SessionAuth
// @Tags Comments
// @Produce json
// @Param id path int true "Comment ID"
// @Success 200 {object} CommentResponse "Comment details"
// @Failure 401 {object} ErrorResponse "Unauthorized"
// @Failure 404 {object} ErrorResponse "Comment not found"
// @Router /comments/{id} [get]
func _() {}

// CreateJob godoc
// @Summary Create a new job posting
// @Description Creates a new job posting
// @Security BearerAuth
// @Security SessionAuth
// @Tags Jobs
// @Accept json
// @Produce json
// @Param jobRequest body CreateJobRequest true "Job creation data"
// @Success 201 {object} JobResponse "Job created successfully"
// @Failure 400 {object} ErrorResponse "Invalid request data"
// @Failure 401 {object} ErrorResponse "Unauthorized"
// @Router /jobs [post]
func _() {}

// ListJobs godoc
// @Summary List job postings
// @Description Retrieves a paginated list of job postings
// @Security BearerAuth
// @Security SessionAuth
// @Tags Jobs
// @Produce json
// @Param page query int false "Page number" default(1)
// @Param limit query int false "Number of items per page" default(20)
// @Param location query string false "Filter by location"
// @Param type query string false "Filter by job type" Enums(full-time, part-time, contract, remote)
// @Success 200 {object} PaginationResponse "List of jobs"
// @Failure 401 {object} ErrorResponse "Unauthorized"
// @Router /jobs [get]
func _() {}

// Additional request/response models would go here...
// CreatePostRequest, UpdatePostRequest, PostResponse, etc.