package models

import "time"

// Document represents a file in the document repository
type Document struct {
	ID               int       `json:"id"`
	UserID           int       `json:"user_id"`
	Title            string    `json:"title"`
	Description      string    `json:"description"`
	FileURL          string    `json:"file_url"`
	FilePublicID     string    `json:"file_public_id"`
	FileType         string    `json:"file_type"`
	Version          int       `json:"version"`
	Tags             string    `json:"tags"`
	TagsArray        []string  `json:"tags_array"`
	IsPublic         bool      `json:"is_public"`
	DownloadCount    int       `json:"download_count"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
	CreatedAtHuman   string    `json:"created_at_human"`
	UpdatedAtHuman   string    `json:"updated_at_human"`
	Username         string    `json:"username"`           // Document uploader's username
	AuthorProfileURL string    `json:"author_profile_url"` // Profile picture of document uploader
	Likes            int       `json:"likes"`
	Dislikes         int       `json:"dislikes"`
	CommentsCount    int       `json:"comments_count"`
	IsOwner          bool      `json:"is_owner"`
}

// DocumentComment represents a comment on a document
type DocumentComment struct {
	ID               int64     `json:"id"`
	DocumentID       int64     `json:"document_id"`
	UserID           int64     `json:"user_id"`
	Username         string    `json:"username"`
	AuthorProfileURL string    `json:"author_profile_url"`
	Content          string    `json:"content"`
	CreatedAt        time.Time `json:"created_at"`
	CreatedAtHuman   string    `json:"created_at_human"`
	Likes            int       `json:"likes"`
	Dislikes         int       `json:"dislikes"`
	IsOwner          bool      `json:"is_owner"`
}
