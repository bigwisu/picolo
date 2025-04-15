// main.go
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	// ** UPDATED Import path for Dialogflow CX client **
	cx "cloud.google.com/go/dialogflow/cx/apiv3" // Use v3 (GA) instead of v3beta1 if possible
	// "github.com/google/uuid"
	"github.com/rs/cors" // For CORS handling
	// ** UPDATED Import path for Dialogflow CX protobuf types **
	cxpb "google.golang.org/genproto/googleapis/cloud/dialogflow/cx/v3"

	// ** ADDED IMPORT for client options **
	"google.golang.org/api/option"
	// structpb is usually needed for parameters, keeping it for now
	// structpb "google.golang.org/protobuf/types/known/structpb"
)

// Configuration struct to hold environment variables
type config struct {
	ProjectID     string
	LocationID    string
	AllowedOrigin string
	Port          string
	// Optional: Default Agent ID if not provided in request
	DefaultAgentID string
}

// Request struct matching the expected JSON body from the client
type DetectIntentRequest struct {
	Message      string `json:"message"`
	AgentID      string `json:"agentId"`      // CX requires Agent ID for session path
	SessionID    string `json:"sessionId"`    // CX requires Session ID
	LanguageCode string `json:"languageCode"` // Optional language code
}

// Response struct sent back to the client
// Simplified to match the JS example (returning first text response)
type DetectIntentResponse struct {
	Text      string `json:"text"` // Field to hold the extracted text response
	SessionID string `json:"sessionId"`
}

var (
	appConfig config
	// ** UPDATED Client Type for CX **
	sessionsClient *cx.SessionsClient
)

func main() {
	var err error
	ctx := context.Background()

	// --- Load Configuration from Environment Variables ---
	appConfig = loadConfig() // Ensure LocationID and ProjectID are loaded correctly

	// --- Initialize Dialogflow CX Client ---
	// Construct the regional endpoint string based on the LocationID config
	// CX uses the same regional endpoint format as ES
	regionalEndpoint := fmt.Sprintf("%s-dialogflow.googleapis.com:443", appConfig.LocationID)
	log.Printf("Using Dialogflow CX regional endpoint: %s", regionalEndpoint)

	// ** UPDATED Client Initialization for CX **
	sessionsClient, err = cx.NewSessionsClient(ctx, option.WithEndpoint(regionalEndpoint))
	if err != nil {
		log.Fatalf("Failed to create Dialogflow CX sessions client: %v", err)
	}
	defer sessionsClient.Close()

	log.Printf("Dialogflow CX client initialized for project %s, location %s", appConfig.ProjectID, appConfig.LocationID)

	// --- Setup HTTP Server & Routing ---
	mux := http.NewServeMux()
	mux.HandleFunc("/api/dialogflow/detectIntent", detectIntentHandler) // Keep same endpoint path for now
	mux.HandleFunc("/healthz", healthCheckHandler)

	// --- CORS Configuration ---
	c := cors.New(cors.Options{
		AllowedOrigins: []string{appConfig.AllowedOrigin},
		AllowedMethods: []string{"POST", "OPTIONS"},
		AllowedHeaders: []string{"Content-Type", "Authorization"},
		OptionsPassthrough: false,
		Debug:              os.Getenv("CORS_DEBUG") == "true",
	})
	handler := c.Handler(mux)

	// --- Start Server ---
	log.Printf("Server starting on port %s", appConfig.Port)
	log.Printf("Allowed CORS origin: %s", appConfig.AllowedOrigin)

	server := &http.Server{
		Addr:         ":" + appConfig.Port,
		Handler:      handler,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Could not listen on %s: %v\n", appConfig.Port, err)
	}
}

// Loads configuration from environment variables with defaults
func loadConfig() config {
	cfg := config{
		ProjectID:     getEnv("DIALOGFLOW_PROJECT_ID", ""),
		LocationID:    getEnv("DIALOGFLOW_LOCATION_ID", ""), // e.g., "us-central1"
		AllowedOrigin: getEnv("ALLOWED_ORIGIN", "*"),
		Port:          getEnv("PORT", "8080"),
		// Optional: Provide a default agent ID via env var if needed
		DefaultAgentID: getEnv("DEFAULT_DIALOGFLOW_AGENT_ID", "1891c50e-e0b6-44cc-b1f0-cc7d04bc73b2"), // Example default from JS
	}
	if cfg.ProjectID == "" || cfg.LocationID == "" {
		log.Fatal("Error: DIALOGFLOW_PROJECT_ID and DIALOGFLOW_LOCATION_ID environment variables must be set.")
	}
	return cfg
}

