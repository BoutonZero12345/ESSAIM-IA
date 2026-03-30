package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/google/generative-ai-go/genai"
	"github.com/joho/godotenv"
	"google.golang.org/api/option"
)

func main() {
	if err := godotenv.Load("../.env"); err != nil {
		log.Println("No .env file found or error reading it")
	}

	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		log.Fatal("GEMINI_API_KEY is missing")
	}
	fmt.Println("Using API Key starting with:", apiKey[:10])

	ctx := context.Background()
	client, err := genai.NewClient(ctx, option.WithAPIKey(apiKey))
	if err != nil {
		log.Fatal("NewClient error:", err)
	}
	defer client.Close()

	modelName := os.Getenv("GEMINI_MODEL")
	if modelName == "" {
		modelName = "gemini-2.0-flash-lite"
	}
	fmt.Println("Using Model:", modelName)

	model := client.GenerativeModel(modelName)
	resp, err := model.GenerateContent(ctx, genai.Text("Say hello"))
	if err != nil {
		log.Fatal("API Call error:", err)
	}

	if len(resp.Candidates) > 0 && len(resp.Candidates[0].Content.Parts) > 0 {
		fmt.Println("API SUCCESS, Response:", resp.Candidates[0].Content.Parts[0])
	} else {
		fmt.Println("API returned empty response.")
	}
}
