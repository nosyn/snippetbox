package main

import (
	"crypto/tls"
	"database/sql"
	"flag"
	"html/template"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/biem97/snippetbox/internal/models"

	"github.com/alexedwards/scs/mysqlstore"
	"github.com/alexedwards/scs/v2"
	"github.com/go-playground/form/v4"
	"github.com/go-sql-driver/mysql"
	_ "github.com/go-sql-driver/mysql"
)

type application struct {
	debug          bool
	errorLog       *log.Logger
	infoLog        *log.Logger
	snippets       models.SnippetModelInterface
	users          models.UserModelInterface
	templateCache  map[string]*template.Template
	formDecoder    *form.Decoder
	sessionManager *scs.SessionManager
}

func main() {
	addr := flag.String("addr", ":8080", "HTTP network address")
	// Define a new command-line flag for the MySQL DSN string.
	debug := flag.Bool("debug", false, "Enable debug mode")
	// dsn := flag.String("dsn", "web:pass@/snippetbox?parseTime=true", "MySQL data source name (connection string)")

	flag.Parse()

	infoLog := log.New(os.Stdout, "INFO\t", log.Ldate|log.Ltime)
	errorLog := log.New(os.Stderr, "ERROR\t", log.Ldate|log.Ltime|log.Lshortfile)

	db, err := openDB()
	if err != nil {
		errorLog.Fatal(err)
	}
	defer db.Close()

	// Initialize a new template cache...
	templateCache, err := newTemplateCache()
	if err != nil {
		errorLog.Fatal(err)
	}

	// Initialize a decoder instance
	formDecoder := form.NewDecoder()

	// Initialize a session manager
	sessionManager := scs.New()
	sessionManager.Store = mysqlstore.New(db)
	sessionManager.Lifetime = 12 * time.Hour
	// Make sure that the Secure attribute is set on our session cookies.
	// Setting this means that the cookie will only be sent by a user's web
	// browser when a HTTPS connection is being used (and won't be sent over an
	// unsecure HTTP connection).
	sessionManager.Cookie.Secure = true

	app := &application{
		debug:          *debug,
		errorLog:       errorLog,
		infoLog:        infoLog,
		snippets:       &models.SnippetModel{DB: db},
		users:          &models.UserModel{DB: db},
		templateCache:  templateCache,
		formDecoder:    formDecoder,
		sessionManager: sessionManager,
	}

	srv := &http.Server{
		Addr:     *addr,
		ErrorLog: errorLog,
		// Call the new app.routes() method to get the servemux containing our routes.
		Handler: app.routes(),
		// TLSConfig: tlsConfig,
		// Add Idle, Read and Write timeouts to the server
		IdleTimeout:  time.Minute,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	infoLog.Printf("Starting server on %s", *addr)

	if os.Getenv("APP_ENV") == "cloud" {
		err = srv.ListenAndServe()
	} else {
		// Initialize a tls.Config struct to hold the non-default TLS settings we want the server to use.
		// In this case the only thing that we're changing is the curve preferences value, so that only
		// elliptic curves with assembly implementations are used.
		tlsConfig := &tls.Config{
			CurvePreferences: []tls.CurveID{tls.X25519, tls.CurveP256},
		}

		srv.TLSConfig = tlsConfig
		// Use the ListenAndServeTLS() method to start the HTTPS server.
		// We pass in the paths to the TLS certificate and corresponding private key
		// as the two parameters
		err = srv.ListenAndServeTLS("./tls/cert.pem", "./tls/key.pem")
	}

	errorLog.Fatal(err)
}

func openDB() (*sql.DB, error) {
	// Capture connection properties.
	cfg := mysql.Config{
		User:   os.Getenv("DB_USER"),
		Passwd: os.Getenv("DB_PWD"),
		Net:    "tcp",
		Addr:   os.Getenv("DB_ADDR"), // 127.0.0.1:3306
		DBName: os.Getenv("DB_NAME"),
		Params: map[string]string{
			"parseTime": "true",
		},
	}

	if os.Getenv("APP_ENV") == "cloud" {
		cfg.AllowNativePasswords = true
	}

	db, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		return nil, err
	}

	if err = db.Ping(); err != nil {
		return nil, err
	}

	return db, nil
}
