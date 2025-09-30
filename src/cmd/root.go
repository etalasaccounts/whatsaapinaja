package cmd

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"crypto/rand"
	"encoding/base64"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"os"
	"strings"
	"time"

	"github.com/aldinokemal/go-whatsapp-web-multidevice/config"
	domainApp "github.com/aldinokemal/go-whatsapp-web-multidevice/domains/app"
	domainChat "github.com/aldinokemal/go-whatsapp-web-multidevice/domains/chat"
	domainChatStorage "github.com/aldinokemal/go-whatsapp-web-multidevice/domains/chatstorage"
	domainGroup "github.com/aldinokemal/go-whatsapp-web-multidevice/domains/group"
	domainMessage "github.com/aldinokemal/go-whatsapp-web-multidevice/domains/message"
	domainNewsletter "github.com/aldinokemal/go-whatsapp-web-multidevice/domains/newsletter"
	domainSend "github.com/aldinokemal/go-whatsapp-web-multidevice/domains/send"
	domainUser "github.com/aldinokemal/go-whatsapp-web-multidevice/domains/user"
	"github.com/aldinokemal/go-whatsapp-web-multidevice/infrastructure/chatstorage"
	"github.com/aldinokemal/go-whatsapp-web-multidevice/infrastructure/whatsapp"
	"github.com/aldinokemal/go-whatsapp-web-multidevice/pkg/utils"
	"github.com/aldinokemal/go-whatsapp-web-multidevice/usecase"
	"golang.org/x/crypto/bcrypt"
	_ "github.com/lib/pq"
	_ "github.com/mattn/go-sqlite3"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"go.mau.fi/whatsmeow"
)

var (
    EmbedIndex embed.FS
    EmbedViews embed.FS

	// Whatsapp
	whatsappCli *whatsmeow.Client

    // Chat Storage
    chatStorageDB   *sql.DB
    chatStorageRepo domainChatStorage.IChatStorageRepository

    // Auth (Postgres-backed)
    authDB *sql.DB

	// Usecase
	appUsecase        domainApp.IAppUsecase
	chatUsecase       domainChat.IChatUsecase
	sendUsecase       domainSend.ISendUsecase
	userUsecase       domainUser.IUserUsecase
	messageUsecase    domainMessage.IMessageUsecase
	groupUsecase      domainGroup.IGroupUsecase
	newsletterUsecase domainNewsletter.INewsletterUsecase
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Short: "Send free whatsapp API",
	Long: `This application is from clone https://github.com/aldinokemal/go-whatsapp-web-multidevice, 
you can send whatsapp over http api but your whatsapp account have to be multi device version`,
}

func init() {
	// Load environment variables first
	utils.LoadConfig(".")

	time.Local = time.UTC

	rootCmd.CompletionOptions.DisableDefaultCmd = true

	// Initialize flags first, before any subcommands are added
	initFlags()

	// Then initialize other components
	cobra.OnInitialize(initEnvConfig, initApp)
}

