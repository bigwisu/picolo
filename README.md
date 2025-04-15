# Picolo - Dialogflow CX Proxy

A minimalist Go backend service acting as a secure proxy for Google Cloud Dialogflow CX agents, designed for Cloud Run deployment.

## Prerequisites

* Go (1.23+)
* Docker
* Google Cloud SDK (`gcloud`)

## Configuration

Configure via environment variables:

* `DIALOGFLOW_PROJECT_ID`: Your GCP Project ID. (Required)
* `DIALOGFLOW_LOCATION_ID`: Your Dialogflow CX Agent Location (e.g., `us-central1`). (Required)
* `DEFAULT_DIALOGFLOW_AGENT_ID`: Default Dialogflow CX Agent ID if not sent in request. (Optional)
* `ALLOWED_ORIGIN`: CORS allowed origin (e.g., `http://localhost:4200`, `*` for dev). (Default: `*`)
* `PORT`: Port for the service. (Default: `8080`)
* `GOOGLE_APPLICATION_CREDENTIALS`: Path to service account key JSON (for local development only).

## Running Locally

1.  Set environment variables:
    ```bash
    export DIALOGFLOW_PROJECT_ID="your-project-id"
    export DIALOGFLOW_LOCATION_ID="your-location-id"
    export GOOGLE_APPLICATION_CREDENTIALS="/path/to/keyfile.json"
    # Optional: export DEFAULT_DIALOGFLOW_AGENT_ID="your-agent-id"
    ```
2.  Run `go mod tidy` to install dependencies.
3.  Run the service:
    ```bash
    go run main.go
    ```

## Deployment (Cloud Run)

1.  Ensure GCP APIs are enabled (Cloud Build, Cloud Run, Artifact Registry, Dialogflow).
2.  Configure substitutions in `cloudbuild.yaml` (especially `_ALLOWED_ORIGIN`, `_DIALOGFLOW_PROJECT_ID`, `_DIALOGFLOW_LOCATION_ID`).
3.  Deploy using Cloud Build:
    ```bash
    gcloud builds submit --config cloudbuild.yaml .
    ```

## API Endpoint

* **`POST /api/dialogflow/detectIntent`**
    * **Body (JSON):** Requires `message` (string), `agentId` (string, optional if default set), `sessionId` (string). `languageCode` (string) is optional.
    * **Response (JSON):** Contains `text` (string) with the bot's reply and `sessionId` (string).
