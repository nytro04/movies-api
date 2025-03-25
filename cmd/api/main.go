package main

import (
	"context"
	"database/sql"
	"expvar"
	"flag"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
	"github.com/nytro04/greenlight/internal/data"
	"github.com/nytro04/greenlight/internal/jsonlog"
	"github.com/nytro04/greenlight/internal/mailer"
)

// buildTime is a string containing the date and time at which the binary was built.
var (
	buildTime string
	version   string
)

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
	smtp struct {
		host     string // SMTP server address
		port     int    // SMTP port
		username string // SMTP username
		password string // SMTP password
		sender   string // email address to send from
	}

	cors struct {
		trustedOrigins []string
	}
}

type application struct {
	config config
	logger *jsonlog.Logger
	models data.Models
	mailer mailer.Mailer
	wg     sync.WaitGroup
}

func main() {
	var cfg config

	logger := jsonlog.New(os.Stdout, jsonlog.LevelInfo)

	flag.IntVar(&cfg.port, "port", 4000, "API server port")
	flag.StringVar(&cfg.env, "env", "development", "Environment (development|staging|production)")

	// flag.StringVar(&cfg.db.dsn, "db-dsn", "", "PostgreSQL DSN")

	// Read the connection pool settings from command-line flags into the config struct.
	// The connection pool settings are used to configure the connection pool that the application will use to connect to the PostgreSQL database.
	// The maxOpenConns setting is used to set the maximum number of open connections in the pool. and the maxIdleConns setting is used to set the maximum number of idle connections in the pool.
	// The maxIdleTime setting is used to set the maximum amount of time that a connection can remain idle in the pool before it is closed and removed from the pool.
	flag.IntVar(&cfg.db.maxOpenConns, "db-max-open-conns", 25, "PostgreSQL max open connections")
	flag.IntVar(&cfg.db.maxIdleConns, "db-max-idle-conns", 25, "PostgreSQL max idle connections")
	flag.StringVar(&cfg.db.maxIdleTime, "db-max-idle-time", "15m", "PostgreSQL max connection idle time")

	// The rate limiter middleware is used to limit the number of requests that a client can make to the API within a given time window.
	// The rate limiter settings are used to configure the rate limiter middleware. settings from command-line flags into the config struct.
	flag.Float64Var(&cfg.limiter.rps, "limiter-rps", 2, "Rate limiter maximu requests per second")
	flag.IntVar(&cfg.limiter.burst, "limit-burst", 4, "Rte limiter maximum burst")
	flag.BoolVar(&cfg.limiter.enabled, "limiter-enabled", true, "Enable rate limiter")

	// Read the SMTP server settings from command-line flags into the config struct.
	// The SMTP server settings are used to configure the SMTP server that the application will use to send emails.
	flag.StringVar(&cfg.smtp.host, "smtp-host", "sandbox.smtp.mailtrap.io", "SMTP host")
	flag.IntVar(&cfg.smtp.port, "smtp-port", 25, "SMTP port")
	flag.StringVar(&cfg.smtp.username, "smtp-username", "", "SMTP username")
	flag.StringVar(&cfg.smtp.password, "smtp-password", "", "SMTP password")
	flag.StringVar(&cfg.smtp.sender, "smtp-sender", "nytro04@gmail.com", "SMTP sender")

	// use teh flag.Func to process the cors-trusted-origins flag. use strings fields to split the space-separated list of origins into a slice of strings and assign it to the config struct.
	// if the flag is not provided, i.e empty string, white space, the trustedOrigins field will be an empty slice.
	// The CORS settings are used to configure Cross-Origin Resource Sharing (CORS) for the API server.
	flag.Func("cors-trusted-origins", "Trusted CORS origins (space-separated)", func(val string) error {
		cfg.cors.trustedOrigins = strings.Fields(val)
		return nil
	})

	// create a new version boolean flag with a default value of false
	displayVersion := flag.Bool("version", false, "Display version and exit")

	err := godotenv.Load()
	if err != nil {
		logger.PrintFatal(err, map[string]string{"message": "Error loading .env file"})
	}

	// Read the connection pool settings, rate limiter settings, and other configuration settings from environment variables.
	var (
		dbHost = os.Getenv("DB_HOST")
		// dbPort             = os.Getenv("DB_PORT")
		dbUser             = os.Getenv("DB_USER")
		dbPassword         = os.Getenv("DB_PASSWORD")
		dbName             = os.Getenv("DB_NAME")
		limiterRPS         = os.Getenv("LIMITER_RPS")
		limiterBurst       = os.Getenv("LIMITER_BURST")
		limiterEnabled     = os.Getenv("LIMITER_ENABLED")
		SMTPHost           = os.Getenv("SMTP_HOST")
		SMTPPortStr        = os.Getenv("SMTP_PORT")
		SMTPUsername       = os.Getenv("SMTP_USERNAME")
		SMTPPassword       = os.Getenv("SMTP_PASSWORD")
		CORSTrustedOrigins = os.Getenv("CORS_TRUSTED_ORIGINS")
		// SMTPSender     = os.Getenv("SMTP_SENDER")
	)

	// Construct the PostgreSQL DSN from the environment variables.
	dsn := fmt.Sprintf("postgres://%s:%s@%s/%s?sslmode=disable", dbUser, dbPassword, dbHost, dbName)
	// dsn := fmt.Sprintf("host=db user=%s password=%s port=%s dbname=%s sslmode=disable", dbUser, dbPassword, dbName, dbPort)

	// construct the PostgreSQL DSN from the terminal flags
	flag.StringVar(&cfg.db.dsn, "db-dsn", "", "PostgreSQL DSN")

	// Parse the command-line flags
	flag.Parse()

	// if the version flag is true, print the version and exit
	if *displayVersion {
		fmt.Printf("Version:\t%s\n", version)
		fmt.Printf("Build time:\t%s\n", buildTime)
		os.Exit(0)
	}

	// assign cgf.db.dsn to the dsn variable
	cfg.db.dsn = dsn

	// assign the trusted origins to the config struct
	cfg.cors.trustedOrigins = strings.Fields(CORSTrustedOrigins)

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

	// open a connection to the database and defer the close
	db, err := openDB(cfg)
	if err != nil {
		logger.PrintFatal(err, map[string]string{"message": "Error opening database connection"})
	}

	defer db.Close()
	logger.PrintInfo("database connection pool established", nil)

	// convert the SMTP port from a string to an integer
	SMTPPort, err := strconv.Atoi(SMTPPortStr)
	if err != nil {
		logger.PrintError(err, map[string]string{"message": "Invalid value for SMTP_PORT"})
	}

	// add a version variable to the expvar package to expose the application version
	expvar.NewString("version").Set(version)

	// publish the number of goroutines to the expvar package
	expvar.Publish("goroutines", expvar.Func(func() any {
		return runtime.NumGoroutine()
	}))

	// publish the database statistics to the expvar package
	expvar.Publish("database", expvar.Func(func() any {
		return db.Stats()
	}))

	// publish the current unix timestamp to the expvar package
	expvar.Publish("timestamp", expvar.Func(func() any {
		return time.Now().Unix()
	}))

	// create a new application struct and pass all the dependencies
	app := &application{
		config: cfg,
		logger: logger,
		models: data.NewModels(db),
		// mailer: mailer.New(cfg.smtp.host, cfg.smtp.port, cfg.smtp.username, cfg.smtp.password, cfg.smtp.sender), // use this when using command line flags
		mailer: mailer.New(SMTPHost, SMTPPort, SMTPUsername, SMTPPassword, cfg.smtp.sender), // use this when using environment variables
	}

	// call the serve method on the application struct
	err = app.serve()
	if err != nil {
		logger.PrintFatal(err, map[string]string{"message": "server shutdown with error"})
	}

}

// openDB opens a new database connection using the provided DSN. It returns a sql.DB connection pool.
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