// initEnvConfig loads configuration from environment variables
func initEnvConfig() {
	fmt.Println(viper.AllSettings())
	// Application settings
	if envPort := viper.GetString("app_port"); envPort != "" {
		config.AppPort = envPort
	}
	if envDebug := viper.GetBool("app_debug"); envDebug {
		config.AppDebug = envDebug
	}
	if envOs := viper.GetString("app_os"); envOs != "" {
		config.AppOs = envOs
	}
	if envBasicAuth := viper.GetString("app_basic_auth"); envBasicAuth != "" {
		credential := strings.Split(envBasicAuth, ",")
		config.AppBasicAuthCredential = credential
	}
	if envBasePath := viper.GetString("app_base_path"); envBasePath != "" {
		config.AppBasePath = envBasePath
	}

	// Database settings
	if envDBURI := viper.GetString("db_uri"); envDBURI != "" {
		config.DBURI = envDBURI
	}
	if envDBKEYSURI := viper.GetString("db_keys_uri"); envDBKEYSURI != "" {
		config.DBKeysURI = envDBKEYSURI
	}

	// WhatsApp settings
	if envAutoReply := viper.GetString("whatsapp_auto_reply"); envAutoReply != "" {
		config.WhatsappAutoReplyMessage = envAutoReply
	}
	if viper.IsSet("whatsapp_auto_mark_read") {
		config.WhatsappAutoMarkRead = viper.GetBool("whatsapp_auto_mark_read")
	}
	if envWebhook := viper.GetString("whatsapp_webhook"); envWebhook != "" {
		webhook := strings.Split(envWebhook, ",")
		config.WhatsappWebhook = webhook
	}
	if envWebhookSecret := viper.GetString("whatsapp_webhook_secret"); envWebhookSecret != "" {
		config.WhatsappWebhookSecret = envWebhookSecret
	}
	if viper.IsSet("whatsapp_account_validation") {
		config.WhatsappAccountValidation = viper.GetBool("whatsapp_account_validation")
	}
}

func initFlags() {
	// Application flags
	rootCmd.PersistentFlags().StringVarP(
		&config.AppPort,
		"port", "p",
		config.AppPort,
		"change port number with --port <number> | example: --port=8080",
	)

	rootCmd.PersistentFlags().BoolVarP(
		&config.AppDebug,
		"debug", "d",
		config.AppDebug,
		"hide or displaying log with --debug <true/false> | example: --debug=true",
	)
	rootCmd.PersistentFlags().StringVarP(
		&config.AppOs,
		"os", "",
		config.AppOs,
		`os name --os <string> | example: --os="Chrome"`,
	)
	rootCmd.PersistentFlags().StringSliceVarP(
		&config.AppBasicAuthCredential,
		"basic-auth", "b",
		config.AppBasicAuthCredential,
		"basic auth credential | -b=yourUsername:yourPassword",
	)
	rootCmd.PersistentFlags().StringVarP(
		&config.AppBasePath,
		"base-path", "",
		config.AppBasePath,
		`base path for subpath deployment --base-path <string> | example: --base-path="/gowa"`,
	)

	// Database flags
	rootCmd.PersistentFlags().StringVarP(
		&config.DBURI,
		"db-uri", "",
		config.DBURI,
		`the database uri to store the connection data database uri (by default, we'll use sqlite3 under storages/whatsapp.db). database uri --db-uri <string> | example: --db-uri="file:storages/whatsapp.db?_foreign_keys=on or postgres://user:password@localhost:5432/whatsapp"`,
	)
	rootCmd.PersistentFlags().StringVarP(
		&config.DBKeysURI,
		"db-keys-uri", "",
		config.DBKeysURI,
		`the database uri to store the keys database uri (by default, we'll use the same database uri). database uri --db-keys-uri <string> | example: --db-keys-uri="file::memory:?cache=shared&_foreign_keys=on"`,
	)

	// WhatsApp flags
	rootCmd.PersistentFlags().StringVarP(
		&config.WhatsappAutoReplyMessage,
		"autoreply", "",
		config.WhatsappAutoReplyMessage,
		`auto reply when received message --autoreply <string> | example: --autoreply="Don't reply this message"`,
	)
	rootCmd.PersistentFlags().BoolVarP(
		&config.WhatsappAutoMarkRead,
		"auto-mark-read", "",
		config.WhatsappAutoMarkRead,
		`auto mark incoming messages as read --auto-mark-read <true/false> | example: --auto-mark-read=true`,
	)
	rootCmd.PersistentFlags().StringSliceVarP(
		&config.WhatsappWebhook,
		"webhook", "w",
		config.WhatsappWebhook,
		`forward event to webhook --webhook <string> | example: --webhook="https://yourcallback.com/callback"`,
	)
	rootCmd.PersistentFlags().StringVarP(
		&config.WhatsappWebhookSecret,
		"webhook-secret", "",
		config.WhatsappWebhookSecret,
		`secure webhook request --webhook-secret <string> | example: --webhook-secret="super-secret-key"`,
	)
	rootCmd.PersistentFlags().BoolVarP(
		&config.WhatsappAccountValidation,
		"account-validation", "",
		config.WhatsappAccountValidation,
		`enable or disable account validation --account-validation <true/false> | example: --account-validation=true`,
	)
}

