package compose

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/branchbase/branchbase/internal/config"
	"gopkg.in/yaml.v3"
)

// ComposeService represents a service block inside docker-compose.yml
type ComposeService struct {
	Image       string    `yaml:"image"`
	Ports       []string  `yaml:"ports"`
	Environment yaml.Node `yaml:"environment"`
	EnvFile     yaml.Node `yaml:"env_file"`
}

// ComposeFile represents the top-level docker-compose.yml structure
type ComposeFile struct {
	Version  string                    `yaml:"version"`
	Services map[string]ComposeService `yaml:"services"`
}

// DatabaseService represents a detected database engine from Docker Compose
type DatabaseService struct {
	Driver       string // "postgres", "mysql", "mariadb"
	Host         string // usually "127.0.0.1"
	Port         int    // published host port (e.g. 5433 or 3306)
	InternalPort int    // container port (e.g. 5432 or 3306)
	User         string
	Password     string
	Database     string
	ServiceName  string
}

// ComposeFileNames lists the standard compose filenames to search for
var ComposeFileNames = []string{
	"docker-compose.yml",
	"docker-compose.yaml",
	"compose.yml",
	"compose.yaml",
}

// DetectCompose scans the directory for Docker Compose files and identifies database services.
func DetectCompose(repoPath string) (*DatabaseService, string, error) {
	for _, filename := range ComposeFileNames {
		fullPath := filepath.Join(repoPath, filename)
		data, err := os.ReadFile(fullPath)
		if err != nil {
			continue
		}

		var comp ComposeFile
		if err := yaml.Unmarshal(data, &comp); err != nil {
			continue
		}

		if db := extractDatabaseService(&comp); db != nil {
			return db, filename, nil
		}
	}

	return nil, "", nil
}

func extractDatabaseService(comp *ComposeFile) *DatabaseService {
	for name, svc := range comp.Services {
		img := strings.ToLower(svc.Image)
		svcName := strings.ToLower(name)

		var drv string
		var defaultPort int

		if strings.Contains(img, "postgres") || strings.Contains(svcName, "postgres") || strings.Contains(svcName, "pg") {
			drv = "postgres"
			defaultPort = 5432
		} else if strings.Contains(img, "mariadb") || strings.Contains(svcName, "mariadb") {
			drv = "mariadb"
			defaultPort = 3306
		} else if strings.Contains(img, "mysql") || strings.Contains(svcName, "mysql") {
			drv = "mysql"
			defaultPort = 3306
		}

		if drv == "" {
			continue
		}

		envMap := parseEnvironment(&svc.Environment)
		hostPort := parsePublishedPort(svc.Ports, defaultPort)

		dbSvc := &DatabaseService{
			Driver:       drv,
			Host:         "127.0.0.1",
			Port:         hostPort,
			InternalPort: defaultPort,
			ServiceName:  name,
		}

		switch drv {
		case "postgres":
			dbSvc.User = getFirstNonEmpty(envMap["POSTGRES_USER"], "postgres")
			dbSvc.Password = envMap["POSTGRES_PASSWORD"]
			dbSvc.Database = getFirstNonEmpty(envMap["POSTGRES_DB"], "myapp_dev")
		case "mysql", "mariadb":
			dbSvc.User = getFirstNonEmpty(envMap["MYSQL_USER"], "root")
			dbSvc.Password = getFirstNonEmpty(envMap["MYSQL_PASSWORD"], envMap["MYSQL_ROOT_PASSWORD"])
			dbSvc.Database = getFirstNonEmpty(envMap["MYSQL_DATABASE"], "myapp_dev")
		}

		return dbSvc
	}

	return nil
}

func parseEnvironment(node *yaml.Node) map[string]string {
	res := make(map[string]string)
	if node == nil {
		return res
	}

	switch node.Kind {
	case yaml.MappingNode:
		for i := 0; i < len(node.Content); i += 2 {
			k := node.Content[i].Value
			v := node.Content[i+1].Value
			res[k] = v
		}
	case yaml.SequenceNode:
		for _, item := range node.Content {
			val := item.Value
			parts := strings.SplitN(val, "=", 2)
			if len(parts) == 2 {
				res[parts[0]] = parts[1]
			}
		}
	}
	return res
}

func parsePublishedPort(ports []string, defaultPort int) int {
	for _, p := range ports {
		parts := strings.Split(p, ":")
		if len(parts) >= 2 {
			hostPortStr := parts[len(parts)-2]
			if val, err := strconv.Atoi(hostPortStr); err == nil && val > 0 {
				return val
			}
		} else if len(parts) == 1 {
			if val, err := strconv.Atoi(parts[0]); err == nil && val > 0 {
				return val
			}
		}
	}
	return defaultPort
}

func getFirstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// ApplyToConfig populates a Config struct with values extracted from Docker Compose.
func ApplyToConfig(cfg *config.Config, dbSvc *DatabaseService) {
	if cfg == nil || dbSvc == nil {
		return
	}

	cfg.Driver = dbSvc.Driver
	cfg.Connection.Host = dbSvc.Host
	cfg.Connection.Port = dbSvc.Port
	cfg.Connection.User = dbSvc.User
	cfg.Connection.Password = dbSvc.Password
	cfg.Connection.BaseDatabase = dbSvc.Database

}
