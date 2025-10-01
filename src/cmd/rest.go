package cmd

import (
    "fmt"
    "net/http"
    "os"

    "github.com/aldinokemal/go-whatsapp-web-multidevice/config"
    "github.com/aldinokemal/go-whatsapp-web-multidevice/ui/rest"
    "github.com/aldinokemal/go-whatsapp-web-multidevice/ui/rest/helpers"
    "github.com/aldinokemal/go-whatsapp-web-multidevice/ui/rest/middleware"
    "github.com/aldinokemal/go-whatsapp-web-multidevice/ui/websocket"
    "github.com/dustin/go-humanize"
    "github.com/gofiber/fiber/v2"
    "github.com/gofiber/fiber/v2/middleware/cors"
    "github.com/gofiber/fiber/v2/middleware/filesystem"
    "github.com/gofiber/fiber/v2/middleware/logger"
    "github.com/gofiber/template/html/v2"
    "github.com/sirupsen/logrus"
    "github.com/spf13/cobra"
)

// rootCmd represents the base command when called without any subcommands
var restCmd = &cobra.Command{
	Use:   "rest",
	Short: "Send whatsapp API over http",
	Long:  `This application is from clone https://github.com/aldinokemal/go-whatsapp-web-multidevice`,
	Run:   restServer,
}

func init() {
	rootCmd.AddCommand(restCmd)
}
func restServer(_ *cobra.Command, _ []string) {
    engine := html.NewFileSystem(http.FS(EmbedIndex), ".html")
    engine.AddFunc("isEnableBasicAuth", func(token any) bool {
        return token != nil
    })
	app := fiber.New(fiber.Config{
		Views:     engine,
		BodyLimit: int(config.WhatsappSettingMaxVideoSize),
		Network:   "tcp",
	})

	app.Static(config.AppBasePath+"/statics", "./statics")
	app.Use(config.AppBasePath+"/components", filesystem.New(filesystem.Config{
		Root:       http.FS(EmbedViews),
		PathPrefix: "views/components",
		Browse:     true,
	}))
	app.Use(config.AppBasePath+"/assets", filesystem.New(filesystem.Config{
		Root:       http.FS(EmbedViews),
		PathPrefix: "views/assets",
		Browse:     true,
	}))

    app.Use(middleware.Recovery())
    // Enforce SQL-backed Basic Auth using credentials from Postgres if available, fallback to SQLite chat storage
    if authDB != nil {
        app.Use(middleware.SQLBasicAuthPostgres(authDB))
    } else {
        app.Use(middleware.SQLBasicAuth(chatStorageDB))
    }
    app.Use(middleware.BasicAuth())
    if config.AppDebug {
        app.Use(logger.New())
    }
    app.Use(cors.New(cors.Config{
        AllowOrigins: "*",
        AllowHeaders: "Origin, Content-Type, Accept",
    }))

	// Create base path group or use app directly
	var apiGroup fiber.Router = app
	if config.AppBasePath != "" {
		apiGroup = app.Group(config.AppBasePath)
	}

	// Rest
	rest.InitRestApp(apiGroup, appUsecase)
	rest.InitRestChat(apiGroup, chatUsecase)
	rest.InitRestSend(apiGroup, sendUsecase)
	rest.InitRestUser(apiGroup, userUsecase)
	rest.InitRestMessage(apiGroup, messageUsecase)
	rest.InitRestGroup(apiGroup, groupUsecase)
	rest.InitRestNewsletter(apiGroup, newsletterUsecase)
	rest.InitRestDocs(apiGroup)

	apiGroup.Get("/", func(c *fiber.Ctx) error {
		return c.Render("views/index", fiber.Map{
			"AppHost":        fmt.Sprintf("%s://%s", c.Protocol(), c.Hostname()),
			"AppVersion":     config.AppVersion,
			"AppBasePath":    config.AppBasePath,
			"BasicAuthToken": c.UserContext().Value(middleware.AuthorizationValue("BASIC_AUTH")),
			"MaxFileSize":    humanize.Bytes(uint64(config.WhatsappSettingMaxFileSize)),
			"MaxVideoSize":   humanize.Bytes(uint64(config.WhatsappSettingMaxVideoSize)),
		})
	})

	websocket.RegisterRoutes(apiGroup, appUsecase)
	go websocket.RunHub()

	// Set auto reconnect to whatsapp server after booting
	go helpers.SetAutoConnectAfterBooting(appUsecase)
	// Set auto reconnect checking
	go helpers.SetAutoReconnectChecking(whatsappCli)

	// Use PORT environment variable for Railway deployment, fallback to config.AppPort
	port := config.AppPort
	if envPort := os.Getenv("PORT"); envPort != "" {
		port = envPort
		logrus.Infof("Using PORT environment variable: %s", port)
	} else {
		logrus.Infof("Using default port: %s", port)
	}

	logrus.Infof("Starting server on port: %s", port)
	if err := app.Listen(":" + port); err != nil {
		logrus.Fatalln("Failed to start: ", err.Error())
	}
}
