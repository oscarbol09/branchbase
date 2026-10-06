package mongodb

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/branchbase/branchbase/internal/driver"
	"github.com/branchbase/branchbase/internal/git"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Config holds connection parameters for MongoDB
type Config struct {
	URI           string
	BaseDatabase  string
	DefaultBranch string
}

type MongoDBDriver struct {
	cfg    Config
	client *mongo.Client
}

func init() {
	driver.Register("mongodb", func(params map[string]interface{}) (driver.Driver, error) {
		uri, _ := params["uri"].(string)
		baseDb, _ := params["base_database"].(string)
		defaultBranch, _ := params["default_branch"].(string)

		if uri == "" {
			return nil, fmt.Errorf("missing 'uri' for mongodb driver")
		}
		if baseDb == "" {
			return nil, fmt.Errorf("missing 'base_database' for mongodb driver")
		}

		return New(Config{
			URI:           uri,
			BaseDatabase:  baseDb,
			DefaultBranch: defaultBranch,
		})
	})
}

func New(cfg Config) (*MongoDBDriver, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(cfg.URI))
	if err != nil {
		return nil, fmt.Errorf("failed to connect to mongodb: %w", err)
	}

	return &MongoDBDriver{
		cfg:    cfg,
		client: client,
	}, nil
}

func (d *MongoDBDriver) Name() string {
	return "mongodb"
}

func (d *MongoDBDriver) formatDBName(branchName string) string {
	if branchName == d.cfg.DefaultBranch {
		return d.cfg.BaseDatabase
	}
	sanitized := git.SanitizeBranchName(branchName)
	return fmt.Sprintf("%s_%s", d.cfg.BaseDatabase, sanitized)
}

func (d *MongoDBDriver) Ping(ctx context.Context) error {
	return d.client.Ping(ctx, nil)
}

func (d *MongoDBDriver) BranchExists(ctx context.Context, branchName string) (bool, error) {
	dbName := d.formatDBName(branchName)
	names, err := d.client.ListDatabaseNames(ctx, bson.M{"name": dbName})
	if err != nil {
		return false, fmt.Errorf("failed to list databases: %w", err)
	}
	return len(names) > 0, nil
}

func (d *MongoDBDriver) CreateBranch(ctx context.Context, sourceBranch, targetBranch string) error {
	sourceDB := d.formatDBName(sourceBranch)
	targetDB := d.formatDBName(targetBranch)

	srcDb := d.client.Database(sourceDB)
	
	colls, err := srcDb.ListCollectionNames(ctx, bson.M{})
	if err != nil {
		return fmt.Errorf("failed to list collections in source db %q: %w", sourceDB, err)
	}

	for _, collName := range colls {
		// Ignore system collections
		if strings.HasPrefix(collName, "system.") {
			continue
		}
		srcColl := srcDb.Collection(collName)
		
		// Use aggregation $out to copy to the target database
		pipeline := mongo.Pipeline{
			bson.D{{Key: "$match", Value: bson.D{}}},
			bson.D{{Key: "$out", Value: bson.D{
				{Key: "db", Value: targetDB},
				{Key: "coll", Value: collName},
			}}},
		}
		cursor, err := srcColl.Aggregate(ctx, pipeline)
		if err != nil {
			return fmt.Errorf("failed to clone collection %q: %w", collName, err)
		}
		if err := cursor.Close(ctx); err != nil {
			return fmt.Errorf("failed to close cursor: %w", err)
		}
	}
	return nil
}

func (d *MongoDBDriver) DeleteBranch(ctx context.Context, branchName string) error {
	dbName := d.formatDBName(branchName)
	if dbName == d.cfg.BaseDatabase {
		return fmt.Errorf("cannot delete protected base database %q", dbName)
	}
	return d.client.Database(dbName).Drop(ctx)
}

func (d *MongoDBDriver) ListBranches(ctx context.Context) ([]driver.BranchInfo, error) {
	dbs, err := d.client.ListDatabases(ctx, bson.M{})
	if err != nil {
		return nil, fmt.Errorf("failed to list databases: %w", err)
	}

	prefix := d.cfg.BaseDatabase + "_"
	var branches []driver.BranchInfo

	for _, db := range dbs.Databases {
		if db.Name == d.cfg.BaseDatabase {
			branches = append(branches, driver.BranchInfo{
				Name:        d.cfg.DefaultBranch,
				Database:    db.Name,
				SizeBytes:   db.SizeOnDisk,
				IsProtected: true,
			})
			continue
		}

		if strings.HasPrefix(db.Name, prefix) {
			branchName := strings.TrimPrefix(db.Name, prefix)
			branches = append(branches, driver.BranchInfo{
				Name:        branchName,
				Database:    db.Name,
				SizeBytes:   db.SizeOnDisk,
				IsProtected: false,
			})
		}
	}

	return branches, nil
}

func (d *MongoDBDriver) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return d.client.Disconnect(ctx)
}
