package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"essaim-backend/internal/core/economy"
	"essaim-backend/internal/core/journal"
	"essaim-backend/internal/core/lifecycle"
	"essaim-backend/internal/core/orchestrator"
	"essaim-backend/internal/domain/agent"
	"essaim-backend/internal/domain/graph"
	"essaim-backend/internal/domain/message"
	"essaim-backend/internal/infrastructure/llm"
	"essaim-backend/internal/infrastructure/persistence"
	ws "essaim-backend/internal/infrastructure/websocket"
	"essaim-backend/internal/utils"

	"github.com/google/uuid"
)

// loadEnvFile reads a .env file and sets environment variables.
// It walks up from the backend dir to find the .env at the project root.
func loadEnvFile() {
	// Try multiple paths to find the .env file
	paths := []string{
		".env",
		"../../.env", // From cmd/server/ -> project root
		"../.env",
	}

	for _, path := range paths {
		file, err := os.Open(path)
		if err != nil {
			continue
		}
		defer file.Close()

		scanner := bufio.NewScanner(file)
		count := 0
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			// Skip comments and empty lines
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			parts := strings.SplitN(line, "=", 2)
			if len(parts) != 2 {
				continue
			}
			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])
			// Only set if not already set by actual env
			if _, exists := os.LookupEnv(key); !exists {
				os.Setenv(key, value)
				count++
			}
		}
		log.Printf("[CONFIG] Loaded %d variables from %s", count, path)
		return
	}
	log.Println("[CONFIG] ⚠ No .env file found — using system environment variables only")
}

