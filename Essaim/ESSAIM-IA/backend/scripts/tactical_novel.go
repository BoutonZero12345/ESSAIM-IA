package main

import (
	"bufio"
	"bytes"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"
)

// Response struct specifically to catch the Architect's initial plan (Chapters)
type StartRequest struct {
	Objective string  `json:"objective"`
	Budget    float64 `json:"budget"`
}

func runBruteForceNovel() bool {
	fmt.Println("================================================")
	fmt.Println("🚀 TACTICAL NOVEL GENERATOR: FORCING 100 CHAPTERS VIA API PIPELINE")
	fmt.Println("================================================")

	// Build the server directly
	buildCmd := exec.Command("go", "build", "-o", "server_forced.exe", "cmd/server/main.go")
	buildCmd.Dir = "."
	buildOut, err := buildCmd.CombinedOutput()
	if err != nil {
		fmt.Printf("Error building server: %v\nOutput: %s\n", err, string(buildOut))
		return false
	}

	cmd := exec.Command(".\\server_forced.exe")
	cmd.Dir = "."
	logFile, _ := os.Create("mega_forced_novel.log")
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
		os.Remove("server_forced.exe")
	}()

	// The logic: 1 API call per chapter iteratively for the ultimate scale.
	// As doing it through 1 massive JSON network inside ESSAIM crashes the LLM output limits.
	// But first, let's ask ESSAIM to just generate the structure natively.

	successChan := make(chan bool)

	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			text := scanner.Text()
			logFile.WriteString("[SERVER] " + text + "\n")
			if strings.Contains(text, "✅ Sous-agent") || strings.Contains(text, "📝 Agent") || strings.Contains(text, "🤖 Appel LLM") {
				fmt.Println(text)
			}
			if strings.Contains(text, "🏁 Root agent") && strings.Contains(text, "completed") {
				logFile.WriteString("✨ MEGA FORCED MISSION COMPLETED DETECTED!\n")
				fmt.Println("\n\n✨ IL L'A FAIT ! MISSION TERMINÉE. LIVRE ÉCRIT.")
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

	// Step 1: Force the LLM to create an organic recursive hierarchy of 50 chapters.
	prompt := `TACTICAL_NOVEL_FORCE: Ecrit moi un livre. Je veux un court roman de science fiction avec une forte morale.`

	promptEscaped := strings.ReplaceAll(prompt, "\n", "\\n")
	promptEscaped = strings.ReplaceAll(promptEscaped, "\"", "\\\"")
	payload := fmt.Sprintf(`{"objective": "%s", "budget": 300}`, promptEscaped)

	fmt.Println("📡 Sending FORCED /start request to local server... (Budget 300 Euros)")
	resp, err := http.Post("http://localhost:8080/start", "application/json", bytes.NewBuffer([]byte(payload)))
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
	case <-time.After(5 * time.Hour):
		fmt.Println("⏳ Timeout reached (5 hours). Test failed.")
		return false
	}
}

func main() {
	if runBruteForceNovel() {
		fmt.Println("✅ SUCCES EPIQUE !!! LE LIVRE EST DANS 'LIVRE_SF_COMPLET.md'")
	}
}
