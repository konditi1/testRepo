package utils

import (
	"context"
	"errors"
	"evalhub/internal/database"
	"fmt"
	"html"
	"image"
	"image/jpeg"
	"image/png"
	"log"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/gofrs/uuid"
	"golang.org/x/crypto/bcrypt"
)

// HashPassword hashes a plain text password using bcrypt.
func HashPassword(password string) (string, error) {
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hashedPassword), nil
}

// CheckPassword compares a hashed password with a plain text password.
func CheckPassword(hashedPassword, password string) error {
	return bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(password))
}

// GenerateSessionToken creates a unique session token using UUID.
func GenerateSessionToken() (string, error) {
	token, err := uuid.NewV4()
	return token.String(), err
}

// ValidatePasswordStrength checks if a password is strong enough.
func ValidatePasswordStrength(password string) error {
	var hasMinLength, hasUpper, hasLower, hasNumber, hasSpecial bool

	if len(password) >= 8 {
		hasMinLength = true
	}

	for _, char := range password {
		switch {
		case unicode.IsUpper(char):
			hasUpper = true
		case unicode.IsLower(char):
			hasLower = true
		case unicode.IsDigit(char):
			hasNumber = true
		case unicode.IsPunct(char) || unicode.IsSymbol(char):
			hasSpecial = true
		}
	}

	if !hasMinLength {
		return errors.New("password must be at least 8 characters long")
	}
	if !hasUpper {
		return errors.New("password must include at least one uppercase letter")
	}
	if !hasLower {
		return errors.New("password must include at least one lowercase letter")
	}
	if !hasNumber {
		return errors.New("password must include at least one numeric digit")
	}
	if !hasSpecial {
		return errors.New("password must include at least one special character")
	}

	return nil
}

// ValidateEmail checks if an email address is valid.
func ValidateEmail(email string) error {
	// Basic email validation regex
	emailRegex := regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)

	if !emailRegex.MatchString(email) {
		return errors.New("invalid email format")
	}

	// Check email length
	if len(email) > 254 {
		return errors.New("email address is too long")
	}

	// Check local part length
	parts := strings.Split(email, "@")
	if len(parts[0]) > 64 {
		return errors.New("email username part is too long")
	}

	return nil
}

// TimeAgo converts a timestamp into a human-readable string like "5 mins ago"
func TimeAgo(t time.Time) string {
	duration := time.Since(t)

	switch {
	case duration < time.Minute:
		seconds := int(duration.Seconds())
		if seconds <= 1 {
			return "just now"
		}
		return fmt.Sprintf("%d secs ago", seconds)
	case duration < time.Hour:
		minutes := int(duration.Minutes())
		return fmt.Sprintf("%d mins ago", minutes)
	case duration < 24*time.Hour:
		hours := int(duration.Hours())
		return fmt.Sprintf("%d hrs ago", hours)
	case duration < 7*24*time.Hour:
		days := int(duration.Hours() / 24)
		return fmt.Sprintf("%d days ago", days)
	case duration < 30*24*time.Hour:
		weeks := int(duration.Hours() / (24 * 7))
		return fmt.Sprintf("%d weeks ago", weeks)
	case duration < 365*24*time.Hour:
		months := int(duration.Hours() / (24 * 30))
		return fmt.Sprintf("%d months ago", months)
	default:
		years := int(duration.Hours() / (24 * 365))
		return fmt.Sprintf("%d years ago", years)
	}
}

// Utility function to truncate content
func TruncateContent(content string, wordLimit int) string {
	words := strings.Fields(content)
	if len(words) > wordLimit {
		return strings.Join(words[:wordLimit], " ") + "..."
	}
	return content
}

// CompressAndResizeImage resizes and compresses an image to the specified dimensions and quality.
// It only uses standard Go packages.
func CompressAndResizeImage(inputPath, outputPath string, maxWidth, maxHeight, quality int) error {
	// Open the input file
	file, err := os.Open(inputPath)
	if err != nil {
		return fmt.Errorf("failed to open input file: %v", err)
	}
	defer file.Close()

	// Decode the image (supports JPEG and PNG)
	img, format, err := image.Decode(file)
	if err != nil {
		return fmt.Errorf("failed to decode image: %v", err)
	}

	// Get the original dimensions
	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()

	// Calculate the new dimensions while maintaining the aspect ratio
	newWidth, newHeight := width, height
	if width > maxWidth || height > maxHeight {
		ratioX := float64(maxWidth) / float64(width)
		ratioY := float64(maxHeight) / float64(height)
		ratio := min(ratioX, ratioY)

		newWidth = int(float64(width) * ratio)
		newHeight = int(float64(height) * ratio)
	}

	// Create a new blank image with the new dimensions
	resizedImg := image.NewRGBA(image.Rect(0, 0, newWidth, newHeight))

	// Resize the original image into the new image using a simple scaling algorithm
	for y := 0; y < newHeight; y++ {
		for x := 0; x < newWidth; x++ {
			// Map the coordinates from the new image to the original image
			srcX := x * width / newWidth
			srcY := y * height / newHeight

			// Set the pixel in the new image
			resizedImg.Set(x, y, img.At(srcX, srcY))
		}
	}

	// Create the output file
	outFile, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("failed to create output file: %v", err)
	}
	defer outFile.Close()

	// Encode the image based on the original format
	switch format {
	case "jpeg", "jpg":
		err = jpeg.Encode(outFile, resizedImg, &jpeg.Options{Quality: quality})
	case "png":
		err = png.Encode(outFile, resizedImg)
	default:
		return fmt.Errorf("unsupported image format: %s", format)
	}

	if err != nil {
		return fmt.Errorf("failed to encode image: %v", err)
	}

	// Check the file size and adjust quality if necessary (only for JPEG)
	if format == "jpeg" || format == "jpg" {
		for {
			info, err := outFile.Stat()
			if err != nil {
				return fmt.Errorf("failed to get file info: %v", err)
			}

			// If the file size is less than 100KB, break the loop
			if info.Size() <= 100*1024 {
				break
			}

			// Reduce the quality further
			quality -= 5
			if quality < 5 {
				quality = 5
			}

			// Re-encode the image with the new quality
			outFile.Seek(0, 0)
			outFile.Truncate(0)
			err = jpeg.Encode(outFile, resizedImg, &jpeg.Options{Quality: quality})
			if err != nil {
				return fmt.Errorf("failed to re-encode JPEG: %v", err)
			}
		}
	}

	return nil
}

