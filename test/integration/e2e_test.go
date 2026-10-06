//go:build integration

package integration

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/branchbase/branchbase/internal/config"
	"github.com/branchbase/branchbase/internal/driver/mongodb"
	"github.com/branchbase/branchbase/internal/driver/mysql"
	"github.com/branchbase/branchbase/internal/driver/postgres"
	"github.com/branchbase/branchbase/internal/driver/sqlite"
	"github.com/branchbase/branchbase/internal/proxy"
	
	_ "github.com/lib/pq"
	_ "github.com/go-sql-driver/mysql"
	_ "modernc.org/sqlite"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func TestPostgreSQLE2EIntegration(t *testing.T) {
	pgHost := os.Getenv("PGHOST")
	if pgHost == "" {
		pgHost = "127.0.0.1"
	}
	pgPort := 5433
	if p := os.Getenv("PGPORT"); p != "" {
		_, _ = fmt.Sscanf(p, "%d", &pgPort)
	}

	baseDSN := fmt.Sprintf("postgres://postgres:postgres@%s:%d/postgres?sslmode=disable", pgHost, pgPort)
	db, err := sql.Open("postgres", baseDSN)
	if err != nil {
		t.Fatalf("failed to connect to postgres: %v", err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		t.Fatalf("postgres ping failed: %v", err)
	}

	_, _ = db.Exec("DROP DATABASE IF EXISTS myapp_dev;")
	_, _ = db.Exec("DROP DATABASE IF EXISTS myapp_dev_feature_payments;")
	_, err = db.Exec("CREATE DATABASE myapp_dev;")
	if err != nil {
		t.Fatalf("failed to create base database: %v", err)
	}

	appDSN := fmt.Sprintf("postgres://postgres:postgres@%s:%d/myapp_dev?sslmode=disable", pgHost, pgPort)
	appDB, err := sql.Open("postgres", appDSN)
	if err != nil {
		t.Fatalf("failed to connect to appDB: %v", err)
	}

	_, err = appDB.Exec("CREATE TABLE users (id SERIAL PRIMARY KEY, name TEXT); INSERT INTO users (name) VALUES ('Alice');")
	if err != nil {
		_ = appDB.Close()
		t.Fatalf("failed to seed users table: %v", err)
	}
	_ = appDB.Close()

	tempDir := t.TempDir()
	gitDir := filepath.Join(tempDir, ".git")
	_ = os.MkdirAll(gitDir, 0755)
	_ = os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte("ref: refs/heads/main\n"), 0644)

	cfg := config.DefaultConfig()
	cfg.Connection.Host = pgHost
	cfg.Connection.Port = pgPort
	cfg.Connection.BaseDatabase = "myapp_dev"
	cfg.Proxy.ListenPort = 0 

	pgDrv, err := postgres.New(postgres.Config{
		Host:         pgHost,
		Port:         pgPort,
		User:         "postgres",
		Password:     "postgres",
		BaseDatabase: "myapp_dev",
		SSLMode:      "disable",
	})
	if err != nil {
		t.Fatalf("postgres driver init failed: %v", err)
	}
	defer pgDrv.Close()

	srv := proxy.NewServer(&cfg, tempDir, pgDrv)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := srv.Start(ctx); err != nil {
		t.Fatalf("proxy.Start failed: %v", err)
	}
	defer func() { _ = srv.Stop() }()

	if err := os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte("ref: refs/heads/feature/payments\n"), 0644); err != nil {
		t.Fatalf("failed to update git HEAD: %v", err)
	}

	if err := pgDrv.CreateBranch(ctx, "main", "feature/payments"); err != nil {
		t.Fatalf("CreateBranch failed: %v", err)
	}

	featDSN := fmt.Sprintf("postgres://postgres:postgres@%s:%d/myapp_dev_feature_payments?sslmode=disable", pgHost, pgPort)
	featDB, err := sql.Open("postgres", featDSN)
	if err != nil {
		t.Fatalf("failed to connect to featDB: %v", err)
	}
	defer featDB.Close()

	var count int
	err = featDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM users").Scan(&count)
	if err != nil || count != 1 {
		t.Fatalf("expected 1 cloned user in feature branch database, got count=%d, err=%v", count, err)
	}
}

