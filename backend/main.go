package main

import (
	"bufio"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	_ "github.com/lib/pq"
)

// ==================== Types ====================

type ModelRequest struct {
	Scenario     int     `json:"scenario"`
	DrillingRate int     `json:"drillingRate"`
	OilPrice     float64 `json:"oilPrice"`
	ExchangeRate float64 `json:"exchangeRate"`
}

type SimulationResult struct {
	Year             float64 `json:"year"`
	Scenario         int     `json:"scenario"`
	Revenue          float64 `json:"revenue"`
	ProductionVolume float64 `json:"productionVolume"`
	NewWellsFund     float64 `json:"newWellsFund"`
	OldWellsFund     float64 `json:"oldWellsFund"`
}

type APIResponse struct {
	Success bool        `json:"success"`
	Message string      `json:"message,omitempty"`
	Error   string      `json:"error,omitempty"`
	Data    interface{} `json:"data,omitempty"`
}

type User struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type RequestLog struct {
	ID           int       `json:"id"`
	Username     string    `json:"username"`
	Timestamp    time.Time `json:"timestamp"`
	Scenario     int       `json:"scenario"`
	DrillingRate int       `json:"drillingRate"`
	OilPrice     float64   `json:"oilPrice"`
	ExchangeRate float64   `json:"exchangeRate"`
	Success      bool      `json:"success"`
	ResultCount  int       `json:"resultCount"`
	Error        string    `json:"error,omitempty"`
}

// ==================== Global State ====================

var (
	db       *sql.DB
	users    = make(map[string]string) // username -> password
	sessions = make(map[string]string) // token -> username
	mu       sync.RWMutex
)

func init() {
	users["admin"] = "admin123"
	users["user"] = "user123"
}

func main() {
	wd, err := os.Getwd()
	if err != nil {
		log.Fatal("Failed to get working directory:", err)
	}
	projectRoot := filepath.Dir(wd)

	// Connect to PostgreSQL
	connStr := "host=localhost port=5432 user=postgres password=postgres dbname=AnyLogicDB sslmode=disable"
	db, err = sql.Open("postgres", connStr)
	if err != nil {
		log.Printf("Warning: Failed to connect to database: %v", err)
	} else {
		if err := db.Ping(); err != nil {
			log.Printf("Warning: Database ping failed: %v", err)
		} else {
			log.Println("Connected to PostgreSQL database")
			initDatabase()
		}
	}

	fmt.Println("==========================================")
	fmt.Println("  Oil Company Model Server v2.0")
	fmt.Println("==========================================")
	fmt.Println("  Project root:", projectRoot)
	fmt.Println()
	fmt.Println("  API Endpoints:")
	fmt.Println("    POST /api/login      - Login")
	fmt.Println("    POST /api/register   - Register new user")
	fmt.Println("    POST /api/logout     - Logout")
	fmt.Println("    POST /api/run-model  - Run simulation (auth required)")
	fmt.Println("    GET  /api/history    - Request history (auth required)")
	fmt.Println("    GET  /api/status     - Server status")
	fmt.Println()
	fmt.Println("  Default users: admin/admin123, user/user123")
	fmt.Println("  Frontend: http://localhost:8080")
	fmt.Println("==========================================")

	frontendDir := filepath.Join(projectRoot, "frontend")
	os.MkdirAll(frontendDir, 0755)

	http.HandleFunc("/", handleStatic(projectRoot))
	http.HandleFunc("/api/login", handleLogin)
	http.HandleFunc("/api/register", handleRegister)
	http.HandleFunc("/api/logout", handleLogout)
	http.HandleFunc("/api/run-model", authMiddleware(handleRunModel(projectRoot)))
	http.HandleFunc("/api/history", authMiddleware(handleHistory))
	http.HandleFunc("/api/status", handleStatus)

	log.Println("Server starting on :8080...")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Fatal("Server failed:", err)
	}
}

func initDatabase() {
	query := `
	CREATE TABLE IF NOT EXISTS request_logs (
		id SERIAL PRIMARY KEY,
		username VARCHAR(255) NOT NULL,
		timestamp TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		scenario INT,
		drilling_rate INT,
		oil_price DOUBLE PRECISION,
		exchange_rate DOUBLE PRECISION,
		success BOOLEAN,
		result_count INT,
		error_msg TEXT
	)`
	if _, err := db.Exec(query); err != nil {
		log.Printf("Failed to create request_logs table: %v", err)
	} else {
		log.Println("Database table 'request_logs' ready")
	}
}

func logRequest(username string, req ModelRequest, success bool, resultCount int, errMsg string) {
	if db == nil {
		return
	}
	query := `INSERT INTO request_logs (username, scenario, drilling_rate, oil_price, exchange_rate, success, result_count, error_msg)
			  VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`
	_, err := db.Exec(query, username, req.Scenario, req.DrillingRate, req.OilPrice, req.ExchangeRate, success, resultCount, errMsg)
	if err != nil {
		log.Printf("Failed to log request: %v", err)
	}
}

