package main

import (
	"context"
	"database/sql"
	"errors"
	"expvar"
	"flag"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
	"github.com/nytro04/greenlight/assets"
	"github.com/nytro04/greenlight/internal/data"
	"github.com/nytro04/greenlight/internal/jsonlog"
	"github.com/nytro04/greenlight/internal/mailer"
)

// buildTime is a string containing the date and time at which the binary was built.
// Read the connection pool settings, rate limiter settings, and other configuration settings from environment variables.

var (
	// buildTime string
	version string
	// env      string
	// dbDSN             = os.Getenv("DB_DSN")
	// dbPort     = os.Getenv("DB_PORT")
	// dbHost     = os.Getenv("DB_HOST")
	// dbUser     = os.Getenv("DB_USER")
	// dbPassword = os.Getenv("DB_PASSWORD")
	// dbName     = os.Getenv("DB_NAME")
	// httpPort   = os.Getenv("HTTP_PORT")
	// limiterRPS         = os.Getenv("LIMITER_RPS")
	// limiterBurst       = os.Getenv("LIMITER_BURST")
	// limiterEnabled     = os.Getenv("LIMITER_ENABLED")
	// SMTPHost           = os.Getenv("SMTP_HOST")
	// SMTPPortStr        = os.Getenv("SMTP_PORT")
	// SMTPUsername       = os.Getenv("SMTP_USERNAME")
	// SMTPPassword       = os.Getenv("SMTP_PASSWORD")
	// CORSTrustedOrigins = os.Getenv("CORS_TRUSTED_ORIGINS")
	// SMTPSender     = os.Getenv("SMTP_SENDER")
	// environment    = os.Getenv("environment")
	// dbMaxIdleTime  = os.Getenv("DB_MAX_IDLE_TIME")
	// dbMaxOpenConns = os.Getenv("DB_MAX_OPEN_CONNS")
	// dbMaxIdleConns = os.Getenv("DB_MAX_IDLE_CONNS")
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

	var (
		dbHost = os.Getenv("DB_HOST")
		// dbUser     = os.Getenv("DB_USER")
		// dbPassword = os.Getenv("DB_PASSWORD")
		// dbName     = os.Getenv("DB_NAME")
	)

	env := os.Getenv("environment")
	if env == "" {
		env = "development"
	}

	// Load the .env file only in development
	if env == "development" {
		err := godotenv.Load()
		if err != nil {
			logger.PrintFatal(err, map[string]string{"message": "Error loading .env file"})
		}
	}

	if env == "development" {
		dbHost = "localhost"
	}

	// fmt.Printf("db user:\t%s\n", os.Getenv("DB_USER"))
	// fmt.Printf("db password:\t%s\n", os.Getenv("DB_PASSWORD"))
	// fmt.Printf("db host:\t%s\n", os.Getenv("DB_HOST"))
	// fmt.Printf("db port:\t%s\n", os.Getenv("DB_PORT"))
	// fmt.Printf("db name:\t%s\n", os.Getenv("DB_NAME"))
	// fmt.Printf("db host:\t%s\n", os.Getenv("DB_HOST"))

	// use DATABASE_URL for railway
	var dsn string
	if env == "development" {
		dsn = fmt.Sprintf("postgres://%s:%s@%s/%s?sslmode=disable", os.Getenv("DB_USER"), os.Getenv("DB_PASSWORD"), dbHost, os.Getenv("DB_NAME"))

	} else {

		dsn = os.Getenv("DATABASE_URL")
	}
	// use the environment variables for local development
	// dsn := fmt.Sprintf("postgres://%s:%s@%s/%s?sslmode=disable", dbUser, dbPassword, dbHost, dbName)

	httpPort := os.Getenv("HTTP_PORT")
	intHttpPort, _ := strconv.Atoi(httpPort)
	flag.IntVar(&cfg.port, "port", intHttpPort, "API server port")
	flag.StringVar(&cfg.env, "env", env, "Environment (development|staging|production)")

	flag.StringVar(&cfg.db.dsn, "db-dsn", dsn, "PostgreSQL DSN")

	// fmt.Printf("intPort:\t%d\n", intHttpPort)
	// fmt.Printf("cfg port:\t%d\n", cfg.port)

	// fmt.Printf("dsn:\t%s\n", dsn) works
	// fmt.Printf("cfg dsn:\t%s\n", cfg.db.dsn)

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

	smtpPort, _ := strconv.Atoi(os.Getenv("SMTP_PORT"))
	// Read the SMTP server settings from command-line flags into the config struct.
	// The SMTP server settings are used to configure the SMTP server that the application will use to send emails.
	flag.StringVar(&cfg.smtp.host, "smtp-host", os.Getenv("SMTP_HOST"), "SMTP host")
	flag.IntVar(&cfg.smtp.port, "smtp-port", smtpPort, "SMTP port")
	flag.StringVar(&cfg.smtp.username, "smtp-username", os.Getenv("SMTP_USERNAME"), "SMTP username")
	flag.StringVar(&cfg.smtp.password, "smtp-password", os.Getenv("SMTP_PASSWORD"), "SMTP password")
	flag.StringVar(&cfg.smtp.sender, "smtp-sender", os.Getenv("SMTP_SENDER"), "SMTP sender")

	// use teh flag.Func to process the cors-trusted-origins flag. use strings fields to split the space-separated list of origins into a slice of strings and assign it to the config struct.
	// if the flag is not provided, i.e empty string, white space, the trustedOrigins field will be an empty slice.
	// The CORS settings are used to configure Cross-Origin Resource Sharing (CORS) for the API server.
	flag.Func("cors-trusted-origins", "Trusted CORS origins (space-separated)", func(val string) error {
		cfg.cors.trustedOrigins = strings.Fields(val)
		return nil
	})

	// create a new version boolean flag with a default value of false
	displayVersion := flag.Bool("version", false, "Display version and exit")

	// get automigrate from env
	automigrate := os.Getenv("AUTO_MIGRATE")
	automigrateBool, _ := strconv.ParseBool(automigrate)

	// Construct the PostgreSQL DSN from the environment variables.
	// dsn := fmt.Sprintf("host=db user=%s password=%s port=%s dbname=%s sslmode=disable", dbUser, dbPassword, dbName, dbPort)

	// construct the PostgreSQL DSN from the terminal flags
	// flag.StringVar(&cfg.db.dsn, "db-dsn", "", "PostgreSQL DSN")

	// Parse the command-line flags
	flag.Parse()

	// if the version flag is true, print the version and exit
	if *displayVersion {
		fmt.Printf("Version:\t%s\n", version)
		// fmt.Printf("Build time:\t%s\n", buildTime)
		os.Exit(0)
	}

	var err error

	// assign cgf.db.dsn to the dsn variable
	cfg.db.dsn = dsn

	// cfg.port, err = strconv.Atoi(httpPort)
	// if err != nil {
	// 	logger.PrintFatal(err, map[string]string{"message": "Invalid value for HTTP_PORT"})
	// }
	// cfg.db.maxIdleTime = dbMaxIdleTime
	// cfg.db.maxIdleConns, err = strconv.Atoi(dbMaxIdleConns)
	// if err != nil {
	// 	logger.PrintFatal(err, map[string]string{"message": "Invalid value for DB_MAX_IDLE_CONNS"})
	// }
	// cfg.db.maxOpenConns, err = strconv.Atoi(dbMaxOpenConns)
	// if err != nil {
	// 	logger.PrintFatal(err, map[string]string{"message": "Invalid value for DB_MAX_OPEN_CONNS"})
	// }

	// assign the trusted origins to the config struct
	// cfg.cors.trustedOrigins = strings.Fields(CORSTrustedOrigins)

	// assign environment variable to the config struct
	// cfg.env = environment

	// add rate limiter settings from environment variables
	// cfg.limiter.rps, err = strconv.ParseFloat(limiterRPS, 64)
	// if err != nil {
	// 	logger.PrintFatal(err, map[string]string{"message": "Invalid value for LIMITER_RPS"})
	// }
	// cfg.limiter.burst, err = strconv.Atoi(limiterBurst)
	// if err != nil {
	// 	logger.PrintFatal(err, map[string]string{"message": "Invalid value for LIMITER_BURST"})
	// }
	// cfg.limiter.enabled, err = strconv.ParseBool(limiterEnabled)
	// if err != nil {
	// 	logger.PrintFatal(err, map[string]string{"message": "Invalid value for LIMITER_ENABLED"})
	// }

	// open a connection to the database and defer the close
	db, err := openDB(cfg, automigrateBool)
	if err != nil {
		logger.PrintFatal(err, map[string]string{"message": "Error opening database connection"})
	}

	defer db.Close()
	logger.PrintInfo("database connection pool established", nil)

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
		mailer: mailer.New(cfg.smtp.host, cfg.smtp.port, cfg.smtp.username, cfg.smtp.password, cfg.smtp.sender), // use this when using command line flags
		// mailer: mailer.New(cfg.smtp.host, cfg.smtp.port, cfg.smtp.username, cfg.smtp.password, cfg.smtp.sender), // use this when using environment variables
	}

	// call the serve method on the application struct
	err = app.serve()
	if err != nil {
		logger.PrintFatal(err, map[string]string{"message": "server shutdown with error"})
	}

}

// openDB opens a new database connection using the provided DSN. It returns a sql.DB connection pool.
func openDB(cfg config, autoMigrate bool) (*sql.DB, error) {
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

	// run automigrate if the autoMigrate flag is true
	if autoMigrate {
		iofsDriver, err := iofs.New(assets.EmbeddedFiles, "migration")
		if err != nil {
			return nil, err
		}

		migrator, err := migrate.NewWithSourceInstance("iofs", iofsDriver, cfg.db.dsn)
		if err != nil {
			return nil, err
		}
		// run the migration
		err = migrator.Up()
		switch {
		case errors.Is(err, migrate.ErrNoChange):
			break
		case err != nil:
			return nil, err

		}
	}

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
