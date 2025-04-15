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

	// ** REVERTED Import path for Dialogflow client **
	dialogflow "cloud.google.com/go/dialogflow/apiv2"
	"github.com/google/uuid"
	"github.com/rs/cors" // For CORS handling
	dialogflowpb "google.golang.org/genproto/googleapis/cloud/dialogflow/v2"

	// ** ADDED IMPORT for client options **
	"google.golang.org/api/option"

	structpb "google.golang.org/protobuf/types/known/structpb"
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
	AgentID      string `json:"agentId"`
	SessionID    string `json:"sessionId"`
	LanguageCode string `json:"languageCode"`
}

// Response struct sent back to the client
type DetectIntentResponse struct {
	FulfillmentText     string                           `json:"fulfillmentText"`
	FulfillmentMessages []*dialogflowpb.Intent_Message `json:"fulfillmentMessages"`
	Intent              string                           `json:"intent"`
	Parameters          *structpb.Struct                 `json:"parameters"`
	SessionID           string                           `json:"sessionId"`
}

var (
	appConfig     config
	// ** REVERTED Client Type ** (Matches reverted import)
	sessionsClient *dialogflow.SessionsClient
)

func main() {
	var err error
	ctx := context.Background()

	// --- Load Configuration from Environment Variables ---
	appConfig = loadConfig() // Ensure LocationID is loaded correctly

	// --- Initialize Dialogflow Client ---
	// ** UPDATED Client Initialization to specify regional endpoint **
	// Ensure GOOGLE_APPLICATION_CREDENTIALS is set for local dev, or running on GCP.
	// Construct the regional endpoint string based on the LocationID config
	regionalEndpoint := fmt.Sprintf("%s-dialogflow.googleapis.com:443", appConfig.LocationID)
	log.Printf("Using Dialogflow regional endpoint: %s", regionalEndpoint)

	sessionsClient, err = dialogflow.NewSessionsClient(ctx, option.WithEndpoint(regionalEndpoint))
	if err != nil {
		log.Fatalf("Failed to create Dialogflow sessions client: %v", err)
	}
	defer sessionsClient.Close()

	log.Printf("Dialogflow client initialized for project %s, location %s", appConfig.ProjectID, appConfig.LocationID)

	// --- Setup HTTP Server & Routing ---
	mux := http.NewServeMux()
	mux.HandleFunc("/api/dialogflow/detectIntent", detectIntentHandler)
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
		LocationID:    getEnv("DIALOGFLOW_LOCATION_ID", ""), // Make sure this is set correctly (e.g., "us-central1")
		AllowedOrigin: getEnv("ALLOWED_ORIGIN", "*"),
		Port:          getEnv("PORT", "8080"),
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
		sessionID = uuid.NewString()
	}

	// --- Language Code ---
	langCode := req.LanguageCode
	if langCode == "" {
		langCode = "en-US" // Default language code
	}

	// --- Construct Dialogflow Request ---
	// Session path format is correct for regionalized agents when client uses correct endpoint
	sessionPath := fmt.Sprintf("projects/%s/locations/%s/agent/sessions/%s", appConfig.ProjectID, appConfig.LocationID, sessionID)

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
	}

	// --- Send Request to Dialogflow ---
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	// The sessionsClient is now configured with the regional endpoint
	response, err := sessionsClient.DetectIntent(ctx, dialogflowRequest)
	if err != nil {
		// Log the specific gRPC error if possible
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

	log.Printf("Received response from Dialogflow: Intent=%s, Fulfillment=%q", result.GetIntent().GetDisplayName(), result.FulfillmentText)

	apiResponse := DetectIntentResponse{
		FulfillmentText:     result.FulfillmentText,
		FulfillmentMessages: result.GetFulfillmentMessages(),
		Intent:              result.GetIntent().GetDisplayName(),
		Parameters:          result.GetParameters(),
		SessionID:           sessionID,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(apiResponse); err != nil {
		log.Printf("Error encoding response: %v", err)
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}