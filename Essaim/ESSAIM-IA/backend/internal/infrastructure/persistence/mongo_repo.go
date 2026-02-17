package persistence

import (
	"context"
	"fmt"
	"log"
	"time"

	"essaim-backend/internal/domain/agent"
	"essaim-backend/internal/domain/message"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

const (
	databaseName      = "essaim_db"
	agentCollection   = "agents_snapshot"
	missionCollection = "mission_logs"
	missionLogTTLDays = 7
)

// MongoRepo implements persistence for agents and mission logs using MongoDB.
type MongoRepo struct {
	client   *mongo.Client
	db       *mongo.Database
	agents   *mongo.Collection
	missions *mongo.Collection
}

// NewMongoRepo connects to MongoDB and initializes collections with required indexes.
func NewMongoRepo(ctx context.Context, uri string) (*MongoRepo, error) {
	client, err := mongo.Connect(options.Client().ApplyURI(uri))
	if err != nil {
		return nil, fmt.Errorf("failed to connect to MongoDB: %w", err)
	}

	// Verify connection
	if err := client.Ping(ctx, nil); err != nil {
		return nil, fmt.Errorf("failed to ping MongoDB: %w", err)
	}

	db := client.Database(databaseName)
	repo := &MongoRepo{
		client:   client,
		db:       db,
		agents:   db.Collection(agentCollection),
		missions: db.Collection(missionCollection),
	}

	if err := repo.ensureIndexes(ctx); err != nil {
		return nil, fmt.Errorf("failed to create indexes: %w", err)
	}

	log.Printf("[MONGO] Connected to %s, database: %s", uri, databaseName)
	return repo, nil
}

// ensureIndexes creates required indexes per Architecture §5.3.
func (r *MongoRepo) ensureIndexes(ctx context.Context) error {
	// agents_snapshot indexes: parent_id, status
	agentIndexes := []mongo.IndexModel{
		{Keys: bson.D{{Key: "parent_id", Value: 1}}},
		{Keys: bson.D{{Key: "status", Value: 1}}},
	}
	if _, err := r.agents.Indexes().CreateMany(ctx, agentIndexes); err != nil {
		return fmt.Errorf("agent indexes: %w", err)
	}

	// mission_logs TTL index: auto-delete after 7 days
	ttlSeconds := int32(missionLogTTLDays * 24 * 60 * 60)
	missionIndexes := []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "timestamp", Value: 1}},
			Options: options.Index().SetExpireAfterSeconds(ttlSeconds),
		},
	}
	if _, err := r.missions.Indexes().CreateMany(ctx, missionIndexes); err != nil {
		return fmt.Errorf("mission indexes: %w", err)
	}

	return nil
}

// UpsertAgent inserts or updates an agent snapshot (frequent upsert).
func (r *MongoRepo) UpsertAgent(ctx context.Context, ag *agent.Agent) error {
	ag.UpdatedAt = time.Now()
	filter := bson.M{"_id": ag.ID}
	opts := options.Replace().SetUpsert(true)
	_, err := r.agents.ReplaceOne(ctx, filter, ag, opts)
	return err
}

// GetAgent retrieves an agent by ID.
func (r *MongoRepo) GetAgent(ctx context.Context, agentID string) (*agent.Agent, error) {
	var ag agent.Agent
	filter := bson.M{"_id": agentID}
	if err := r.agents.FindOne(ctx, filter).Decode(&ag); err != nil {
		return nil, err
	}
	return &ag, nil
}

// GetAgentsByParent retrieves all children of a given parent agent.
func (r *MongoRepo) GetAgentsByParent(ctx context.Context, parentID string) ([]*agent.Agent, error) {
	filter := bson.M{"parent_id": parentID}
	cursor, err := r.agents.Find(ctx, filter)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var agents []*agent.Agent
	if err := cursor.All(ctx, &agents); err != nil {
		return nil, err
	}
	return agents, nil
}

// GetAgentsByStatus retrieves all agents with a given status (for monitoring).
func (r *MongoRepo) GetAgentsByStatus(ctx context.Context, status agent.AgentStatus) ([]*agent.Agent, error) {
	filter := bson.M{"status": status}
	cursor, err := r.agents.Find(ctx, filter)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var agents []*agent.Agent
	if err := cursor.All(ctx, &agents); err != nil {
		return nil, err
	}
	return agents, nil
}

// DeleteAgent removes an agent from the snapshot collection.
func (r *MongoRepo) DeleteAgent(ctx context.Context, agentID string) error {
	filter := bson.M{"_id": agentID}
	_, err := r.agents.DeleteOne(ctx, filter)
	return err
}

// InsertMissionLog adds an immutable log record.
func (r *MongoRepo) InsertMissionLog(ctx context.Context, logEntry *message.MissionLog) error {
	_, err := r.missions.InsertOne(ctx, logEntry)
	return err
}

// Disconnect closes the MongoDB connection.
func (r *MongoRepo) Disconnect(ctx context.Context) error {
	return r.client.Disconnect(ctx)
}
