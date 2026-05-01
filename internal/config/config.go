package config

import (
	"errors"
	"fmt"
	"net"
	"strconv"
	"time"

	"github.com/joho/godotenv"
	"github.com/kelseyhightower/envconfig"
)

type Config struct {
	App      AppConfig
	Database DatabaseConfig
	Redis    RedisConfig
	JWT      JWTConfig
	Log      LogConfig
	CORS     CORSConfig
	Worker   WorkerConfig
	Payment  PaymentConfig
}

type AppConfig struct {
	Name            string        `envconfig:"APP_NAME" default:"ecommerce-api"`
	Env             string        `envconfig:"APP_ENV" default:"development"`
	Port            int           `envconfig:"APP_PORT" default:"8080"`
	ReadTimeout     time.Duration `envconfig:"APP_READ_TIMEOUT" default:"15s"`
	WriteTimeout    time.Duration `envconfig:"APP_WRITE_TIMEOUT" default:"15s"`
	IdleTimeout     time.Duration `envconfig:"APP_IDLE_TIMEOUT" default:"60s"`
	ShutdownTimeout time.Duration `envconfig:"APP_SHUTDOWN_TIMEOUT" default:"30s"`
	MaxCartItems    int           `envconfig:"MAX_CART_ITEMS" default:"50"`
	OrderRateLimit  int           `envconfig:"ORDER_RATE_LIMIT" default:"5"`
}

type DatabaseConfig struct {
	Host                            string        `envconfig:"DB_HOST" default:"localhost"`
	Port                            int           `envconfig:"DB_PORT" default:"5432"`
	User                            string        `envconfig:"DB_USER" default:"postgres"`
	Password                        string        `envconfig:"DB_PASSWORD" default:"postgres"`
	Name                            string        `envconfig:"DB_NAME" default:"ecommerce"`
	SSLMode                         string        `envconfig:"DB_SSLMODE" default:"disable"`
	MaxConns                        int           `envconfig:"DB_MAX_CONNS" default:"25"`
	MinConns                        int           `envconfig:"DB_MIN_CONNS" default:"5"`
	MaxConnLifetime                 time.Duration `envconfig:"DB_MAX_CONN_LIFETIME" default:"1h"`
	MaxConnIdleTime                 time.Duration `envconfig:"DB_MAX_CONN_IDLE_TIME" default:"30m"`
	ReaderURL                       string        `envconfig:"READER_DATABASE_URL" default:""`
	StatementTimeout                time.Duration `envconfig:"DB_STATEMENT_TIMEOUT" default:"30s"`
	IdleInTransactionSessionTimeout time.Duration `envconfig:"DB_IDLE_IN_TX_SESSION_TIMEOUT" default:"60s"`
}

func (d DatabaseConfig) DSN() string {
	return fmt.Sprintf("postgres://%s:%s@%s/%s?sslmode=%s&statement_timeout=%d&idle_in_transaction_session_timeout=%d",
		d.User, d.Password, net.JoinHostPort(d.Host, strconv.Itoa(d.Port)), d.Name, d.SSLMode,
		d.StatementTimeout.Milliseconds(), d.IdleInTransactionSessionTimeout.Milliseconds())
}

type RedisConfig struct {
	Host     string `envconfig:"REDIS_HOST" default:"localhost"`
	Port     int    `envconfig:"REDIS_PORT" default:"6379"`
	Password string `envconfig:"REDIS_PASSWORD" default:""`
	DB       int    `envconfig:"REDIS_DB" default:"0"`
}

func (r RedisConfig) Addr() string {
	return net.JoinHostPort(r.Host, strconv.Itoa(r.Port))
}

type JWTConfig struct {
	Secret          string        `envconfig:"JWT_SECRET" required:"true"`
	AccessTokenTTL  time.Duration `envconfig:"JWT_ACCESS_TTL" default:"15m"`
	RefreshTokenTTL time.Duration `envconfig:"JWT_REFRESH_TTL" default:"168h"`
	Issuer          string        `envconfig:"JWT_ISSUER" default:"ecommerce-api"`
}

type LogConfig struct {
	Level  string `envconfig:"LOG_LEVEL" default:"info"`
	Format string `envconfig:"LOG_FORMAT" default:"json"`
}

type CORSConfig struct {
	AllowedOrigins []string `envconfig:"CORS_ALLOWED_ORIGINS" default:"*"`
	AllowedMethods []string `envconfig:"CORS_ALLOWED_METHODS" default:"GET,POST,PUT,DELETE,OPTIONS"`
	AllowedHeaders []string `envconfig:"CORS_ALLOWED_HEADERS" default:"Content-Type,Authorization,X-Request-ID,Idempotency-Key"`
	MaxAge         int      `envconfig:"CORS_MAX_AGE" default:"86400"`
}

type WorkerConfig struct {
	Interval      time.Duration `envconfig:"WORKER_INTERVAL" default:"10s"`
	BatchSize     int           `envconfig:"WORKER_BATCH_SIZE" default:"10"`
	LeaseDuration time.Duration `envconfig:"WORKER_LEASE_DURATION" default:"2m"`
	Concurrency   int           `envconfig:"WORKER_CONCURRENCY" default:"5"`
}

type PaymentConfig struct {
	Gateway        string        `envconfig:"PAYMENT_GATEWAY" default:"mock"`
	GatewayURL     string        `envconfig:"PAYMENT_GATEWAY_URL" default:"http://localhost:8080/mock/payment"`
	GatewayTimeout time.Duration `envconfig:"PAYMENT_GATEWAY_TIMEOUT" default:"10s"`
	GatewayAPIKey  string        `envconfig:"PAYMENT_GATEWAY_API_KEY" default:""`
	WebhookSecret  string        `envconfig:"PAYMENT_WEBHOOK_SECRET" default:"webhook-secret"`
}

func Load() (*Config, error) {
	_ = godotenv.Load()

	var cfg Config
	if err := envconfig.Process("", &cfg); err != nil {
		return nil, fmt.Errorf("loading config: %w", err)
	}

	if err := cfg.validate(); err != nil {
		return nil, err
	}

	return &cfg, nil
}

func (c *Config) validate() error {
	if c.App.Env != "development" && c.Payment.WebhookSecret == "webhook-secret" {
		return errors.New("PAYMENT_WEBHOOK_SECRET must be set in non-development environments")
	}

	if c.Worker.LeaseDuration < c.Payment.GatewayTimeout*3 {
		return errors.New("WORKER_LEASE_DURATION must be at least 3× PAYMENT_GATEWAY_TIMEOUT to avoid duplicate gateway calls")
	}

	if c.Worker.Interval < 5*time.Second {
		return errors.New("WORKER_INTERVAL must be at least 5s to avoid database polling overhead")
	}

	return nil
}
