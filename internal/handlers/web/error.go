// file: internal/handlers/web/error.go
package web

import (
	"fmt"
	"log"
	"net/http"
)

func RenderErrorPage(w http.ResponseWriter, statusCode int, err error) {
	// Log the error for debugging
	log.Printf("Error %d: %v", statusCode, err)

	// Set the response status code
	w.WriteHeader(statusCode)

	data := map[string]interface{}{
		"Title":      fmt.Sprintf("%d Error", statusCode),
		"IsLoggedIn": false, // Default to false for error pages
	}

	var templateName string

	switch statusCode {
	case http.StatusBadRequest:
		templateName = "400"
	case http.StatusUnauthorized:
		templateName = "401"
	case http.StatusForbidden:
		templateName = "403"
	case http.StatusNotFound:
		templateName = "404"
	case http.StatusMethodNotAllowed:
		templateName = "405"
	case http.StatusInternalServerError:
		templateName = "500"
	default:
		templateName = "500" // Default to 500 for unknown errors
	}

	err = templates.ExecuteTemplate(w, templateName, data)
	if err != nil {
		log.Printf("Error rendering error template: %v", err)
		http.Error(w, fmt.Sprintf("%d - Server Error", statusCode), statusCode)
	}
}

func NotFound(w http.ResponseWriter, r *http.Request) {
	log.Printf("404 Not Found: %s", r.URL.Path)
	RenderErrorPage(w, http.StatusNotFound, fmt.Errorf("page not found: %s", r.URL.Path))
}

// // RenderErrorPage displays an error page with appropriate status code
// func RenderErrorPage(w http.ResponseWriter, statusCode int, err error) {
// 	w.WriteHeader(statusCode)
// 	data := map[string]interface{}{
// 		"Title":      fmt.Sprintf("Error %d - EvalHub", statusCode),
// 		"StatusCode": statusCode,
// 		"Error":      err.Error(),
// 	}
// 	templates.ExecuteTemplate(w, "error", data)
// }