func main() {
	utils.InitLogger()
	log.Println("========================================")
	log.Println("  ESSAIM IA — Backend Server Starting")
	log.Println("========================================")

	// === Load .env file ===
	loadEnvFile()

	// === Configuration ===
	mongoURI := utils.GetEnv("MONGO_URI", "mongodb://localhost:27017")
	geminiKey := utils.GetEnv("GEMINI_API_KEY", "")
	geminiModel := utils.GetEnv("GEMINI_MODEL", "gemini-2.5-pro")
	globalBudget := 1000000000.0 // Default 1 Billion tokens
	if envBudget := utils.GetEnv("GLOBAL_BUDGET", ""); envBudget != "" {
		if parsed, err := strconv.ParseFloat(envBudget, 64); err == nil {
			if parsed < 1000000 {
				parsed = parsed * 1000000 // Convert what user thought was euros into tokens
			}
			globalBudget = parsed
		}
	}
	port := utils.GetEnv("PORT", "8080")

	log.Printf("[CONFIG] MONGO_URI    = %s", mongoURI)
	log.Printf("[CONFIG] GEMINI_MODEL = %s", geminiModel)
	log.Printf("[CONFIG] PORT         = %s", port)
	if geminiKey == "" {
		log.Println("[CONFIG] ⚠ GEMINI_API_KEY is EMPTY — LLM calls will fail!")
	} else {
		log.Printf("[CONFIG] GEMINI_API_KEY = %s...%s (loaded)", geminiKey[:8], geminiKey[len(geminiKey)-4:])
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// === Initialize MongoDB (OPTIONAL — won't crash if unavailable) ===
	var repo *persistence.MongoRepo
	repo, err := persistence.NewMongoRepo(ctx, mongoURI)
	if err != nil {
		log.Printf("[WARN] ⚠ MongoDB connection failed: %v", err)
		log.Println("[WARN] ⚠ Server will run WITHOUT persistence (in-memory only)")
		repo = nil
	} else {
		defer repo.Disconnect(ctx)
		log.Println("[OK] ✅ MongoDB connected")
	}

	// === Initialize LLM Client (OPTIONAL — won't crash if unavailable) ===
	var llmClient *llm.Client
	if geminiKey != "" {
		llmClient, err = llm.NewClient(ctx, geminiKey, geminiModel)
		if err != nil {
			log.Printf("[WARN] ⚠ LLM client init failed: %v", err)
			log.Println("[WARN] ⚠ Server will run WITHOUT LLM capabilities")
			llmClient = nil
		} else {
			defer llmClient.Close()
			log.Println("[OK] ✅ Gemini LLM client connected")
		}
	} else {
		log.Println("[WARN] ⚠ No GEMINI_API_KEY — LLM disabled")
	}

	// === Initialize Core Components ===
	registry := graph.NewRegistry()
	budgetMgr := economy.NewBudgetManager(globalBudget)
	wsHub := ws.NewHub()

	log.Println("[OK] ✅ Core components initialized (Registry, BudgetMgr, WebSocket Hub)")

	// === Create Dispatcher first (processor needs it) ===
	// We create dispatcher with a temporary nil handler, then set the real one
	var processor *lifecycle.Processor
	handler := func(pkt message.Packet) error {
		if processor != nil {
			return processor.HandlePacket(pkt)
		}
		log.Printf("[WARN] Processor not ready, dropping packet %s", pkt.Head.ID)
		return nil
	}

	dispatcher := orchestrator.NewDispatcher(registry, budgetMgr, handler)

	// === Create Journal (auto-generates markdown per mission) ===
	missionJournal := journal.New("logs/journals")
	log.Println("[OK] ✅ Journal initialisé (logs/journals/)")

	// === Create Processor (the brain) ===
	processor = lifecycle.NewProcessor(ctx, registry, budgetMgr, llmClient, repo, wsHub, dispatcher, missionJournal)
	log.Println("[OK] ✅ Agent Processor (brain) initialized")

	dispatcher.Start()
	defer dispatcher.Stop()

	log.Println("[OK] ✅ Dispatcher started (50 workers)")

	// === HTTP Routes ===
	mux := http.NewServeMux()

	// WebSocket endpoint
	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[WS] New WebSocket connection attempt from %s", r.RemoteAddr)
		wsHub.HandleWebSocket(w, r)
	})

	// Health check
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":        "ok",
			"active_agents": registry.Count(),
			"global_budget": budgetMgr.GetGlobalBudget(),
			"ws_clients":    wsHub.ClientCount(),
			"mongo":         repo != nil,
			"llm":           llmClient != nil,
		})
	})

	// GET /api/test-llm - Test in-situ if GEMINI_API_KEY is valid and authorized
	mux.HandleFunc("/api/test-llm", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[API] GET /api/test-llm from %s", r.RemoteAddr)
		w.Header().Set("Content-Type", "application/json")

		if llmClient == nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"status":  "error",
				"message": "Client LLM non initialisé (clé GEMINI_API_KEY manquante dans le fichier .env)",
			})
			return
		}

		// Quick 15s timeout for Gemini API key response ping
		testCtx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()

		systemPrompt := "Tu es un service de test. Réponds obligatoirement avec un format JSON strict : {\"status\": \"ok\"}"
		userMessage := "Ping"

		resp, err := llmClient.Call(testCtx, systemPrompt, userMessage)
		if err != nil {
			log.Printf("[API] ❌ Gemini API test failed: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"status":  "error",
				"message": fmt.Sprintf("Clé API invalide ou expirée: %v", err),
			})
			return
		}

		log.Println("[API] ✅ Gemini API test successful!")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":     "ok",
			"message":    "Clé API Gemini valide et active !",
			"latency_ms": resp.LatencyMs,
			"model":      geminiModel,
		})
	})

	// POST /start - Genesis: Inject the first Agent "Alpha"
	mux.HandleFunc("/start", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[API] %s /start from %s", r.Method, r.RemoteAddr)

		if r.Method != http.MethodPost {
			http.Error(w, `{"error":"Method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}

		var req struct {
			Objective string  `json:"objective"`
			Budget    float64 `json:"budget,omitempty"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			log.Printf("[API] ❌ Invalid JSON body: %v", err)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "Invalid JSON: " + err.Error()})
			return
		}
		if req.Objective == "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "objective is required"})
			return
		}
		if req.Budget <= 0 {
			req.Budget = 5000000.0 // Default 5 Million tokens for a single mission if none specified
		} else {
			// User inputted budget in euros, scale up to tokens (approx 1M tokens per euro)
			req.Budget = req.Budget * 1000000
		}

		log.Printf("[API] Creating Alpha agent for objective: %s", req.Objective)
		missionJournal.SetObjective(req.Objective)
		processor.SetObjective(req.Objective) // reset stats for this mission

		// Create the Alpha agent (root of the graph)
		alpha := &agent.Agent{
			ID:         uuid.New().String(),
			BubbleID:   "", // Root architect stands alone initially
			Role:       agent.RoleArchitect,
			Status:     agent.StatusBorn,
			Budget:     req.Budget,
			RetryCount: 0,
			Memory:     make([]agent.Message, 0),
			CreatedAt:  time.Now(),
			UpdatedAt:  time.Now(),
		}

		// Register in graph
		if err := registry.Register(alpha); err != nil {
			log.Printf("[API] ❌ Failed to register Alpha: %v", err)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "Failed to register: " + err.Error()})
			return
		}

		// Register budget
		if err := budgetMgr.RegisterAgent(alpha.ID, alpha.Budget); err != nil {
			log.Printf("[API] ❌ Failed to register budget: %v", err)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "Budget error: " + err.Error()})
			return
		}

		// Persist to MongoDB (optional)
		if repo != nil {
			if err := repo.UpsertAgent(ctx, alpha); err != nil {
				log.Printf("[API] ⚠ Failed to persist Alpha (non-fatal): %v", err)
			}
		}

		// Broadcast the new agent to frontend
		wsHub.Broadcast(ws.Event{
			Type: ws.EventGraphUpdate,
			Payload: map[string]interface{}{
				"action": "ADD_NODE",
				"agent":  alpha,
			},
		})

		// Send the initial task to Alpha via the dispatcher
		taskPkt := message.Packet{
			Head: message.Header{
				ID:        uuid.New().String(),
				Timestamp: time.Now().Unix(),
				From:      "SYSTEM",
				To:        alpha.ID,
				Type:      message.CmdTask,
			},
			Body: map[string]interface{}{
				"objective": req.Objective,
			},
		}
		dispatcher.Enqueue(taskPkt)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"message":  "Agent Alpha spawned",
			"agent_id": alpha.ID,
			"budget":   alpha.Budget,
		})

		log.Printf("[GENESIS] ✅ Agent Alpha (%s) spawned with objective: %s", alpha.ID, req.Objective)
	})

	// POST /stop - Terminate the entire active swarm
	mux.HandleFunc("/stop", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[API] POST /stop from %s", r.RemoteAddr)
		w.Header().Set("Content-Type", "application/json")

		activeNodes := registry.AllNodes()
		count := len(activeNodes)

		for _, n := range activeNodes {
			id := n.Agent.ID
			// Remove from registry
			registry.Remove(id)
			// Remove budget
			budgetMgr.RemoveAgent(id)

			// Broadcast removal to clients
			wsHub.Broadcast(ws.Event{
				Type: ws.EventGraphUpdate,
				Payload: map[string]interface{}{
					"action":  "REMOVE_NODE",
					"agentId": id,
				},
			})
		}

		// Send a system alert message
		wsHub.Broadcast(ws.Event{
			Type: ws.EventSystemAlert,
			Payload: map[string]interface{}{
				"message": "⚠️ ESSAIM ARRÊTÉ : Mission interrompue manuellement par l'opérateur.",
			},
		})

		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":              "stopped",
			"killed_agents_count": count,
		})
	})

	// CORS middleware wrapper
	corsHandler := corsMiddleware(mux)

	// === Start Server ===
	server := &http.Server{
		Addr:    ":" + port,
		Handler: corsHandler,
	}

	// Graceful shutdown
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		log.Println("[SERVER] Shutting down gracefully...")
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()
		server.Shutdown(shutdownCtx)
	}()

	log.Println("========================================")
	log.Printf("  ✅ ESSAIM IA Backend READY on :%s", port)
	log.Println("  Routes: /ws, /health, /start")
	log.Println("========================================")

	if err := server.ListenAndServe(); err != http.ErrServerClosed {
		log.Fatalf("[FATAL] ❌ Server error: %v", err)
	}
	log.Println("[SERVER] Stopped")
}

// corsMiddleware adds CORS headers for frontend communication.
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}
		next.ServeHTTP(w, r)
	})
}
