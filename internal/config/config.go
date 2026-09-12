// Package config loads all runtime configuration from environment variables,
// per twelve-factor "store config in the environment" (see docs/twelve-factor.md).
// No file-based config, no hardcoded secrets.
package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	Env  string // "development" | "test" | "production"
	HTTP HTTPConfig
	GRPC GRPCConfig
	MySQL MySQLConfig
	Redis RedisConfig
	Mongo MongoConfig
	Kafka KafkaConfig
	RabbitMQ RabbitMQConfig
	Elasticsearch ElasticsearchConfig
	Auth  AuthConfig
	OTel  OTelConfig
}

type HTTPConfig struct {
	Addr            string
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	ShutdownTimeout time.Duration
	CORSOrigins     []string
}

type GRPCConfig struct {
	Addr string
	// SearchIndexerAddr and NotificationAddr are the monolith's outbound
	// gRPC client targets — empty means that service isn't configured, and
	// callers (internal/search.Service, internal/notification) treat that
	// as "always use the fallback/no-op path" rather than erroring.
	SearchIndexerAddr string
	NotificationAddr  string
}

type MySQLConfig struct {
	PrimaryDSN string
	ReplicaDSN string // falls back to PrimaryDSN when unset (single-node dev mode)
	MaxOpenConns int
	MaxIdleConns int
}

type RedisConfig struct {
	Addr     string
	Password string
	DB       int
}

type MongoConfig struct {
	URI      string
	Database string
}

type KafkaConfig struct {
	Brokers []string
}

type RabbitMQConfig struct {
	URL string
}

type ElasticsearchConfig struct {
	Addresses []string
}

type AuthConfig struct {
	JWTSecret          string
	JWTAccessTTL       time.Duration
	JWTRefreshTTL      time.Duration
	BcryptCost         int
	APIKeyPepper       string
	SessionSecret      string
	OAuthGoogleClientID     string
	OAuthGoogleClientSecret string
	OAuthGoogleRedirectURL  string
	SAMLCertPath string
	SAMLKeyPath  string
	SAMLIDPMetadataURL string
}

type OTelConfig struct {
	ServiceName    string
	OTLPEndpoint   string
	Enabled        bool
}

// Load reads configuration from the environment, applying sane local-dev
// defaults so `go run ./cmd/api` works against docker-compose out of the box.
func Load() (*Config, error) {
	cfg := &Config{
		Env: getEnv("APP_ENV", "development"),
		HTTP: HTTPConfig{
			Addr:            getEnv("HTTP_ADDR", ":8080"),
			ReadTimeout:     getDuration("HTTP_READ_TIMEOUT", 10*time.Second),
			WriteTimeout:    getDuration("HTTP_WRITE_TIMEOUT", 10*time.Second),
			ShutdownTimeout: getDuration("HTTP_SHUTDOWN_TIMEOUT", 15*time.Second),
			CORSOrigins:     []string{getEnv("CORS_ORIGIN", "http://localhost:8081")},
		},
		GRPC: GRPCConfig{
			Addr:              getEnv("GRPC_ADDR", ":9090"),
			SearchIndexerAddr: getEnv("SEARCH_INDEXER_ADDR", ""),
			NotificationAddr:  getEnv("NOTIFICATION_ADDR", ""),
		},
		MySQL: MySQLConfig{
			PrimaryDSN:   getEnv("MYSQL_PRIMARY_DSN", "gearshare_app:app_password@tcp(127.0.0.1:3306)/gearshare?parseTime=true&multiStatements=true"),
			ReplicaDSN:   getEnv("MYSQL_REPLICA_DSN", ""),
			MaxOpenConns: getInt("MYSQL_MAX_OPEN_CONNS", 25),
			MaxIdleConns: getInt("MYSQL_MAX_IDLE_CONNS", 10),
		},
		Redis: RedisConfig{
			Addr:     getEnv("REDIS_ADDR", "127.0.0.1:6379"),
			Password: getEnv("REDIS_PASSWORD", ""),
			DB:       getInt("REDIS_DB", 0),
		},
		Mongo: MongoConfig{
			URI:      getEnv("MONGO_URI", "mongodb://127.0.0.1:27017"),
			Database: getEnv("MONGO_DATABASE", "gearshare"),
		},
		Kafka: KafkaConfig{
			Brokers: []string{getEnv("KAFKA_BROKERS", "127.0.0.1:9092")},
		},
		RabbitMQ: RabbitMQConfig{
			URL: getEnv("RABBITMQ_URL", "amqp://guest:guest@127.0.0.1:5672/"),
		},
		Elasticsearch: ElasticsearchConfig{
			Addresses: []string{getEnv("ELASTICSEARCH_ADDR", "http://127.0.0.1:9200")},
		},
		Auth: AuthConfig{
			JWTSecret:               getEnv("JWT_SECRET", "dev-only-change-me"),
			JWTAccessTTL:            getDuration("JWT_ACCESS_TTL", 15*time.Minute),
			JWTRefreshTTL:           getDuration("JWT_REFRESH_TTL", 7*24*time.Hour),
			BcryptCost:              getInt("BCRYPT_COST", 12),
			APIKeyPepper:            getEnv("API_KEY_PEPPER", "dev-only-change-me"),
			SessionSecret:           getEnv("SESSION_SECRET", "dev-only-change-me"),
			OAuthGoogleClientID:     getEnv("OAUTH_GOOGLE_CLIENT_ID", ""),
			OAuthGoogleClientSecret: getEnv("OAUTH_GOOGLE_CLIENT_SECRET", ""),
			OAuthGoogleRedirectURL:  getEnv("OAUTH_GOOGLE_REDIRECT_URL", "http://localhost:8080/api/v1/auth/oauth/google/callback"),
			SAMLCertPath:            getEnv("SAML_CERT_PATH", "deployments/idp/saml/sp.crt"),
			SAMLKeyPath:             getEnv("SAML_KEY_PATH", "deployments/idp/saml/sp.key"),
			SAMLIDPMetadataURL:      getEnv("SAML_IDP_METADATA_URL", ""),
		},
		OTel: OTelConfig{
			ServiceName:  getEnv("OTEL_SERVICE_NAME", "gearshare-api"),
			OTLPEndpoint: getEnv("OTEL_EXPORTER_OTLP_ENDPOINT", "127.0.0.1:4318"),
			Enabled:      getBool("OTEL_ENABLED", true),
		},
	}

	if cfg.MySQL.ReplicaDSN == "" {
		cfg.MySQL.ReplicaDSN = cfg.MySQL.PrimaryDSN
	}
	if cfg.Env == "production" && cfg.Auth.JWTSecret == "dev-only-change-me" {
		return nil, fmt.Errorf("config: JWT_SECRET must be set explicitly in production")
	}
	return cfg, nil
}

func getEnv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}

func getInt(key string, fallback int) int {
	if v, ok := os.LookupEnv(key); ok {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}

func getBool(key string, fallback bool) bool {
	if v, ok := os.LookupEnv(key); ok {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return fallback
}

func getDuration(key string, fallback time.Duration) time.Duration {
	if v, ok := os.LookupEnv(key); ok {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return fallback
}
