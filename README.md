# Picolo - Dialogflow Proxy Service

## Overview

Picolo is a lightweight, production-ready backend proxy service written in Go. It securely forwards requests from a client application (like a web-based chat widget) to Google Cloud Dialogflow's `detectIntent` API (V2).

It's designed to be easily deployed as a containerized application on Google Cloud Run, leveraging Application Default Credentials (ADC) for authentication and reading configuration from environment variables.

This service acts as a secure intermediary, preventing the need to expose your Dialogflow credentials directly in the frontend application.

## Features

* **Secure Proxy:** Handles communication with Dialogflow API, keeping credentials server-side.
* **Dialogflow V2 API:** Interacts with the `detectIntent` endpoint.
* **Stateless Design:** Suitable for serverless environments like Cloud Run.
* **Configurable:** Uses environment variables for project ID, location, CORS origins, and port.
* **Session Management:** Supports client-provided session IDs or generates new ones per request.
* **Cloud Run Ready:** Includes Dockerfile and `cloudbuild.yaml` for easy deployment.
* **CORS Handling:** Configurable Cross-Origin Resource Sharing support.
* **Health Check:** Basic `/healthz` endpoint.

## Prerequisites

* **Go:** Version 1.21 or later (for local development/building).
* **Docker:** To build the container image.
* **Google Cloud SDK (`gcloud`):** For deployment and interacting with GCP.
* **Google Cloud Project:** With billing enabled.
* **Dialogflow Agent:** A configured Dialogflow ES Agent (V2 API). Note the **Project ID** and **Location ID** (e.g., `us-central1`, `global`).
* **Enabled GCP APIs:**
    * Cloud Build API
    * Cloud Run Admin API
    * Artifact Registry API
    * Dialogflow API
* **Artifact Registry Repository:** A Docker repository in your GCP project (e.g., `cloudrun-repo`).
* **Service Account Credentials (for local development):** A service account key file with the "Dialogflow API Client" role. Download the JSON key file.

## Configuration

The service is configured via environment variables:

| Variable                            | Description                                                                                                | Required | Default      | Example                               |
| :---------------------------------- | :--------------------------------------------------------------------------------------------------------- | :------- | :----------- | :------------------------------------ |
| `DIALOGFLOW_PROJECT_ID`             | Your Google Cloud Project ID where the Dialogflow agent resides.                                           | Yes      | -            | `my-gcp-project-id`                 |
| `DIALOGFLOW_LOCATION_ID`            | The location ID of your Dialogflow agent (e.g., `us-central1`, `global`).                                  | Yes      | -            | `us-central1`                         |
| `ALLOWED_ORIGIN`                    | The URL of your frontend application allowed to make requests (CORS). Use `*` for development only.        | No       | `*`          | `https://my-chat-app.com`             |
| `PORT`                              | The port the service will listen on inside the container. Cloud Run sets this automatically.               | No       | `8080`       | `8080`                                |
| `GOOGLE_APPLICATION_CREDENTIALS`    | **(Local Dev Only)** Path to your service account key file for local testing. Not needed when deployed on GCP. | No       | -            | `/path/to/your/keyfile.json`        |
| `CORS_DEBUG`                        | Set to `true` to enable verbose CORS debugging logs.                                                       | No       | `false`      | `true`                                |

## Running Locally

1.  **Clone the repository:** (Assuming you have the code)
    ```bash
    # git clone ...
    # cd picolo
    ```
2.  **Set Environment Variables:**
    ```bash
    export DIALOGFLOW_PROJECT_ID="your-gcp-project-id"
    export DIALOGFLOW_LOCATION_ID="your-agent-location" # e.g., us-central1
    export GOOGLE_APPLICATION_CREDENTIALS="/path/to/your/keyfile.json"
    export ALLOWED_ORIGIN="http://localhost:4200" # Or your local frontend origin
    export PORT="8080" # Optional, defaults to 8080
    ```
3.  **Run the application:**
    ```bash
    go run main.go
    ```
    The server will start on `localhost:8080` (or the specified `PORT`).

## Building the Docker Image

You can build the container image using Docker directly:

```bash
docker build -t picolo-dialogflow-proxy .
```

Or, use Google Cloud Build (which also tags it for Artifact Registry):