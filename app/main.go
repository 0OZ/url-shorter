// main.go - URL shortener in Go with PostgreSQL
package main

import (
	"database/sql"
	"fmt"
	"html/template"
	"log"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"time"

	_ "github.com/lib/pq" // PostgreSQL driver
)

// Database connection
var db *sql.DB

// characters used for generating short URLs
const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

// generateShortURL creates a random short code
func generateShortURL() string {
	seededRand := rand.New(rand.NewSource(time.Now().UnixNano()))
	b := make([]byte, 6) // 6 character short URLs
	for i := range b {
		b[i] = charset[seededRand.Intn(len(charset))]
	}
	return string(b)
}

// URL structure
type URL struct {
	ID            int
	ShortCode     string
	OriginalURL   string
	CreatedAt     time.Time
	FormattedDate string
}

// Template data structure
type TemplateData struct {
	OriginalURL string
	ShortURL    string
	Host        string
	Display     string
	RecentURLs  []URL
	CurrentYear int
}

// getRecentURLs retrieves the 10 most recent URLs from the database
func getRecentURLs() ([]URL, error) {
	rows, err := db.Query("SELECT short_code, original_url, created_at FROM urls ORDER BY created_at DESC LIMIT 10")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var urls []URL
	for rows.Next() {
		var url URL
		var createdAt time.Time
		if err := rows.Scan(&url.ShortCode, &url.OriginalURL, &createdAt); err != nil {
			return nil, err
		}
		url.CreatedAt = createdAt
		url.FormattedDate = createdAt.Format("Jan 02, 2006 15:04")
		urls = append(urls, url)
	}

	return urls, nil
}

// Initialize database connection and create the table if needed
func initDB() *sql.DB {
	// Get connection details from environment variables or use defaults
	host := getEnv("DB_HOST", "postgres")
	port := getEnv("DB_PORT", "5432")
	user := getEnv("DB_USER", "postgres")
	password := getEnv("DB_PASSWORD", "postgres")
	dbname := getEnv("DB_NAME", "urlshortener")

	// Build connection string
	connStr := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		host, port, user, password, dbname)

	// Connect to database
	var err error
	db, err = sql.Open("postgres", connStr)
	if err != nil {
		log.Fatal("Error connecting to database:", err)
	}

	// Test connection
	err = db.Ping()
	if err != nil {
		log.Fatal("Could not ping database:", err)
	}

	// Create table if it doesn't exist
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS urls (
			id SERIAL PRIMARY KEY,
			short_code VARCHAR(10) UNIQUE NOT NULL,
			original_url TEXT NOT NULL,
			created_at TIMESTAMP NOT NULL DEFAULT NOW()
		)
	`)
	if err != nil {
		log.Fatal("Error creating table:", err)
	}

	// Create index on short_code for faster lookups
	_, err = db.Exec("CREATE INDEX IF NOT EXISTS idx_short_code ON urls(short_code)")
	if err != nil {
		log.Fatal("Error creating index:", err)
	}

	fmt.Println("Database initialized successfully")
	return db
}

// Helper function to get environment variables with defaults
func getEnv(key, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value
}

func main() {
	// Initialize database
	db = initDB()
	defer db.Close()

	// Initialize template
	tmplPath := filepath.Join("app", "templates", "home.html")
	tmpl, err := template.ParseFiles(tmplPath)
	if err != nil {
		log.Fatal("Error parsing template:", err)
	}

	// Handle the home page and URL shortening
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// If it's the root path and GET request, show the form
		if r.URL.Path == "/" && r.Method == "GET" {
			// Get recent URLs
			recentURLs, err := getRecentURLs()
			if err != nil {
				log.Println("Error getting recent URLs:", err)
				// Continue anyway, just show empty list
				recentURLs = []URL{}
			}

			data := TemplateData{
				Display:     "none",
				RecentURLs:  recentURLs,
				Host:        r.Host,
				CurrentYear: time.Now().Year(),
			}
			tmpl.Execute(w, data)
			return
		}

		// Handle form submission
		if r.URL.Path == "/" && r.Method == "POST" {
			err := r.ParseForm()
			if err != nil {
				http.Error(w, "Error parsing form", http.StatusBadRequest)
				return
			}

			originalURL := r.FormValue("url")
			if originalURL == "" {
				http.Error(w, "URL is required", http.StatusBadRequest)
				return
			}

			// Generate a short code
			shortCode := generateShortURL()

			// Store the URL in the database
			_, err = db.Exec("INSERT INTO urls (short_code, original_url) VALUES ($1, $2)",
				shortCode, originalURL)
			if err != nil {
				log.Println("Error inserting URL into database:", err)
				http.Error(w, "Error shortening URL", http.StatusInternalServerError)
				return
			}

			// Get recent URLs for display
			recentURLs, err := getRecentURLs()
			if err != nil {
				log.Println("Error getting recent URLs:", err)
				// Continue anyway
				recentURLs = []URL{}
			}

			// Render the template with the result
			data := TemplateData{
				OriginalURL: originalURL,
				ShortURL:    "/" + shortCode,
				Host:        r.Host,
				Display:     "block",
				RecentURLs:  recentURLs,
				CurrentYear: time.Now().Year(),
			}
			tmpl.Execute(w, data)
			return
		}

		// Otherwise, check if the path is a short URL and redirect
		shortCode := r.URL.Path[1:] // Remove the leading slash
		if shortCode != "" {
			var originalURL string
			err := db.QueryRow("SELECT original_url FROM urls WHERE short_code = $1", shortCode).Scan(&originalURL)
			if err == nil {
				http.Redirect(w, r, originalURL, http.StatusFound)
				return
			}
		}

		// If not found, show a 404 page
		http.NotFound(w, r)
	})

	// Get port from environment variable or use default
	port := getEnv("PORT", "8080")
	fmt.Printf("Server starting on port %s...\n", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
