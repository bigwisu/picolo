# Guide: Logging Dialogflow Interactions Directly to BigQuery

This guide details how to modify your Go Dialogflow proxy service to log interaction details directly to a Google BigQuery table.

**Goal:** Capture key information about each `detectIntent` call (user input, detected intent, parameters, response, etc.) and store it in BigQuery for analysis.

**Approach:** We will add code to the `detectIntentHandler` to asynchronously send a structured log entry to BigQuery after successfully processing a Dialogflow request.

## Prerequisites

*   **Google Cloud Project:** You need an active Google Cloud project.
*   **Billing Enabled:** BigQuery usage (beyond the free tier) requires billing to be enabled for your project.
*   **APIs Enabled:** Ensure the **BigQuery API** and **Dialogflow API** are enabled in your Google Cloud project. You can enable them via the Cloud Console.
*   **Service Account Permissions:** The service account running your Go application (e.g., the default Cloud Run service account) needs permissions to write data to BigQuery. The `BigQuery Data Editor` role (`roles/bigquery.dataEditor`) on the target dataset/table is typically sufficient.

## Step 1: Define BigQuery Table Schema

Decide on the structure for your log data. Here's a recommended schema:

| Field Name         | Type      | Mode     | Description                                      |
| :----------------- | :-------- | :------- | :----------------------------------------------- |
| `timestamp`        | TIMESTAMP | REQUIRED | When the interaction was logged by the service.  |
| `session_id`       | STRING    | NULLABLE | The Dialogflow session ID.                       |
| `agent_id`         | STRING    | NULLABLE | The Dialogflow Agent ID used for the request.    |
| `user_input`       | STRING    | NULLABLE | The raw text message sent by the user.           |
| `detected_intent`  | STRING    | NULLABLE | The display name of the matched intent.          |
| `intent_confidence`| FLOAT     | NULLABLE | The confidence score of the intent match.        |
| `parameters`       | STRING    | NULLABLE | Extracted parameters, stored as a JSON string.   |
| `fulfillment_text` | STRING    | NULLABLE | The text response sent back to the user.         |
| `language_code`    | STRING    | NULLABLE | The language code used for the request.          |
| `project_id`       | STRING    | NULLABLE | Your Google Cloud Project ID.                    |
| `location_id`      | STRING    | NULLABLE | The Dialogflow agent's location ID (region).     |

Action: Create a dataset (e.g., dialogflow_logs) and a table (e.g., intent_interactions) in BigQuery using the Cloud Console or the bq command-line tool with the schema above.

Step 2: Install BigQuery Go Client Library
If you haven't already, add the BigQuery client library to your project:

bash
go get cloud.google.com/go/bigquery
go mod tidy
Commit the changes to your go.mod and go.sum files.

Step 3: Update Go Code
Modify your main.go file to include the BigQuery logging logic.

Apply the following diff:

```diff
--- a/Users/wisu/Source/picolo/main.go
+++ b/Users/wisu/Source/picolo/main.go
@@ -10,6 +10,7 @@
 	"os"
 	"time"
 
+	"cloud.google.com/go/bigquery"
 	cx "cloud.google.com/go/dialogflow/cx/apiv3"
 	"github.com/rs/cors"
 	cxpb "google.golang.org/genproto/googleapis/cloud/dialogflow/cx/v3"
@@ -23,6 +24,9 @@
 	LocationID    string
 	AllowedOrigin string
 	Port          string
+	// BigQuery specific config
+	BigQueryDatasetID string
+	BigQueryTableID   string
 	DefaultAgentID string
 }
 
@@ -42,9 +46,24 @@
 	Parameters map[string]interface{} `json:"parameters"` // Parameters extracted by Dialogflow
 }
 
+// Struct for logging data to BigQuery - must match your table schema
+type IntentLogEntry struct {
+	Timestamp        time.Time `bigquery:"timestamp"`
+	SessionID        string    `bigquery:"session_id"`
+	AgentID          string    `bigquery:"agent_id"`
+	UserInput        string    `bigquery:"user_input"`
+	DetectedIntent   string    `bigquery:"detected_intent"`
+	IntentConfidence float32   `bigquery:"intent_confidence"`
+	Parameters       string    `bigquery:"parameters"` // Store as JSON string
+	FulfillmentText  string    `bigquery:"fulfillment_text"`
+	LanguageCode     string    `bigquery:"language_code"`
+	ProjectID        string    `bigquery:"project_id"`
+	LocationID       string    `bigquery:"location_id"`
+}
+
 var (
 	appConfig config
 	sessionsClient *cx.SessionsClient
+	bqClient       *bigquery.Client // BigQuery client instance
 )
 
 func main() {
@@ -71,6 +90,20 @@
 
 	log.Printf("Dialogflow CX client initialized for project %s, location %s", appConfig.ProjectID, appConfig.LocationID)
 
+	// --- Initialize BigQuery Client (if configured) ---
+	if appConfig.BigQueryDatasetID != "" && appConfig.BigQueryTableID != "" {
+		var bqErr error
+		// Use the same context used for other initializations
+		bqClient, bqErr = bigquery.NewClient(ctx, appConfig.ProjectID)
+		if bqErr != nil {
+			// Log warning instead of fatal, as logging might be optional or recoverable
+			log.Printf("Warning: Failed to create BigQuery client: %v. Intent logging will be disabled.", bqErr)
+			bqClient = nil // Ensure client is nil if initialization failed
+		} else {
+			log.Printf("BigQuery client initialized for project %s. Logging to %s.%s", appConfig.ProjectID, appConfig.BigQueryDatasetID, appConfig.BigQueryTableID)
+			// Note: Closing the client is typically done on application shutdown.
+			// In Cloud Run, the instance might be shut down abruptly. Deferring close here might not always run.
+			// The client is designed to manage connections efficiently. Explicit close isn't always necessary for short-lived apps.
+		}
+	} else {
+		log.Printf("BigQuery logging is disabled (BIGQUERY_DATASET_ID or BIGQUERY_TABLE_ID not set).")
+	}
 	// --- Setup HTTP Server & Routing ---
 	mux := http.NewServeMux()
 	mux.HandleFunc("/api/dialogflow/detectIntent", detectIntentHandler)
@@ -109,11 +142,13 @@
 		LocationID:    getEnv("DIALOGFLOW_LOCATION_ID", ""),
 		AllowedOrigin: getEnv("ALLOWED_ORIGIN", "*"),
 		Port:          getEnv("PORT", "8080"),
-		DefaultAgentID: getEnv("DEFAULT_DIALOGFLOW_AGENT_ID", "1891c50e-e0b6-44cc-b1f0-cc7d04bc73b2"),
+		BigQueryDatasetID: getEnv("BIGQUERY_DATASET_ID", ""), // e.g., "dialogflow_logs"
+		BigQueryTableID:   getEnv("BIGQUERY_TABLE_ID", ""),   // e.g., "intent_interactions"
+		DefaultAgentID: getEnv("DEFAULT_DIALOGFLOW_AGENT_ID", "1891c50e-e0b6-44cc-b1f0-cc7d04bc73b2"), // Keep this line
 	}
 	if cfg.ProjectID == "" || cfg.LocationID == "" {
 		log.Fatal("Error: DIALOGFLOW_PROJECT_ID and DIALOGFLOW_LOCATION_ID environment variables must be set.")
@@ -192,6 +227,7 @@
 		return
 	}
 
+	var detectedIntentName string
 	responseText := ""
 	// Extract the first text response message, similar to the JS example
 	responseMessages := queryResult.GetResponseMessages()
@@ -207,6 +243,11 @@
 		}
 	}
 
+	// Extract intent details if a match occurred
+	if match := queryResult.GetMatch(); match != nil && match.GetIntent() != nil {
+		detectedIntentName = match.GetIntent().GetDisplayName()
+	}
+
 	if responseText == "" {
 		log.Printf("Warning: No text response found in Dialogflow CX result.")
 	}
@@ -214,13 +255,12 @@
 	// --- Extract Parameters ---
 	var parameters map[string]interface{}
 	if queryResult.GetParameters() != nil {
-		parameters = queryResult.GetParameters().AsMap()
+		// Convert structpb.Struct to map[string]interface{} for easier handling/logging
+		parameters = queryResult.GetParameters().AsMap()
 		log.Printf("Dialogflow CX Parameters: %v", parameters)
 	} else {
 		log.Printf("No parameters found in Dialogflow CX result.")
 	}
-	log.Printf("Received response from Dialogflow CX: Fulfillment=%q", responseText)
-
 	// ** UPDATED Response format **
 	apiResponse := DetectIntentResponse{
 		Text:      responseText,
@@ -228,13 +268,55 @@
 		Parameters: parameters, // Include extracted parameters
 	}
 
+	// --- Asynchronous BigQuery Logging ---
+	// Check if BQ client was initialized successfully
+	if bqClient != nil {
+		// Use the request context initially, but the logging function should detach if needed
+		go logIntentToBigQuery(r.Context(), &req, queryResult, agentID, sessionID, responseText, parameters, detectedIntentName)
+	}
+
 	w.Header().Set("Content-Type", "application/json")
 	w.WriteHeader(http.StatusOK)
 	if err := json.NewEncoder(w).Encode(apiResponse); err != nil {
 		log.Printf("Error encoding response: %v", err)
-		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
+		// Don't try to write another header if Encode fails, just log.
 	}
 }
+
+// logIntentToBigQuery sends interaction data to BigQuery asynchronously.
+func logIntentToBigQuery(ctx context.Context, req *DetectIntentRequest, qr *cxpb.QueryResult, agentID, sessionID, fulfillmentText string, params map[string]interface{}, intentName string) {
+	// Use a background context for the BigQuery operation itself to detach from the incoming request's deadline/cancellation.
+	// This ensures the logging attempt completes even if the client disconnects.
+	logCtx := context.Background()
+
+	inserter := bqClient.Dataset(appConfig.BigQueryDatasetID).Table(appConfig.BigQueryTableID).Inserter()
+
+	// Convert parameters map to JSON string for storage
+	paramsJSON := "{}" // Default to empty JSON object
+	if params != nil {
+		jsonBytes, err := json.Marshal(params)
+		if err == nil {
+			paramsJSON = string(jsonBytes)
+		} else {
+			log.Printf("Warning: Could not marshal parameters to JSON for BigQuery logging: %v", err)
+			// Decide if you want to log the entry anyway with default paramsJSON or skip logging
+		}
+	}
+
+	entry := IntentLogEntry{
+		Timestamp:        time.Now().UTC(), // Use UTC for consistency
+		SessionID:        sessionID,
+		AgentID:          agentID,
+		UserInput:        req.Message,
+		DetectedIntent:   intentName,
+		IntentConfidence: qr.GetMatch().GetConfidence(), // Safely get confidence (returns 0.0 if no match)
+		Parameters:       paramsJSON,
+		FulfillmentText:  fulfillmentText,
+		LanguageCode:     req.LanguageCode,
+		ProjectID:        appConfig.ProjectID,
+		LocationID:       appConfig.LocationID,
+	}
+
+	// Put attempts to insert rows into the table. Errors are returned for failures.
+	// It buffers rows and sends them asynchronously. For explicit batching or sync writes, use other methods.
+	if err := inserter.Put(logCtx, &entry); err != nil {
+		log.Printf("Error inserting intent log into BigQuery: %v", err)
+		// Consider adding more robust error handling here if needed (e.g., retry logic, dead-letter queue)
+	} else {
+		// Optional: Log success, might be too verbose for high traffic
+		// log.Printf("Successfully queued intent log for BigQuery for session %s", sessionID)
+	}
+}
```