func getRequestHistory(username string) ([]RequestLog, error) {
	if db == nil {
		return nil, fmt.Errorf("database not connected")
	}

	query := `SELECT id, username, timestamp, scenario, drilling_rate, oil_price, exchange_rate, success, result_count, COALESCE(error_msg, '')
			  FROM request_logs WHERE username = $1 ORDER BY timestamp DESC LIMIT 50`
	rows, err := db.Query(query, username)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []RequestLog
	for rows.Next() {
		var l RequestLog
		if err := rows.Scan(&l.ID, &l.Username, &l.Timestamp, &l.Scenario, &l.DrillingRate, &l.OilPrice, &l.ExchangeRate, &l.Success, &l.ResultCount, &l.Error); err != nil {
			continue
		}
		logs = append(logs, l)
	}
	return logs, nil
}

func generateToken() string {
	b := make([]byte, 32)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func handleLogin(w http.ResponseWriter, r *http.Request) {
	setCORSHeaders(w)
	if r.Method == "OPTIONS" {
		return
	}
	if r.Method != "POST" {
		sendError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var user User
	if err := json.NewDecoder(r.Body).Decode(&user); err != nil {
		sendError(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	mu.RLock()
	password, exists := users[user.Username]
	mu.RUnlock()

	if !exists || password != user.Password {
		sendError(w, "Invalid username or password", http.StatusUnauthorized)
		return
	}

	token := generateToken()
	mu.Lock()
	sessions[token] = user.Username
	mu.Unlock()

	log.Printf("User '%s' logged in", user.Username)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(APIResponse{
		Success: true,
		Message: "Login successful",
		Data: map[string]string{
			"token":    token,
			"username": user.Username,
		},
	})
}

func handleRegister(w http.ResponseWriter, r *http.Request) {
	setCORSHeaders(w)
	if r.Method == "OPTIONS" {
		return
	}
	if r.Method != "POST" {
		sendError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var user User
	if err := json.NewDecoder(r.Body).Decode(&user); err != nil {
		sendError(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	if len(user.Username) < 3 || len(user.Password) < 4 {
		sendError(w, "Username must be 3+ chars, password 4+ chars", http.StatusBadRequest)
		return
	}

	mu.Lock()
	if _, exists := users[user.Username]; exists {
		mu.Unlock()
		sendError(w, "Username already exists", http.StatusConflict)
		return
	}
	users[user.Username] = user.Password
	mu.Unlock()

	log.Printf("New user registered: '%s'", user.Username)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(APIResponse{
		Success: true,
		Message: "Registration successful. Please login.",
	})
}

func handleLogout(w http.ResponseWriter, r *http.Request) {
	setCORSHeaders(w)
	if r.Method == "OPTIONS" {
		return
	}

	token := r.Header.Get("Authorization")
	if strings.HasPrefix(token, "Bearer ") {
		token = strings.TrimPrefix(token, "Bearer ")
	}

	mu.Lock()
	delete(sessions, token)
	mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(APIResponse{
		Success: true,
		Message: "Logged out",
	})
}

func authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		setCORSHeaders(w)
		if r.Method == "OPTIONS" {
			return
		}

		token := r.Header.Get("Authorization")
		if strings.HasPrefix(token, "Bearer ") {
			token = strings.TrimPrefix(token, "Bearer ")
		}

		mu.RLock()
		username, exists := sessions[token]
		mu.RUnlock()

		if !exists || token == "" {
			sendError(w, "Unauthorized. Please login.", http.StatusUnauthorized)
			return
		}

		// Add username to request context via header (simple approach)
		r.Header.Set("X-Username", username)
		next(w, r)
	}
}

// ==================== Handlers ====================

func handleStatic(projectRoot string) http.HandlerFunc {
	frontendDir := filepath.Join(projectRoot, "frontend")
	return func(w http.ResponseWriter, r *http.Request) {
		setCORSHeaders(w)
		if r.Method == "OPTIONS" {
			return
		}

		path := r.URL.Path
		if path == "/" {
			path = "/index.html"
		}

		fullPath := filepath.Join(frontendDir, path)
		if !strings.HasPrefix(fullPath, frontendDir) {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		switch {
		case strings.HasSuffix(path, ".html"):
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
		case strings.HasSuffix(path, ".css"):
			w.Header().Set("Content-Type", "text/css; charset=utf-8")
		case strings.HasSuffix(path, ".js"):
			w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
		}

		http.ServeFile(w, r, fullPath)
	}
}

func handleStatus(w http.ResponseWriter, r *http.Request) {
	setCORSHeaders(w)
	if r.Method == "OPTIONS" {
		return
	}

	dbStatus := "disconnected"
	if db != nil && db.Ping() == nil {
		dbStatus = "connected"
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(APIResponse{
		Success: true,
		Message: "Server is running",
		Data: map[string]interface{}{
			"timestamp": time.Now().Unix(),
			"version":   "2.0.0",
			"database":  dbStatus,
		},
	})
}

func handleHistory(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		sendError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	username := r.Header.Get("X-Username")
	logs, err := getRequestHistory(username)
	if err != nil {
		sendError(w, "Failed to fetch history: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(APIResponse{
		Success: true,
		Data:    logs,
	})
}

func handleRunModel(projectRoot string) http.HandlerFunc {
	modelDir := filepath.Join(projectRoot, "model")

	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			sendError(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		username := r.Header.Get("X-Username")

		var req ModelRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			sendError(w, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
			return
		}

		// Validate parameters
		if req.Scenario < 1 || req.Scenario > 3 {
			req.Scenario = 1
		}
		if req.DrillingRate <= 0 {
			req.DrillingRate = 50
		}
		if req.OilPrice <= 0 {
			req.OilPrice = 80.0
		}
		if req.ExchangeRate <= 0 {
			req.ExchangeRate = 75.0
		}

		log.Printf("[%s] Running model: scenario=%d, drilling=%d, oilPrice=%.2f, exchange=%.2f",
			username, req.Scenario, req.DrillingRate, req.OilPrice, req.ExchangeRate)

		classpath := strings.Join([]string{
			modelDir,
			filepath.Join(modelDir, "model.jar"),
			filepath.Join(modelDir, "lib", "*"),
			filepath.Join(modelDir, "lib", "logging", "*"),
			filepath.Join(modelDir, "lib", "database", "*"),
			filepath.Join(modelDir, "lib", "database", "querydsl", "*"),
			filepath.Join(modelDir, "lib", "database", "ucanaccess", "*"),
		}, ":")

		cmd := exec.Command("java",
			"-cp", classpath,
			"ModelRunner",
			strconv.Itoa(req.Scenario),
			strconv.Itoa(req.DrillingRate),
			fmt.Sprintf("%.2f", req.OilPrice),
			fmt.Sprintf("%.2f", req.ExchangeRate),
		)
		cmd.Dir = modelDir

		output, err := cmd.Output()
		if err != nil {
			errMsg := ""
			if exitErr, ok := err.(*exec.ExitError); ok {
				errMsg = string(exitErr.Stderr)
			} else {
				errMsg = err.Error()
			}
			log.Printf("[%s] Model execution failed: %s", username, errMsg)
			logRequest(username, req, false, 0, errMsg)
			sendError(w, "Model execution failed: "+errMsg, http.StatusInternalServerError)
			return
		}

		results, err := parseCSVOutput(string(output))
		if err != nil {
			log.Printf("[%s] Failed to parse results: %v", username, err)
			logRequest(username, req, false, 0, err.Error())
			sendError(w, "Failed to parse results: "+err.Error(), http.StatusInternalServerError)
			return
		}

		log.Printf("[%s] Model completed successfully, %d results", username, len(results))
		logRequest(username, req, true, len(results), "")

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(APIResponse{
			Success: true,
			Message: "Simulation completed",
			Data: map[string]interface{}{
				"parameters": req,
				"results":    results,
				"timestamp":  time.Now().Unix(),
			},
		})
	}
}

// ==================== Helpers ====================

func parseCSVOutput(output string) ([]SimulationResult, error) {
	var results []SimulationResult
	scanner := bufio.NewScanner(strings.NewReader(output))
	lineNum := 0

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		lineNum++
		if line == "" || lineNum == 1 {
			continue
		}

		parts := strings.Split(line, ",")
		if len(parts) < 6 {
			continue
		}

		year, _ := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
		scenario, _ := strconv.Atoi(strings.TrimSpace(parts[1]))
		revenue, _ := strconv.ParseFloat(strings.TrimSpace(parts[2]), 64)
		production, _ := strconv.ParseFloat(strings.TrimSpace(parts[3]), 64)
		newWells, _ := strconv.ParseFloat(strings.TrimSpace(parts[4]), 64)
		oldWells, _ := strconv.ParseFloat(strings.TrimSpace(parts[5]), 64)

		results = append(results, SimulationResult{
			Year:             year,
			Scenario:         scenario,
			Revenue:          revenue,
			ProductionVolume: production,
			NewWellsFund:     newWells,
			OldWellsFund:     oldWells,
		})
	}
	return results, scanner.Err()
}

func setCORSHeaders(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
}

func sendError(w http.ResponseWriter, message string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(APIResponse{
		Success: false,
		Error:   message,
	})
}