func initChatStorage() (*sql.DB, error) {
    uri := config.ChatStorageURI
    // Choose driver based on URI prefix
    if strings.HasPrefix(strings.ToLower(uri), "postgres") {
        db, err := sql.Open("postgres", uri)
        if err != nil {
            return nil, err
        }
        db.SetMaxOpenConns(25)
        db.SetMaxIdleConns(5)
        if err := db.Ping(); err != nil {
            db.Close()
            return nil, fmt.Errorf("failed to ping Postgres chat storage: %w", err)
        }
        return db, nil
    }

    // Default to SQLite (file:)
    connStr := fmt.Sprintf("%s?_journal_mode=WAL", uri)
    if config.ChatStorageEnableForeignKeys {
        connStr += "&_foreign_keys=on"
    }

    db, err := sql.Open("sqlite3", connStr)
    if err != nil {
        return nil, err
    }
    db.SetMaxOpenConns(25)
    db.SetMaxIdleConns(5)
    if err := db.Ping(); err != nil {
        db.Close()
        return nil, fmt.Errorf("failed to ping SQLite chat storage: %w", err)
    }
    return db, nil
}

func initApp() {
	if config.AppDebug {
		config.WhatsappLogLevel = "DEBUG"
		logrus.SetLevel(logrus.DebugLevel)
	}

	//preparing folder if not exist
	err := utils.CreateFolder(config.PathQrCode, config.PathSendItems, config.PathStorages, config.PathMedia)
	if err != nil {
		logrus.Errorln(err)
	}

	ctx := context.Background()

	chatStorageDB, err = initChatStorage()
	if err != nil {
		// Terminate the application if chat storage fails to initialize to avoid nil pointer panics later.
		logrus.Fatalf("failed to initialize chat storage: %v", err)
	}

    // Select repository based on ChatStorageURI
    if strings.HasPrefix(strings.ToLower(config.ChatStorageURI), "postgres") {
        chatStorageRepo = chatstorage.NewPostgresRepository(chatStorageDB)
    } else {
        chatStorageRepo = chatstorage.NewStorageRepository(chatStorageDB)
    }
	chatStorageRepo.InitializeSchema()

	// Seed a default admin user if none exists
	seedDefaultAdmin(chatStorageDB)

    // Initialize Postgres auth DB (use main DB_URI if it's postgres)
    if db, ok := initAuthDBPostgres(config.DBURI); ok {
        authDB = db
        ensurePGAppUsers(authDB)
        seedDefaultAdminPG(authDB)
    }

	whatsappDB := whatsapp.InitWaDB(ctx, config.DBURI)
	var keysDB *sqlstore.Container
	if config.DBKeysURI != "" {
		keysDB = whatsapp.InitWaDB(ctx, config.DBKeysURI)
	}

	whatsapp.InitWaCLI(ctx, whatsappDB, keysDB, chatStorageRepo)

	// Usecase
	appUsecase = usecase.NewAppService(chatStorageRepo)
	chatUsecase = usecase.NewChatService(chatStorageRepo)
	sendUsecase = usecase.NewSendService(appUsecase, chatStorageRepo)
	userUsecase = usecase.NewUserService()
	messageUsecase = usecase.NewMessageService(chatStorageRepo)
	groupUsecase = usecase.NewGroupService()
	newsletterUsecase = usecase.NewNewsletterService()
}

