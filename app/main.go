package main

import (
	"bufio"
	"database/sql"
	"fmt"
	"html/template"
	"log"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
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
	CurrentYear int
}

// loadEnvFile loads environment variables from a file
func loadEnvFile(filePath string) {
	log.Printf("Trying to load environment variables from: %s", filePath)
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		log.Printf("File %s does not exist, skipping", filePath)
		return
	}

	file, err := os.Open(filePath)
	if err != nil {
		log.Printf("Error opening %s: %v", filePath, err)
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		// Skip comments and empty lines
		if strings.HasPrefix(line, "#") || strings.TrimSpace(line) == "" {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		// Only set if not already set
		if os.Getenv(key) == "" {
			os.Setenv(key, value)
			log.Printf("Set environment variable: %s", key)
		}
	}

	if err := scanner.Err(); err != nil {
		log.Printf("Error reading %s: %v", filePath, err)
	}

	log.Printf("Environment variables loaded from %s", filePath)
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

	log.Printf("Connecting to database: %s:%s/%s", host, port, dbname)

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

// getShortCodeForURL checks if a URL already exists and returns its short code
func getShortCodeForURL(url string) (string, error) {
	var shortCode string
	err := db.QueryRow("SELECT short_code FROM urls WHERE original_url = $1", url).Scan(&shortCode)
	if err == sql.ErrNoRows {
		return "", nil // URL doesn't exist
	}
	return shortCode, err
}

func main() {
	// Load local environment variables if available
	workingDir, err := os.Getwd()
	if err == nil {
		// Check if running in Docker by looking for /.dockerenv file
		_, dockerErr := os.Stat("/.dockerenv")
		isDocker := !os.IsNotExist(dockerErr)

		rootDir := filepath.Dir(workingDir)
		if isDocker {
			// Use .env file in Docker environment
			if filepath.Base(workingDir) == "app" {
				loadEnvFile(filepath.Join(rootDir, ".env"))
			} else {
				loadEnvFile(filepath.Join(workingDir, ".env"))
			}
		} else {
			// Use .env.local in non-Docker environment
			if filepath.Base(workingDir) == "app" {
				loadEnvFile(filepath.Join(rootDir, ".env.local"))
			} else {
				loadEnvFile(filepath.Join(workingDir, ".env.local"))
			}
		}
	}

	// Initialize database
	db = initDB()
	defer db.Close()

	// Initialize template
	var tmplPath string
	if filepath.Base(workingDir) == "app" {
		tmplPath = filepath.Join("templates", "home.html")
	} else {
		tmplPath = filepath.Join("app", "templates", "home.html")
	}

	tmpl, err := template.ParseFiles(tmplPath)
	if err != nil {
		log.Fatal("Error parsing template:", err)
	}

	// Handle the home page and URL shortening
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// If it's the root path and GET request, show the form
		if r.URL.Path == "/" && r.Method == "GET" {
			// No longer need to fetch recent URLs from the database
			data := TemplateData{
				Display:     "none",
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

			// Check if URL already exists
			existingShortCode, err := getShortCodeForURL(originalURL)
			if err != nil {
				log.Println("Error checking for existing URL:", err)
				http.Error(w, "Error shortening URL", http.StatusInternalServerError)
				return
			}

			shortCode := existingShortCode
			isNew := false

			if shortCode == "" {
				// Generate a new short code if URL doesn't exist
				shortCode = generateShortURL()
				isNew = true

				// Store the URL in the database
				_, err = db.Exec("INSERT INTO urls (short_code, original_url) VALUES ($1, $2)",
					shortCode, originalURL)
				if err != nil {
					log.Println("Error inserting URL into database:", err)
					http.Error(w, "Error shortening URL", http.StatusInternalServerError)
					return
				}
			}

			// Redirect to prevent form resubmission
			redirectURL := fmt.Sprintf("/?short=%s&url=%s&new=%t",
				shortCode, url.QueryEscape(originalURL), isNew)
			http.Redirect(w, r, redirectURL, http.StatusSeeOther)
			return
		}

		// Handle GET request with short code parameter (after redirect)
		if r.URL.Path == "/" && r.Method == "GET" && r.URL.Query().Get("short") != "" {
			shortCode := r.URL.Query().Get("short")
			originalURL := r.URL.Query().Get("url")

			// No longer need to fetch recent URLs from the database
			// Render the template with the result
			data := TemplateData{
				OriginalURL: originalURL,
				ShortURL:    "/" + shortCode,
				Host:        r.Host,
				Display:     "block",
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
