package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	dialogflow "cloud.google.com/go/dialogflow/apiv2"
	"github.com/google/uuid"
	"github.com/rs/cors" // For CORS handling
	dialogflowpb "google.golang.org/genproto/googleapis/cloud/dialogflow/v2"
	// ADC is implicitly used when no explicit credentials are provided
	// "google.golang.org/api/option"
)

// Configuration struct to hold environment variables
type config struct {
	ProjectID     string
	LocationID    string
	AllowedOrigin string
	Port          string
}

// Request struct matching the expected JSON body from the client
type DetectIntentRequest struct {
	Message      string `json:"message"`
	AgentID      string `json:"agentId"` // Note: AgentID isn't directly used in V2 DetectIntent path, but kept for consistency with Node example
	SessionID    string `json:"sessionId"`    // Optional session ID from client
	LanguageCode string `json:"languageCode"` // Optional language code
}

// Response struct sent back to the client
type DetectIntentResponse struct {
	FulfillmentText     string                              `json:"fulfillmentText"`
	FulfillmentMessages []*dialogflowpb.ResponseMessage `json:"fulfillmentMessages"`
	Intent              string                              `json:"intent"`
	Parameters          *dialogflowpb.Struct                `json:"parameters"`
	SessionID           string                              `json:"sessionId"` // The session ID used for the request
}

var (
	appConfig     config
	sessionsClient *dialogflow.SessionsClient
)

func main() {
	var err error
	ctx := context.Background()

	// --- Load Configuration from Environment Variables ---
	appConfig = loadConfig()

	// --- Initialize Dialogflow Client ---
	// When running on Google Cloud (like Cloud Run), ADC are used automatically.
	// No need to explicitly provide credentials file path in most cases.
	// If running locally and GOOGLE_APPLICATION_CREDENTIALS is set, it will use that.
	// Set the correct API endpoint if not global (common for newer agents)
	// endpoint := fmt.Sprintf("%s-dialogflow.googleapis.com:443", appConfig.LocationID)
	// sessionsClient, err = dialogflow.NewSessionsClient(ctx, option.WithEndpoint(endpoint))
	sessionsClient, err = dialogflow.NewSessionsClient(ctx) // Simpler initialization often works
	if err != nil {
		log.Fatalf("Failed to create Dialogflow sessions client: %v", err)
	}
	defer sessionsClient.Close()

	log.Printf("Dialogflow client initialized for project %s, location %s", appConfig.ProjectID, appConfig.LocationID)

	// --- Setup HTTP Server & Routing ---
	mux := http.NewServeMux()
	mux.HandleFunc("/api/dialogflow/detectIntent", detectIntentHandler)
	mux.HandleFunc("/healthz", healthCheckHandler) // Basic health check

	// --- CORS Configuration ---
	c := cors.New(cors.Options{
		AllowedOrigins: []string{appConfig.AllowedOrigin}, // Use configured origin
		AllowedMethods: []string{"POST", "OPTIONS"},
		AllowedHeaders: []string{"Content-Type", "Authorization"}, // Adjust as needed
		// AllowCredentials: true, // Set to true if you need cookies/auth headers
		OptionsPassthrough: false, // Let CORS handle OPTIONS
		Debug:              os.Getenv("CORS_DEBUG") == "true", // Enable debug logging if needed
	})
	handler := c.Handler(mux) // Wrap mux with CORS middleware

	// --- Start Server ---
	log.Printf("Server starting on port %s", appConfig.Port)
	log.Printf("Allowed CORS origin: %s", appConfig.AllowedOrigin)

	server := &http.Server{
		Addr:         ":" + appConfig.Port,
		Handler:      handler, // Use the CORS-wrapped handler
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
		ProjectID:     getEnv("DIALOGFLOW_PROJECT_ID", ""), // Must be set
		LocationID:    getEnv("DIALOGFLOW_LOCATION_ID", ""), // Must be set
		AllowedOrigin: getEnv("ALLOWED_ORIGIN", "*"),       // Default to allow all for dev, restrict in prod
		Port:          getEnv("PORT", "8080"),            // Default for Cloud Run
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

// Handles requests to the /api/dialogflow/detectIntent endpoint
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
	if req.Message == "" || req.AgentID == "" {
		log.Printf("Validation Error: Missing message or agentId. AgentID received: %s", req.AgentID)
		http.Error(w, "Missing required fields: message and agentId", http.StatusBadRequest)
		return
	}

	// --- Session Management ---
	sessionID := req.SessionID
	if sessionID == "" {
		sessionID = uuid.NewString() // Generate new session ID if client didn't provide one
	}

	// --- Language Code ---
	langCode := req.LanguageCode
	if langCode == "" {
		langCode = "en-US" // Default language code if not provided
	}

	// --- Construct Dialogflow Request ---
	// Format: projects/<Project ID>/locations/<Location ID>/agent/sessions/<Session ID>
	sessionPath := fmt.Sprintf("projects/%s/locations/%s/agent/sessions/%s", appConfig.ProjectID, appConfig.LocationID, sessionID)

	// Note: AgentID from the request is logged but not part of the session path itself in V2 API.
	// It might be used if passed via queryParams or for other backend logic.
	log.Printf("Sending request to Dialogflow: Project=%s, Location=%s, Session=%s, AgentID=%s, Lang=%s, Message=%q",
		appConfig.ProjectID, appConfig.LocationID, sessionID, req.AgentID, langCode, req.Message)

	dialogflowRequest := &dialogflowpb.DetectIntentRequest{
		Session: sessionPath,
		QueryInput: &dialogflowpb.QueryInput{
			Input: &dialogflowpb.QueryInput_Text{
				Text: &dialogflowpb.TextInput{
					Text:         req.Message,
					LanguageCode: langCode,
				},
			},
		},
		// Optional: Add QueryParams if needed
		// QueryParams: &dialogflowpb.QueryParameters{...},
	}

	// --- Send Request to Dialogflow ---
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second) // Add timeout
	defer cancel()

	response, err := sessionsClient.DetectIntent(ctx, dialogflowRequest)
	if err != nil {
		log.Printf("Error calling Dialogflow DetectIntent: %v", err)
		http.Error(w, fmt.Sprintf("Dialogflow API error: %v", err), http.StatusInternalServerError)
		return
	}

	// --- Process and Return Response ---
	result := response.GetQueryResult()
	if result == nil {
		log.Printf("Error: Dialogflow response missing query result.")
		http.Error(w, "Dialogflow returned empty result", http.StatusInternalServerError)
		return
	}

	log.Printf("Received response from Dialogflow: Intent=%s, Fulfillment=%q", result.Intent.GetDisplayName(), result.FulfillmentText)

	apiResponse := DetectIntentResponse{
		FulfillmentText:     result.FulfillmentText,
		FulfillmentMessages: result.FulfillmentMessages,
		Intent:              result.Intent.GetDisplayName(),
		Parameters:          result.Parameters,
		SessionID:           sessionID, // Return the session ID used
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(apiResponse); err != nil {
		log.Printf("Error encoding response: %v", err)
		// Attempt to send a plain text error if JSON encoding fails
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}