// Helper to get environment variable or return default
func getEnv(key, fallback string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return fallback
}

// Simple health check endpoint
func healthCheckHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

// Handles requests to the /api/dialogflow/detectIntent endpoint for CX
func detectIntentHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// --- Decode Request Body ---
	var req DetectIntentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Printf("Error decoding request body: %v", err)
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	// --- Input Validation ---
	// CX requires message, agentId, and sessionId
	agentID := req.AgentID
	if agentID == "" {
		agentID = appConfig.DefaultAgentID // Use default if not provided
	}
	sessionID := req.SessionID // Use session ID from request
	if req.Message == "" || agentID == "" || sessionID == "" {
		log.Printf("Validation Error: Missing message, agentId, or sessionId. AgentID used: %s, SessionID: %s", agentID, sessionID)
		http.Error(w, "Missing required fields: message, agentId, sessionId", http.StatusBadRequest)
		return
	}

	// --- Language Code ---
	langCode := req.LanguageCode
	if langCode == "" {
		langCode = "en" // Default language code from JS example
	}

	// --- Construct Dialogflow CX Request ---
	// ** UPDATED Session Path construction for CX **
	// Format: projects/<Project ID>/locations/<Location ID>/agents/<Agent ID>/sessions/<Session ID>
	sessionPath := fmt.Sprintf("projects/%s/locations/%s/agents/%s/sessions/%s",
		appConfig.ProjectID, appConfig.LocationID, agentID, sessionID)
	// The CX client library also has a helper:
	// sessionPath = sessionsClient.SessionPath(appConfig.ProjectID, appConfig.LocationID, agentID, sessionID)

	log.Printf("Sending CX request to Dialogflow: Path=%s, Lang=%s, Message=%q",
		sessionPath, langCode, req.Message)

	// ** UPDATED Request struct for CX **
	dialogflowRequest := &cxpb.DetectIntentRequest{
		Session: sessionPath,
		QueryInput: &cxpb.QueryInput{
			Input: &cxpb.QueryInput_Text{
				Text: &cxpb.TextInput{
					Text: req.Message,
				},
			},
			LanguageCode: langCode,
		},
		// Optional: Add QueryParams if needed for CX
		// QueryParams: &cxpb.QueryParameters{...},
	}

	// --- Send Request to Dialogflow CX ---
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	// ** UPDATED API call for CX **
	response, err := sessionsClient.DetectIntent(ctx, dialogflowRequest)
	if err != nil {
		log.Printf("Error calling Dialogflow CX DetectIntent: %v", err)
		http.Error(w, fmt.Sprintf("Dialogflow CX API error: %v", err), http.StatusInternalServerError)
		return
	}

	// --- Process and Return Response (Simplified like JS example) ---
	queryResult := response.GetQueryResult()
	if queryResult == nil {
		log.Printf("Error: Dialogflow CX response missing query result.")
		http.Error(w, "Dialogflow CX returned empty result", http.StatusInternalServerError)
		return
	}

	responseText := ""
	// Extract the first text response message, similar to the JS example
	responseMessages := queryResult.GetResponseMessages()
	if len(responseMessages) > 0 {
		// Check if the first message is a text message
		if textMessage := responseMessages[0].GetText(); textMessage != nil {
			// Get the list of texts (usually just one)
			texts := textMessage.GetText()
			if len(texts) > 0 {
				responseText = texts[0]
			}
		}
	}

	if responseText == "" {
		log.Printf("Warning: No text response found in Dialogflow CX result.")
		// You might want to return a default message or handle other response types
	}

	log.Printf("Received response from Dialogflow CX: Fulfillment=%q", responseText)

	// ** UPDATED Response format **
	apiResponse := DetectIntentResponse{
		Text:      responseText,
		SessionID: sessionID, // Return session ID used
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(apiResponse); err != nil {
		log.Printf("Error encoding response: %v", err)
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}