func TestMySQLE2EIntegration(t *testing.T) {
	myHost := os.Getenv("MYSQL_HOST")
	if myHost == "" {
		myHost = "127.0.0.1"
	}
	myPort := 3307
	if p := os.Getenv("MYSQL_PORT"); p != "" {
		_, _ = fmt.Sscanf(p, "%d", &myPort)
	}

	baseDSN := fmt.Sprintf("root:root@tcp(%s:%d)/?multiStatements=true", myHost, myPort)
	db, err := sql.Open("mysql", baseDSN)
	if err != nil {
		t.Fatalf("failed to connect to mysql: %v", err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		t.Fatalf("mysql ping failed: %v", err)
	}

	_, _ = db.Exec("DROP DATABASE IF EXISTS myapp_dev;")
	_, _ = db.Exec("DROP DATABASE IF EXISTS myapp_dev_feature_orders;")
	_, err = db.Exec("CREATE DATABASE myapp_dev;")
	if err != nil {
		t.Fatalf("failed to create base database: %v", err)
	}

	_, err = db.Exec("CREATE TABLE myapp_dev.orders (id INT AUTO_INCREMENT PRIMARY KEY, item TEXT); INSERT INTO myapp_dev.orders (item) VALUES ('Laptop');")
	if err != nil {
		t.Fatalf("failed to seed orders table: %v", err)
	}

	myDrv, err := mysql.New(mysql.Config{
		Host:         myHost,
		Port:         myPort,
		User:         "root",
		Password:     "root",
		BaseDatabase: "myapp_dev",
	})
	if err != nil {
		t.Fatalf("mysql driver init failed: %v", err)
	}
	defer myDrv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := myDrv.CreateBranch(ctx, "main", "feature/orders"); err != nil {
		t.Fatalf("CreateBranch failed: %v", err)
	}

	var count int
	err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM myapp_dev_feature_orders.orders").Scan(&count)
	if err != nil || count != 1 {
		t.Fatalf("expected 1 cloned order in feature branch database, got count=%d, err=%v", count, err)
	}
}

func TestSQLiteE2EIntegration(t *testing.T) {
	tempDir := t.TempDir()
	basePath := filepath.Join(tempDir, "myapp_dev.db")
	
	db, err := sql.Open("sqlite", basePath)
	if err != nil {
		t.Fatalf("failed to create base sqlite db: %v", err)
	}
	_, err = db.Exec("CREATE TABLE inventory (id INTEGER PRIMARY KEY, name TEXT); INSERT INTO inventory (name) VALUES ('Keyboard');")
	if err != nil {
		t.Fatalf("failed to seed inventory table: %v", err)
	}
	db.Close() // close to release locks

	drv, err := sqlite.New(sqlite.Config{
		BasePath:      basePath,
		DefaultBranch: "main",
	})
	if err != nil {
		t.Fatalf("sqlite driver init failed: %v", err)
	}

	ctx := context.Background()
	if err := drv.CreateBranch(ctx, "main", "feature/stock"); err != nil {
		t.Fatalf("CreateBranch failed: %v", err)
	}

	featPath := filepath.Join(tempDir, "myapp_dev_feature_stock.db")
	featDB, err := sql.Open("sqlite", featPath)
	if err != nil {
		t.Fatalf("failed to connect to feat db: %v", err)
	}
	defer featDB.Close()

	var count int
	err = featDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM inventory").Scan(&count)
	if err != nil || count != 1 {
		t.Fatalf("expected 1 cloned inventory item in feature branch database, got count=%d, err=%v", count, err)
	}
}

func TestMongoDBE2EIntegration(t *testing.T) {
	mongoURI := os.Getenv("MONGO_URI")
	if mongoURI == "" {
		mongoURI = "mongodb://root:root@127.0.0.1:27018/admin?authSource=admin"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(mongoURI))
	if err != nil {
		t.Fatalf("failed to connect to mongodb: %v", err)
	}
	defer func() {
		_ = client.Disconnect(ctx)
	}()

	if err := client.Ping(ctx, nil); err != nil {
		t.Fatalf("mongodb ping failed: %v", err)
	}

	_ = client.Database("myapp_dev").Drop(ctx)
	_ = client.Database("myapp_dev_feature_users").Drop(ctx)

	coll := client.Database("myapp_dev").Collection("users")
	_, err = coll.InsertOne(ctx, bson.M{"name": "Bob"})
	if err != nil {
		t.Fatalf("failed to insert document: %v", err)
	}

	mongoDrv, err := mongodb.New(mongodb.Config{
		URI:           mongoURI,
		BaseDatabase:  "myapp_dev",
		DefaultBranch: "main",
	})
	if err != nil {
		t.Fatalf("mongodb driver init failed: %v", err)
	}
	defer mongoDrv.Close()

	if err := mongoDrv.CreateBranch(ctx, "main", "feature/users"); err != nil {
		t.Fatalf("CreateBranch failed: %v", err)
	}

	featColl := client.Database("myapp_dev_feature_users").Collection("users")
	count, err := featColl.CountDocuments(ctx, bson.M{})
	if err != nil || count != 1 {
		t.Fatalf("expected 1 cloned document in feature branch database, got count=%d, err=%v", count, err)
	}
}
