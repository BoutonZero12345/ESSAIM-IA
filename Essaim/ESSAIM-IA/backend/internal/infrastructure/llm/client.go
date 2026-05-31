package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/google/generative-ai-go/genai"
	"google.golang.org/api/option"
)

// DefaultRPM is the default rate limit (requests per minute) for the API.
// Level 1 Quota: 2000 RPM
const DefaultRPM = 2000

// DefaultTPM is the default rate limit (tokens per minute) for the API.
// Level 1 Quota: 4,000,000 TPM
const DefaultTPM = 4000000

// Client wraps the Gemini API with a Token Bucket rate limiter.
type Client struct {
	genaiClient *genai.Client
	model       *genai.GenerativeModel
	rpmLimiter  *TokenBucket
	tpmLimiter  *TokenBucket
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

	initialTokens := float64(rpm)
	// For highly restricted endpoints (Free Tier Gemini), prevent initial burst hitting API limits instantly
	if rpm <= 60 {
		initialTokens = 1.0
	}

	return &TokenBucket{
		tokens:     initialTokens,
		maxTokens:  float64(rpm),
		refillRate: rate,
		lastRefill: time.Now(),
	}
}

// Wait blocks until 1 token is available.
func (tb *TokenBucket) Wait(ctx context.Context) error {
	return tb.WaitN(ctx, 1)
}

// WaitN blocks until n tokens are available, using exponential backoff if needed.
func (tb *TokenBucket) WaitN(ctx context.Context, n float64) error {
	backoff := 100 * time.Millisecond
	maxBackoff := 10 * time.Second

	for {
		tb.mu.Lock()
		tb.refill()
		if tb.tokens >= n {
			tb.tokens -= n
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
		rpmLimiter:  NewTokenBucket(DefaultRPM),
		tpmLimiter:  NewTokenBucket(DefaultTPM),
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
	// Wait for RPM rate limiter
	if err := c.rpmLimiter.WaitN(ctx, 1); err != nil {
		return nil, fmt.Errorf("rpm rate limiter: %w", err)
	}

	// Estimate TPM based on input char count + buffer (3 chars ~ 1 token, plus 1000 for response output)
	estimatedTokens := float64((len(systemPrompt)+len(userMessage))/3 + 1000)
	if estimatedTokens > DefaultTPM {
		estimatedTokens = DefaultTPM
	}

	// Wait for TPM rate limiter
	if err := c.tpmLimiter.WaitN(ctx, estimatedTokens); err != nil {
		return nil, fmt.Errorf("tpm rate limiter: %w", err)
	}

	start := time.Now()

	if os.Getenv("MOCK_LLM") == "true" {
		// Simulate network latency & Jitter limit
		jitter := time.Duration(100+rand.Intn(50)) * time.Millisecond
		time.Sleep(jitter)

		var mockRaw string

		if strings.Contains(systemPrompt, "TON RÔLE : ARCHITECT") {
			mockRaw = `{"action":"SPAWN", "payload": {"subtasks": [
				{"role": "DIRECTOR", "task_description": "Dirige l'Acte 1", "budget_fraction": 0.3},
				{"role": "DIRECTOR", "task_description": "Dirige l'Acte 2", "budget_fraction": 0.3},
				{"role": "DIRECTOR", "task_description": "Dirige l'Acte 3", "budget_fraction": 0.3}
			]}}`
		} else if strings.Contains(systemPrompt, "TON RÔLE : DIRECTOR") {
			mockRaw = `{"action":"SPAWN", "payload": {"subtasks": [
				{"role": "MANAGER", "task_description": "Ch 1", "budget_fraction": 0.15},
				{"role": "MANAGER", "task_description": "Ch 2", "budget_fraction": 0.15},
				{"role": "MANAGER", "task_description": "Ch 3", "budget_fraction": 0.15},
				{"role": "MANAGER", "task_description": "Ch 4", "budget_fraction": 0.15},
				{"role": "MANAGER", "task_description": "Ch 5", "budget_fraction": 0.15},
				{"role": "MANAGER", "task_description": "Ch 6", "budget_fraction": 0.15}
			]}}`
		} else if strings.Contains(systemPrompt, "TON RÔLE : MANAGER") {
			mockRaw = `{"action":"SPAWN", "payload": {"subtasks": [
				{"role": "WORKER", "task_description": "Sc 1", "budget_fraction": 0.1},
				{"role": "WORKER", "task_description": "Sc 2", "budget_fraction": 0.1},
				{"role": "WORKER", "task_description": "Sc 3", "budget_fraction": 0.1},
				{"role": "WORKER", "task_description": "Sc 4", "budget_fraction": 0.1},
				{"role": "WORKER", "task_description": "Sc 5", "budget_fraction": 0.1},
				{"role": "WORKER", "task_description": "Sc 6", "budget_fraction": 0.1},
				{"role": "WORKER", "task_description": "Sc 7", "budget_fraction": 0.1},
				{"role": "WORKER", "task_description": "Sc 8", "budget_fraction": 0.1},
				{"role": "WORKER", "task_description": "Sc 9", "budget_fraction": 0.1}
			]}}`
		} else if strings.Contains(systemPrompt, "TON RÔLE : RESUMEUR") {
			mockRaw = `{"action":"REPORT", "payload": {"result_summary": "Résumé de la strate consolidé avec succès via Simulation."}}`
		} else {
			mockRaw = `{"action":"REPORT", "payload": {"result": "Voici la scène rédigée avec passion (Générée par Simulation Locale)."}}`
		}

		return &LLMResponse{
			RawJSON:    mockRaw,
			TokensUsed: 50,
			LatencyMs:  time.Since(start).Milliseconds(),
		}, nil
	}

	// Set system instruction
	c.model.SystemInstruction = genai.NewUserContent(genai.Text(systemPrompt))

	// Retry with backoff on 429 rate limit errors or timeouts
	var resp *genai.GenerateContentResponse
	var err error
	maxRetries := 100000 // Infinite resilience waiting for daily quota limit resets
	retryBackoff := 10 * time.Second

	for attempt := 0; attempt <= maxRetries; attempt++ {
		// Anti-burst Jitter (100ms - 150ms) to ensure concurrent Goroutines stagger their API hits
		jitter := time.Duration(100+rand.Intn(50)) * time.Millisecond
		time.Sleep(jitter)

		// Wrap with a strict 4-minute timeout per call to prevent indefinite hangs
		callCtx, cancel := context.WithTimeout(ctx, 4*time.Minute)
		resp, err = c.model.GenerateContent(callCtx, genai.Text(userMessage))
		cancel()

		if err == nil {
			break
		}

		// Check if it's a rate limit error (429) or a deadline exceeded (timeout)
		errStr := err.Error()
		isRetryable := false
		for _, keyword := range []string{"429", "rate", "quota", "RESOURCE_EXHAUSTED", "deadline exceeded", "timeout"} {
			if len(errStr) > 0 && strings.Contains(errStr, keyword) {
				isRetryable = true
				break
			}
		}

		if !isRetryable || attempt >= maxRetries {
			return nil, fmt.Errorf("Gemini API error: %w", err)
		}

		log.Printf("[LLM] ⚠ API error/timeout (%s), retrying in %v (attempt %d/%d)", errStr, retryBackoff, attempt+1, maxRetries)
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(retryBackoff):
		}

		// Exponential backoff, capped at 5 minutes to wait out daily resets
		retryBackoff *= 2
		if retryBackoff > 5*time.Minute {
			retryBackoff = 5 * time.Minute
		}
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
