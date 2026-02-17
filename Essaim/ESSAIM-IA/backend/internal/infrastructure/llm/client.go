package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/google/generative-ai-go/genai"
	"google.golang.org/api/option"
)

// DefaultRPM is the default rate limit (requests per minute) for the API.
const DefaultRPM = 60

// Client wraps the Gemini API with a Token Bucket rate limiter.
type Client struct {
	genaiClient *genai.Client
	model       *genai.GenerativeModel
	limiter     *TokenBucket
}

// TokenBucket implements a rate limiter per Architecture §7.
type TokenBucket struct {
	mu         sync.Mutex
	tokens     float64
	maxTokens  float64
	refillRate float64 // tokens per second
	lastRefill time.Time
}

// NewTokenBucket creates a new rate limiter for the given RPM.
func NewTokenBucket(rpm int) *TokenBucket {
	rate := float64(rpm) / 60.0 // tokens per second
	return &TokenBucket{
		tokens:     float64(rpm),
		maxTokens:  float64(rpm),
		refillRate: rate,
		lastRefill: time.Now(),
	}
}

// Wait blocks until a token is available, using exponential backoff if needed.
func (tb *TokenBucket) Wait(ctx context.Context) error {
	backoff := 100 * time.Millisecond
	maxBackoff := 10 * time.Second

	for {
		tb.mu.Lock()
		tb.refill()
		if tb.tokens >= 1 {
			tb.tokens--
			tb.mu.Unlock()
			return nil
		}
		tb.mu.Unlock()

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoff):
			// Exponential backoff
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
		}
	}
}

func (tb *TokenBucket) refill() {
	now := time.Now()
	elapsed := now.Sub(tb.lastRefill).Seconds()
	tb.tokens += elapsed * tb.refillRate
	if tb.tokens > tb.maxTokens {
		tb.tokens = tb.maxTokens
	}
	tb.lastRefill = now
}

// NewClient creates a new LLM client connected to Google Gemini.
func NewClient(ctx context.Context, apiKey string, modelName string) (*Client, error) {
	genaiClient, err := genai.NewClient(ctx, option.WithAPIKey(apiKey))
	if err != nil {
		return nil, fmt.Errorf("failed to create Gemini client: %w", err)
	}

	model := genaiClient.GenerativeModel(modelName)
	// Force JSON output per Architecture Rule #1 (SILENCE RADIO)
	model.ResponseMIMEType = "application/json"

	log.Printf("[LLM] Connected to Gemini model: %s", modelName)

	return &Client{
		genaiClient: genaiClient,
		model:       model,
		limiter:     NewTokenBucket(DefaultRPM),
	}, nil
}

// LLMResponse holds the parsed response from the LLM.
type LLMResponse struct {
	RawJSON    string
	TokensUsed int32
	LatencyMs  int64
}

// Call sends a prompt to the LLM with rate limiting. Returns the JSON response.
// This is NON-BLOCKING compliant: must be called from a worker goroutine (Rule #5).
func (c *Client) Call(ctx context.Context, systemPrompt string, userMessage string) (*LLMResponse, error) {
	// Wait for rate limiter
	if err := c.limiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("rate limiter: %w", err)
	}

	start := time.Now()

	// Set system instruction
	c.model.SystemInstruction = genai.NewUserContent(genai.Text(systemPrompt))

	resp, err := c.model.GenerateContent(ctx, genai.Text(userMessage))
	if err != nil {
		return nil, fmt.Errorf("Gemini API error: %w", err)
	}

	latency := time.Since(start).Milliseconds()

	// Extract text content from response
	if len(resp.Candidates) == 0 || len(resp.Candidates[0].Content.Parts) == 0 {
		return nil, fmt.Errorf("empty response from Gemini")
	}

	text, ok := resp.Candidates[0].Content.Parts[0].(genai.Text)
	if !ok {
		return nil, fmt.Errorf("unexpected response part type from Gemini")
	}

	// Validate that the response is valid JSON (Rule #1)
	var jsonCheck json.RawMessage
	if err := json.Unmarshal([]byte(text), &jsonCheck); err != nil {
		return nil, fmt.Errorf("LLM response is not valid JSON: %w (raw: %s)", err, string(text))
	}

	tokensUsed := int32(0)
	if resp.UsageMetadata != nil {
		tokensUsed = resp.UsageMetadata.TotalTokenCount
	}

	return &LLMResponse{
		RawJSON:    string(text),
		TokensUsed: tokensUsed,
		LatencyMs:  latency,
	}, nil
}

// Close releases the Gemini client resources.
func (c *Client) Close() {
	c.genaiClient.Close()
}
