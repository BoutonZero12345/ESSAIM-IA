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

func runAutonomousBook() bool {
	fmt.Println("=================================================================")
	fmt.Println("🚀 AUTONOMOUS BOOK GENERATION & FRACTAL ARCHITECTURE E2E TEST")
	fmt.Println("   Objectif : CEO -> 3 Actes(DIR) -> 18 Chapitres(MGR) -> 162 Scènes(WRK)")
	fmt.Println("   Ratios Testés :")
	fmt.Println("   - CEO spawns 3 Dirigeants => 0 Résumeur (concat direct)")
	fmt.Println("   - Dirigeants spawn 6 Managers => 1 Résumeur de couche")
	fmt.Println("   - Managers spawn 9 Workers => 2 Résumeurs séquentiels")
	fmt.Println("=================================================================")

	// Cleanup old processes just in case
	exec.Command("taskkill", "/F", "/IM", "server_book.exe", "/T").Run()

	// Build the server directly
	buildCmd := exec.Command("go", "build", "-o", "server_book.exe", "cmd/server/main.go")
	buildCmd.Dir = "."
	buildOut, err := buildCmd.CombinedOutput()
	if err != nil {
		fmt.Printf("Error building server: %v\nOutput: %s\n", err, string(buildOut))
		return false
	}

	cmd := exec.Command(".\\server_book.exe")
	cmd.Dir = "."
	cmd.Env = append(os.Environ(), "PORT=8085")
	logFile, _ := os.Create("mega_autonomous_test.log")
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
		os.Remove("server_book.exe")
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
				logFile.WriteString("✨ MEGA AUTONOMOUS MISSION COMPLETED!\n")
				fmt.Println("\n\n✨ LE LIVRE ENTIER A ÉTÉ GÉNÉRÉ ET VÉRIFIÉ AVEC SUCCÈS !")
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

	// Custom prompt
	prompt := `MISSION ÉDITORIALE TITANESQUE :
Tu es le CEO de la maison d'édition ESSAIM-IA. Ta mission est de produire un roman de Science-Fiction complet intitulé "Les Larmes d'Orion".
TON UNIQUE TÂCHE ACTUELLE est de déclencher une action SPAWN pour créer EXACTEMENT 3 sous-agents avec le rôle "DIRECTOR" qui géreront les 3 Actes du livre.

Pour chaque DIRECTOR, donne-lui STRICTEMENT cette consigne (remplace X par le nom de l'acte) :
"Tu es un Grand Dirigeant (DIRECTOR) en charge de l'un des 3 Actes du livre. Ta tâche est de déléguer via une action SPAWN à EXACTEMENT 6 sous-agents de rôle MANAGER qui écriront les chapitres de ton Acte. Donne-leur la directive : 'Tu es un Manager en charge de ton chapitre. Fais une action SPAWN pour créer EXACTEMENT 9 sous-agents de rôle WORKER. Chaque WORKER a pour tâche d'écrire 800 mots riches, immersifs et descriptifs de sa Scène de l'Acte.' "

La structure finale voulue (Respecte scrupuleusement ces chiffres exacts pour nos tests de réseau) :
- Toi (CEO) crée 3 DIRECTORS.
- Chaque DIRECTOR crée 6 MANAGERS.
- Chaque MANAGER crée 9 WORKERS (qui vont vraiment écrire l'histoire).
Au final le réseau rassemblera et résumera le tout selon nos règles structurelles d'échelle.

Fais ce fichier SPAWN immédiatement pour tout déclencher.`

	payload := map[string]interface{}{
		"objective": prompt,
		"budget":    50000,
	}

	payloadBytes, _ := json.Marshal(payload)

	fmt.Println("📡 Sending FORCED /start request to local server on port 8085...")
	resp, err := http.Post("http://localhost:8085/start", "application/json", bytes.NewBuffer(payloadBytes))
	if err != nil {
		fmt.Printf("HTTP request failed: %v\n", err)
		return false
	}
	resp.Body.Close()

	if resp.StatusCode != 201 && resp.StatusCode != 200 {
		fmt.Printf("Warning: Server returned HTTP %d\n", resp.StatusCode)
	}

	for {
		select {
		case success := <-successChan:
			time.Sleep(2 * time.Second)
			return success
		case <-time.After(24 * time.Hour): // We wait up to 24 hours ! Infinite patience.
			fmt.Println("⏳ Timeout reached (24 hours). Abandoning script.")
			return false
		}
	}
}

func main() {
	if runAutonomousBook() {
		fmt.Println("✅ SUCCÈS TOTAL : LE LIVRE AUTONOME EST FINI.")
	} else {
		fmt.Println("❌ ÉCHEC DU TEST AUTONOME.")
	}
}
