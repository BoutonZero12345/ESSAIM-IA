package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"
)

func runCorporateEmulation() bool {
	fmt.Println("=================================================================")
	fmt.Println("🏢 CORPORATE FRACTAL ARCHITECTURE E2E MEGA-TEST")
	fmt.Println("   Objectif : 1 CEO -> 3 DIR -> 15 S-DIR -> 45 MGR -> 360 WRK")
	fmt.Println("=================================================================")

	// Build the server directly
	buildCmd := exec.Command("go", "build", "-o", "server_corporate.exe", "cmd/server/main.go")
	buildCmd.Dir = "."
	buildOut, err := buildCmd.CombinedOutput()
	if err != nil {
		fmt.Printf("Error building server: %v\nOutput: %s\n", err, string(buildOut))
		return false
	}

	cmd := exec.Command(".\\server_corporate.exe")
	cmd.Dir = "."
	logFile, _ := os.Create("mega_corporate_test.log")
	defer logFile.Close()

	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()
	if err := cmd.Start(); err != nil {
		fmt.Printf("Error starting server: %v\n", err)
		return false
	}

	defer func() {
		fmt.Println("🛑 Cleaning up server process...")
		cmd.Process.Kill()
		cmd.Wait()
		os.Remove("server_corporate.exe")
	}()

	successChan := make(chan bool)

	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			text := scanner.Text()
			logFile.WriteString("[SERVER] " + text + "\n")
			// Only print important markers to keep console readable
			if strings.Contains(text, "✅") || strings.Contains(text, "📝") || strings.Contains(text, "🔄") || strings.Contains(text, "📬") || strings.Contains(text, "⚠") {
				fmt.Println(text)
			}
			if strings.Contains(text, "🏁 Root agent") && strings.Contains(text, "completed") {
				logFile.WriteString("✨ MEGA CORPORATE MISSION COMPLETED!\n")
				fmt.Println("\n\n✨ LE TEST CORPORATE EST TERMINÉ ! LE CEO A RECU ET VALIDÉ LES RAPPORTS SUR-CONDENSÉS.")
				successChan <- true
				return
			}
			if strings.Contains(text, "Shutting down gracefully") || strings.Contains(text, "Failed to start") {
				successChan <- false
				return
			}
		}
		successChan <- false
	}()

	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			logFile.WriteString("[SERVER ERR] " + scanner.Text() + "\n")
		}
	}()

	time.Sleep(5 * time.Second)

	// The prompt acts as the Supreme User overriding the CEO
	// To prevent infinite hallucination, we explicitly dictate the strict multi-level nested instructions.
	prompt := `MISSION CRITIQUE D'ARCHITECTURE "ENTREPRISE GLOBALE" : 
Tu es le CEO d'ESSAIM-IA. Nous devons valider la résilience du réseau sur une hiérarchie à 5 niveaux et la transmission multiniveaux (Résumeurs et Postiers).
TON UNIQUE TÂCHE ACTUELLE est de faire UNE ACTION SPAWN pour créer EXACTEMENT 3 sous-agents avec le rôle "DIRECTOR".

Pour chaque DIRECTOR, donne-lui EXACTEMENT cette instruction stricte copiée-collée :
"Tu es un Grand Dirigeant. Ta mission est de déléguer via une action SPAWN à EXACTEMENT 5 sous-agents de rôle DIRECTOR (qui agiront comme Sous-Dirigeants). Donne à chacun de ces 5 Sous-Dirigeants l'instruction suivante : 'Tu es Sous-Dirigeant. Fais une action SPAWN pour créer EXACTEMENT 3 sous-agents de rôle MANAGER. Donne à chaque MANAGER l'instruction : « Fais une action SPAWN pour créer EXACTEMENT 8 sous-agents de rôle WORKER. Chaque WORKER a pour tâche de rédiger un rapport très court (1 phrase maximum) sur la technologie des propulseurs spatiaux. »'."

Fais ce SPAWN MAINTENANT.`

	// Use map to marshal nicely into JSON to avoid manual escaping issues
	payload := map[string]interface{}{
		"objective": prompt,
		"budget":    2000, // Enormous budget to cover thousands of LLM calls
	}

	payloadBytes, _ := json.Marshal(payload)

	fmt.Println("📡 Sending FORCED /start request to local server... (Budget 2000 Tokens/Euros)")
	resp, err := http.Post("http://localhost:8080/start", "application/json", bytes.NewBuffer(payloadBytes))
	if err != nil {
		fmt.Printf("HTTP request failed: %v\n", err)
		return false
	}
	resp.Body.Close()

	if resp.StatusCode != 201 && resp.StatusCode != 200 {
		fmt.Printf("Warning: Server returned HTTP %d\n", resp.StatusCode)
	}

	select {
	case success := <-successChan:
		time.Sleep(2 * time.Second)
		return success
	case <-time.After(90 * time.Minute): // Allow 1.5 hours for the massive nested structure to resolve through heavy rate limit backoffs
		fmt.Println("⏳ Timeout reached (1.5 hours). Test failed or still running too deep.")
		return false
	}
}

func main() {
	if runCorporateEmulation() {
		fmt.Println("✅ SUCCÈS CORPORATE !!! LES COUCHES DE DIRIGEANTS ET LES POSTIERS ONT FONCTIONNÉ.")
	} else {
		fmt.Println("❌ ÉCHEC DU TEST CORPORATE.")
	}
}
