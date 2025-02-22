package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
	"github.com/nytro04/greenlight/internal/data"
	"github.com/nytro04/greenlight/internal/jsonlog"
)

const version = "1.0.0"

type config struct {
	port int
	env  string
	db   struct {
		dsn          string // data source name
		maxOpenConns int
		maxIdleConns int
		maxIdleTime  string
	}

	limiter struct {
		rps     float64 // requests per second
		burst   int     // burst
		enabled bool
	}
}

type application struct {
	config config
	logger *jsonlog.Logger
	models data.Models
}

func main() {
	var cfg config

	logger := jsonlog.New(os.Stdout, jsonlog.LevelInfo)

	flag.IntVar(&cfg.port, "port", 4000, "API server port")
	flag.StringVar(&cfg.env, "env", "development", "Environment (development|staging|production)")

	// flag.StringVar(&cfg.db.dsn, "db-dsn", "", "PostgreSQL DSN")

	// Read the connection pool settings from command-line flags into the config struct.
	flag.IntVar(&cfg.db.maxOpenConns, "db-max-open-conns", 25, "PostgreSQL max open connections")
	flag.IntVar(&cfg.db.maxIdleConns, "db-max-idle-conns", 25, "PostgreSQL max idle connections")
	flag.StringVar(&cfg.db.maxIdleTime, "db-max-idle-time", "15m", "PostgreSQL max connection idle time")

	// Read the rate limiter settings from command-line flags into the config struct.
	flag.Float64Var(&cfg.limiter.rps, "limiter-rps", 2, "Rate limiter maximu requests per second")
	flag.IntVar(&cfg.limiter.burst, "limit-burst", 4, "Rte limiter maximum burst")
	flag.BoolVar(&cfg.limiter.enabled, "limiter-enabled", true, "Enable rate limiter")

	err := godotenv.Load()
	if err != nil {
		logger.PrintFatal(err, map[string]string{"message": "Error loading .env file"})
	}

	var (
		dbHost = os.Getenv("DB_HOST")
		// dbPort     = os.Getenv("DB_PORT")
		dbUser         = os.Getenv("DB_USER")
		dbPassword     = os.Getenv("DB_PASSWORD")
		dbName         = os.Getenv("DB_NAME")
		limiterRPS     = os.Getenv("LIMITER_RPS")
		limiterBurst   = os.Getenv("LIMITER_BURST")
		limiterEnabled = os.Getenv("LIMITER_ENABLED")
	)

	dsn := fmt.Sprintf("postgres://%s:%s@%s/%s?sslmode=disable", dbUser, dbPassword, dbHost, dbName)

	flag.StringVar(&cfg.db.dsn, "db-dsn", dsn, "PostgreSQL DSN")

	flag.Parse()

	cfg.db.dsn = dsn

	// add rate limiter settings from environment variables
	cfg.limiter.rps, err = strconv.ParseFloat(limiterRPS, 64)
	if err != nil {
		logger.PrintFatal(err, map[string]string{"message": "Invalid value for LIMITER_RPS"})
	}
	cfg.limiter.burst, err = strconv.Atoi(limiterBurst)
	if err != nil {
		logger.PrintFatal(err, map[string]string{"message": "Invalid value for LIMITER_BURST"})
	}
	cfg.limiter.enabled, err = strconv.ParseBool(limiterEnabled)
	if err != nil {
		logger.PrintFatal(err, map[string]string{"message": "Invalid value for LIMITER_ENABLED"})
	}

	db, err := openDB(cfg)
	if err != nil {
		logger.PrintFatal(err, nil)
	}

	defer db.Close()
	logger.PrintInfo("database connection pool established", nil)

	app := &application{
		config: cfg,
		logger: logger,
		models: data.NewModels(db),
	}

	// call the serve method on the application struct
	err = app.serve()
	if err != nil {
		logger.PrintFatal(err, nil)
	}

	// srv := &http.Server{
	// 	Addr:         fmt.Sprintf(":%d", cfg.port),
	// 	Handler:      app.routes(),
	// 	ErrorLog:     log.New(logger, "", 0),
	// 	IdleTimeout:  time.Minute,
	// 	ReadTimeout:  10 * time.Second,
	// 	WriteTimeout: 30 * time.Second,
	// }

	// // Start the server
	// logger.PrintInfo("starting server", map[string]string{
	// 	"addr": srv.Addr,
	// 	"env":  cfg.env,
	// })
	// err = srv.ListenAndServe()
	// logger.PrintFatal(err, nil)
}

func openDB(cfg config) (*sql.DB, error) {
	// Open a sql.DB connection pool
	db, err := sql.Open("postgres", cfg.db.dsn)
	if err != nil {
		return nil, err
	}

	// Set the maximum number of open (in-use + idle) connections in the pool.
	db.SetMaxOpenConns(cfg.db.maxOpenConns)

	// Set the maximum number of idle connections in the pool.
	db.SetMaxIdleConns(cfg.db.maxIdleConns)

	duration, err := time.ParseDuration(cfg.db.maxIdleTime)
	if err != nil {
		return nil, err
	}

	// set the maximum idle timeout
	db.SetConnMaxIdleTime(duration)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Ping the database with context above to check if the connection is working,
	//if the couldnt be established after 5 seconds, this will return an error
	err = db.PingContext(ctx)
	if err != nil {
		return nil, err
	}

	// return the sql.DB connection pool
	return db, nil
}