// min returns the smaller of two float64 numbers
func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

// ConfigureUploadHandler sets up the file upload handler
func ConfigureUpload() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Printf("Handling upload request: %s", r.URL.Path)

		// Set proper content type header based on file extension
		ext := strings.ToLower(filepath.Ext(r.URL.Path))
		switch ext {
		case ".jpg", ".jpeg":
			w.Header().Set("Content-Type", "image/jpeg")
		case ".png":
			w.Header().Set("Content-Type", "image/png")
		case ".gif":
			w.Header().Set("Content-Type", "image/gif")
		}

		// Serve the file
		http.FileServer(http.Dir(".")).ServeHTTP(w, r)
	}
}

// InitializeUploadDirectory creates and configures the uploads directory
func InitializeUploadDirectory() error {
	if _, err := os.Stat("uploads"); os.IsNotExist(err) {
		err = os.MkdirAll("uploads", 0755)
		if err != nil {
			return fmt.Errorf("failed to create uploads directory: %v", err)
		}
		log.Printf("Uploads directory created/verified at: %s", filepath.Join(".", "uploads"))
		if err := os.Chmod("uploads", 0755); err != nil {
			log.Printf("Warning: Failed to set uploads directory permissions: %v", err)
		}
	}
	return nil
}

// SanitizeString trims whitespace and escapes HTML to prevent XSS
func SanitizeString(s string) string {
	return html.EscapeString(strings.TrimSpace(s))
}

// FormatLastSeen formats a timestamp in WhatsApp style: "last seen today at 08:31", "last seen yesterday at 15:45", "last seen April 18, 2025"
func FormatLastSeen(t time.Time) string {
	now := time.Now()

	// Ensure both times are in the same timezone
	// No need to call Local() as PostgreSQL should return the time in the server's timezone
	// and we want to compare it as-is

	// Format time as HH:MM (24-hour format)
	timeStr := t.Format("15:04")

	// Check if it's today
	if now.Year() == t.Year() && now.Month() == t.Month() && now.Day() == t.Day() {
		return fmt.Sprintf("last seen today at %s", timeStr)
	}

	// Check if it's yesterday
	yesterday := now.AddDate(0, 0, -1)
	if yesterday.Year() == t.Year() && yesterday.Month() == t.Month() && yesterday.Day() == t.Day() {
		return fmt.Sprintf("last seen yesterday at %s", timeStr)
	}

	// If it's within the last 7 days, show the day name
	if now.Sub(t) < 7*24*time.Hour {
		return fmt.Sprintf("last seen %s at %s", t.Format("Monday"), timeStr)
	}

	// For older dates, show the date
	return fmt.Sprintf("last seen %s", t.Format("January 2, 2006"))
}

// GetUserOnlineStatus returns a user's online status and last seen time
func GetUserOnlineStatus(userID int) (bool, time.Time, error) {
	var isOnline bool
	var lastSeen time.Time

	query := `SELECT is_online, last_seen FROM users WHERE id = $1`
	err := database.DB.QueryRowContext(context.Background(), query, userID).Scan(&isOnline, &lastSeen)
	if err != nil {
		return false, time.Time{}, err
	}

	return isOnline, lastSeen, nil
}

// Add this function to your utils.go file

// GenerateInitialsAvatar creates HTML for an avatar with user initials
func GenerateInitialsAvatar(username string) string {
	if username == "" {
		return "<div class='initials-avatar' style='background-color: #ccc;'>?</div>"
	}

	// Get the initials (up to 2 characters)
	var initials string
	parts := strings.Fields(username)

	if len(parts) >= 2 {
		// If there are multiple names, take first letter of first and last name
		initials = string([]rune(parts[0])[0]) + string([]rune(parts[len(parts)-1])[0])
	} else if len(username) > 0 {
		// If there's just one name, take the first letter or first two letters
		if len([]rune(username)) >= 2 {
			initials = string([]rune(username)[0:2])
		} else {
			initials = username
		}
	} else {
		initials = "?"
	}

	// Generate a background color based on the username
	// Simple hash function to get a consistent color for the same username
	var hash int
	for _, r := range username {
		hash = int(r) + ((hash << 5) - hash)
	}

	// Convert hash to a color (HSL is easier to ensure readable colors)
	hue := math.Abs(float64(hash % 360))
	// Always use fully saturated colors with 50% lightness for good contrast
	// Format as HSL color string
	color := fmt.Sprintf("hsl(%.0f, 70%%, 60%%)", hue)

	// Generate the avatar HTML
	return fmt.Sprintf("<div class='initials-avatar' style='background-color: %s;'>%s</div>", color, initials)
}
