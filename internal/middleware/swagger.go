package middleware

import (
	"net/http"
	"os"

	httpSwagger "github.com/swaggo/http-swagger"
)

// SwaggerConfig represents the configuration for Swagger middleware
type SwaggerConfig struct {
	// URL points to the Swagger JSON endpoint
	URL string
	// DeepLinking enables deep linking for tags and operations
	DeepLinking bool
	// DocExpansion controls the default expansion setting for the operations and tags
	DocExpansion string
}

// DefaultSwaggerConfig returns the default Swagger configuration
func DefaultSwaggerConfig() *SwaggerConfig {
	return &SwaggerConfig{
		URL:          "/swagger/doc.json",
		DeepLinking:  true,
		DocExpansion: "list",
	}
}

// SwaggerHandler returns a handler that serves the Swagger UI
func SwaggerHandler(config *SwaggerConfig) http.Handler {
	if config == nil {
		config = DefaultSwaggerConfig()
	}

	return httpSwagger.Handler(
		httpSwagger.URL(config.URL),
		httpSwagger.DeepLinking(config.DeepLinking),
		httpSwagger.DocExpansion(config.DocExpansion),
		httpSwagger.DomID("#swagger-ui"),
	)
}

// SwaggerAuthMiddleware adds basic authentication for Swagger UI in production
func SwaggerAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip authentication in development
		if os.Getenv("ENV") != "production" {
			next.ServeHTTP(w, r)
			return
		}

		// Get username and password from environment variables
		username := os.Getenv("SWAGGER_USERNAME")
		password := os.Getenv("SWAGGER_PASSWORD")

		// If no credentials are set, allow access
		if username == "" && password == "" {
			next.ServeHTTP(w, r)
			return
		}

		// Get Basic Auth credentials
		user, pass, ok := r.BasicAuth()
		if !ok || user != username || pass != password {
			w.Header().Set("WWW-Authenticate", `Basic realm="Swagger Documentation"`)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	})
}
