package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

// ProviderRequest is the payload sent to the external notification provider.
type ProviderRequest struct {
	To      string `json:"to"`
	Channel string `json:"channel"`
	Content string `json:"content"`
}

// ProviderResponse is the expected 202 response from the external provider.
type ProviderResponse struct {
	MessageID string `json:"messageId"`
	Status    string `json:"status"`
	Timestamp string `json:"timestamp"`
}

// NotificationProvider defines the interface for sending notifications externally.
type NotificationProvider interface {
	Send(ctx context.Context, req ProviderRequest) (*ProviderResponse, error)
}

// WebhookProvider sends notifications to a webhook.site URL.
type WebhookProvider struct {
	webhookURL string
	httpClient *http.Client
}

// NewWebhookProvider creates a new WebhookProvider with the given webhook URL.
func NewWebhookProvider(webhookURL string) *WebhookProvider {
	return &WebhookProvider{
		webhookURL: webhookURL,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// Send POSTs the notification to the webhook URL and expects a 202 response.
func (p *WebhookProvider) Send(ctx context.Context, req ProviderRequest) (*ProviderResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal provider request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.webhookURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create http request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	log.Printf("Sending notification to external provider: to=%s channel=%s", req.To, req.Channel)

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("provider request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read provider response body: %w", err)
	}

	if resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("provider returned unexpected status %d: %s", resp.StatusCode, string(respBody))
	}

	var provResp ProviderResponse
	if err := json.Unmarshal(respBody, &provResp); err != nil {
		log.Printf("Warning: could not parse provider response JSON (status 202 accepted): %v", err)
		return &ProviderResponse{
			MessageID: "",
			Status:    "accepted",
			Timestamp: time.Now().UTC().Format(time.RFC3339),
		}, nil
	}

	log.Printf("External provider accepted notification: messageId=%s status=%s timestamp=%s",
		provResp.MessageID, provResp.Status, provResp.Timestamp)

	return &provResp, nil
}
