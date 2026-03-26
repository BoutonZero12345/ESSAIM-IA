package main

import (
	"context"
	"fmt"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func main() {
	clientOptions := options.Client().ApplyURI("mongodb://localhost:27017")
	client, err := mongo.Connect(context.TODO(), clientOptions)
	if err != nil {
		fmt.Println("Error connecting:", err)
		return
	}
	err = client.Database("essaim_db").Drop(context.TODO())
	if err != nil {
		fmt.Println("Error dropping DB:", err)
		return
	}
	fmt.Println("✅ DB dropped successfully.")
}