## Step 4: Configure Environment Variables
When deploying your service (e.g., to Cloud Run), make sure to set the following environment variables:

* **BIGQUERY_DATASET_ID:** The ID of the BigQuery dataset you created (e.g., dialogflow_logs).
* **BIGQUERY_TABLE_ID:** The ID of the BigQuery table you created (e.g., intent_interactions).
* **DIALOGFLOW_PROJECT_ID:** Your Google Cloud Project ID.
* **DIALOGFLOW_LOCATION_ID:** The location/region of your Dialogflow agent.
* **ALLOWED_ORIGIN:** The allowed origin for CORS.
* **PORT:** The port your service listens on (Cloud Run sets this automatically).
* **DEFAULT_DIALOGFLOW_AGENT_ID:** Your default agent ID.

## Step 5: Deployment and Testing

1.  **Build and Deploy:** Build your Go application into a container and deploy it (e.g., using `gcloud run deploy` or `gcloud builds submit`). Ensure the service account used by your deployment has the necessary BigQuery permissions (see Prerequisites).
2.  **Test:** Send requests to your `/api/dialogflow/detectIntent` endpoint.
3.  **Verify:** Check your BigQuery table in the Cloud Console. You should see new rows appearing shortly after successful requests are processed by your service. Use the "Preview" tab or write SQL queries to inspect the data.

This setup provides a solid foundation for logging your Dialogflow interactions. Remember to monitor BigQuery costs if you anticipate very high volumes, as streaming inserts are charged per GiB of data inserted (though there is a monthly free tier).