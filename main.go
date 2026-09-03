package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"
)

var tokens = make(map[string]Token)
var mu sync.Mutex
var jsonFile string

type Token struct {
	ID          string    `json:"id"`
	CreatedAt   time.Time `json:"created_at"`
	IsActivated bool      `json:"is_activated"`
	ActivatedAt time.Time `json:"activated_at,omitempty"`
	IP          string    `json:"ip,omitempty"`
	UserAgent   string    `json:"user_agent,omitempty"`
}

func generateToken() (string, error) {
	b := make([]byte, 16)
	_, err := rand.Read(b)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
func generateTokenHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	token, err := generateToken()
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		log.Printf("Generation error: %v", err)
		return
	}
	mu.Lock()
	tokens[token] = Token{
		ID:          token,
		CreatedAt:   time.Now(),
		IsActivated: false,
	}
	mu.Unlock()
	if jsonFile != "" {
		go saveData(jsonFile)
	}
	fmt.Fprintln(w, token)
}
func checkTokenHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	tokenStr := r.URL.Query().Get("token")
	if tokenStr == "" {
		http.Error(w, "missing token parameter", http.StatusBadRequest)
		return
	}
	mu.Lock()
	defer mu.Unlock()
	token, exists := tokens[tokenStr]
	if !exists {
		http.Error(w, "token not found", http.StatusNotFound)
		return
	}
	if token.IsActivated {
		http.Error(w, "token already activated", http.StatusConflict)
		return
	}
	token.IsActivated = true
	token.ActivatedAt = time.Now()
	token.IP = r.RemoteAddr
	token.UserAgent = r.UserAgent()
	tokens[tokenStr] = token
	log.Printf("Token %v is activated from IP: %v, User-Agent: %v", tokenStr, token.IP, token.UserAgent)
	w.WriteHeader(http.StatusOK)
	if jsonFile != "" {
		go saveData(jsonFile)
	}
	fmt.Fprintln(w, "Token activated")
}
func loadData(filename string) {
	data, err := os.ReadFile(filename)
	if err != nil {
		if os.IsNotExist(err) {
			return
		}
		log.Printf("File reading error: %v", err)
	}
	var loadTokens map[string]Token
	err = json.Unmarshal(data, &loadTokens)
	if err != nil {
		log.Printf("Error parsing JSON: %v", err)
		return
	}
	mu.Lock()
	tokens = loadTokens
	mu.Unlock()
	log.Printf("Loaded %v tokens from %v", len(tokens), filename)
}
func saveData(filename string) {
	mu.Lock()
	defer mu.Unlock()
	data, err := json.MarshalIndent(tokens, "", " ")
	if err != nil {
		log.Printf("Error marshaling tokens: %v", err)
		return
	}
	err = os.WriteFile(filename, data, 0644)
	if err != nil {
		log.Printf("Error writing to file: %v", err)
		return
	} else {
		log.Printf("Saving %v tokens to %v", len(tokens), filename)
	}
}
func main() {
	portFlag := flag.String("port", "8080", "server port")
	jsonFlag := flag.String("json", "", "the name of the JSON file")
	flag.Parse()
	port := *portFlag
	jsonFile = strings.TrimSpace(*jsonFlag)
	if jsonFile != "" {
		loadData(jsonFile)
	}
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		if jsonFile != "" {
			saveData(jsonFile)
		}
		os.Exit(0)
	}()
	http.HandleFunc("/generate", generateTokenHandler)
	http.HandleFunc("/check", checkTokenHandler)
	log.Printf("The server is running on port %v", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