// seedDefaultAdmin creates an initial admin user when user table is empty.
func seedDefaultAdmin(db *sql.DB) {
    // Ensure app_users table exists (InitializeSchema ran earlier)
    var count int
    if err := db.QueryRow("SELECT COUNT(*) FROM app_users").Scan(&count); err != nil {
        logrus.Warnf("Skipping admin seed, cannot query app_users: %v", err)
        return
    }
    if count > 0 {
        return
    }

    // Generate random password
    pwdBytes := make([]byte, 16)
    if _, err := rand.Read(pwdBytes); err != nil {
        logrus.Warnf("Failed to generate admin password: %v", err)
        return
    }
    // URL-safe base64 without padding for readability
    password := base64.RawURLEncoding.EncodeToString(pwdBytes)
    hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
    if err != nil {
        logrus.Warnf("Failed to hash admin password: %v", err)
        return
    }

    if _, err := db.Exec(
        "INSERT INTO app_users (username, password_hash, role, enabled) VALUES (?, ?, 'admin', 1)",
        "admin", string(hash),
    ); err != nil {
        logrus.Warnf("Failed to insert default admin: %v", err)
        return
    }

    logrus.Infof("Seeded default admin user for REST Basic Auth -> username: admin, password: %s", password)
}

// initAuthDBPostgres opens Postgres connection for auth when using postgres URI
func initAuthDBPostgres(uri string) (*sql.DB, bool) {
    if !strings.HasPrefix(strings.ToLower(uri), "postgres") {
        return nil, false
    }
    db, err := sql.Open("postgres", uri)
    if err != nil {
        logrus.Warnf("Failed to open Postgres auth DB: %v", err)
        return nil, false
    }
    db.SetMaxOpenConns(25)
    db.SetMaxIdleConns(5)
    if err := db.Ping(); err != nil {
        logrus.Warnf("Failed to ping Postgres auth DB: %v", err)
        _ = db.Close()
        return nil, false
    }
    return db, true
}

// ensurePGAppUsers creates app_users table in Postgres if missing
func ensurePGAppUsers(db *sql.DB) {
    _, err := db.Exec(`
        CREATE TABLE IF NOT EXISTS app_users (
            username TEXT PRIMARY KEY,
            password_hash TEXT NOT NULL,
            role TEXT DEFAULT 'admin',
            enabled BOOLEAN DEFAULT TRUE,
            created_at TIMESTAMP DEFAULT NOW(),
            updated_at TIMESTAMP DEFAULT NOW()
        );
        CREATE INDEX IF NOT EXISTS idx_app_users_enabled ON app_users(enabled);
    `)
    if err != nil {
        logrus.Warnf("Failed to ensure app_users in Postgres: %v", err)
    }
}

// seedDefaultAdminPG seeds admin user in Postgres auth DB if empty
func seedDefaultAdminPG(db *sql.DB) {
    var count int
    if err := db.QueryRow("SELECT COUNT(*) FROM app_users").Scan(&count); err != nil {
        logrus.Warnf("Skipping PG admin seed, cannot query app_users: %v", err)
        return
    }
    if count > 0 {
        return
    }

    pwdBytes := make([]byte, 16)
    if _, err := rand.Read(pwdBytes); err != nil {
        logrus.Warnf("Failed to generate PG admin password: %v", err)
        return
    }
    password := base64.RawURLEncoding.EncodeToString(pwdBytes)
    hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
    if err != nil {
        logrus.Warnf("Failed to hash PG admin password: %v", err)
        return
    }
    if _, err := db.Exec(
        "INSERT INTO app_users (username, password_hash, role, enabled) VALUES ($1, $2, 'admin', TRUE)",
        "admin", string(hash),
    ); err != nil {
        logrus.Warnf("Failed to insert default PG admin: %v", err)
        return
    }
    logrus.Infof("Seeded default admin user (Postgres) -> username: admin, password: %s", password)
}

// Execute adds all child commands to the root command and sets flags appropriately.
func Execute(embedIndex embed.FS, embedViews embed.FS) {
	EmbedIndex = embedIndex
	EmbedViews = embedViews
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
