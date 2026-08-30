package config

import (
	"fmt"
	"net"
	"strconv"
	"time"

	"github.com/joho/godotenv"
	"github.com/kelseyhightower/envconfig"
)

type Settings struct {
	App      App
	Database Database
	Redis    Redis
	Log      Log
	CORS     CORS
	Worker   Worker
}

func Load() (*Settings, error) {
	_ = godotenv.Load()

	var settings Settings
	if err := envconfig.Process("", &settings); err != nil {
		return nil, fmt.Errorf("loading app config: %w", err)
	}

	return &settings, settings.validate()
}

func (s *Settings) validate() error {
	return nil
}

type App struct {
	Name            string        `envconfig:"APP_NAME"             default:"ecommerce-api"`
	Env             string        `envconfig:"APP_ENV"              default:"development"`
	Port            int           `envconfig:"APP_PORT"             default:"8080"`
	ReadTimeout     time.Duration `envconfig:"APP_READ_TIMEOUT"     default:"15s"`
	WriteTimeout    time.Duration `envconfig:"APP_WRITE_TIMEOUT"    default:"15s"`
	IdleTimeout     time.Duration `envconfig:"APP_IDLE_TIMEOUT"     default:"60s"`
	ShutdownTimeout time.Duration `envconfig:"APP_SHUTDOWN_TIMEOUT" default:"30s"`
}

type Database struct {
	Host                            string        `envconfig:"DB_HOST"                       default:"localhost"`
	Port                            int           `envconfig:"DB_PORT"                       default:"5432"`
	User                            string        `envconfig:"DB_USER"                       default:"postgres"`
	Password                        string        `envconfig:"DB_PASSWORD"                   default:"postgres"`
	Name                            string        `envconfig:"DB_NAME"                       default:"ecommerce"`
	SSLMode                         string        `envconfig:"DB_SSLMODE"                    default:"disable"`
	MaxConns                        int           `envconfig:"DB_MAX_CONNS"                  default:"25"`
	MinConns                        int           `envconfig:"DB_MIN_CONNS"                  default:"5"`
	MaxConnLifetime                 time.Duration `envconfig:"DB_MAX_CONN_LIFETIME"          default:"1h"`
	MaxConnIdleTime                 time.Duration `envconfig:"DB_MAX_CONN_IDLE_TIME"         default:"30m"`
	ReplicaURL                      string        `envconfig:"REPLICA_DATABASE_URL"          default:""`
	StatementTimeout                time.Duration `envconfig:"DB_STATEMENT_TIMEOUT"          default:"30s"`
	IdleInTransactionSessionTimeout time.Duration `envconfig:"DB_IDLE_IN_TX_SESSION_TIMEOUT" default:"60s"`
}

func (d Database) DSN() string {
	return fmt.Sprintf("postgres://%s:%s@%s/%s?sslmode=%s&statement_timeout=%d&idle_in_transaction_session_timeout=%d",
		d.User, d.Password, net.JoinHostPort(d.Host, strconv.Itoa(d.Port)), d.Name, d.SSLMode,
		d.StatementTimeout.Milliseconds(), d.IdleInTransactionSessionTimeout.Milliseconds())
}

type Redis struct {
	Host         string        `envconfig:"REDIS_HOST"           default:"localhost"`
	Port         int           `envconfig:"REDIS_PORT"           default:"6379"`
	Password     string        `envconfig:"REDIS_PASSWORD"       default:""`
	DB           int           `envconfig:"REDIS_DB"             default:"0"`
	PoolSize     int           `envconfig:"REDIS_POOL_SIZE"      default:"10"`
	MinIdleConns int           `envconfig:"REDIS_MIN_IDLE_CONNS" default:"2"`
	DialTimeout  time.Duration `envconfig:"REDIS_DIAL_TIMEOUT"   default:"5s"`
	ReadTimeout  time.Duration `envconfig:"REDIS_READ_TIMEOUT"   default:"3s"`
	WriteTimeout time.Duration `envconfig:"REDIS_WRITE_TIMEOUT"  default:"3s"`
	PoolTimeout  time.Duration `envconfig:"REDIS_POOL_TIMEOUT"   default:"4s"`
}

func (r Redis) Addr() string {
	return net.JoinHostPort(r.Host, strconv.Itoa(r.Port))
}

type Log struct {
	Level  string `envconfig:"LOG_LEVEL"  default:"info"`
	Format string `envconfig:"LOG_FORMAT" default:"json"`
}

type CORS struct {
	AllowedOrigins []string `envconfig:"CORS_ALLOWED_ORIGINS" default:"*"`
	AllowedMethods []string `envconfig:"CORS_ALLOWED_METHODS" default:"GET,POST,PUT,DELETE,OPTIONS"`
	AllowedHeaders []string `envconfig:"CORS_ALLOWED_HEADERS" default:"Content-Type,Authorization,X-Request-ID,Idempotency-Key"`
	MaxAge         int      `envconfig:"CORS_MAX_AGE"         default:"86400"`
}

type Worker struct {
	RescueAfter time.Duration `envconfig:"WORKER_RESCUE_AFTER" default:"5m"`
}
