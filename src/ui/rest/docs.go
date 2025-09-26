package rest

import (
	"io/ioutil"

	"github.com/gofiber/fiber/v2"
)

func InitRestDocs(app fiber.Router) {
	app.Get("/docs", serveRedocDocs)
	app.Get("/openapi.yaml", serveOpenAPISpec)
}

// serveRedocDocs serves the Redoc documentation page
func serveRedocDocs(c *fiber.Ctx) error {
	html := `<!DOCTYPE html>
<html>
<head>
    <title>WhatsApp API Documentation</title>
    <meta charset="utf-8"/>
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <link href="https://fonts.googleapis.com/css?family=Montserrat:300,400,700|Roboto:300,400,700" rel="stylesheet">
    <style>
        body {
            margin: 0;
            padding: 0;
            font-family: 'Roboto', sans-serif;
        }
        .header {
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
            color: white;
            padding: 20px;
            text-align: center;
            box-shadow: 0 2px 10px rgba(0,0,0,0.1);
        }
        .header h1 {
            margin: 0;
            font-size: 2.5em;
            font-weight: 300;
        }
        .header p {
            margin: 10px 0 0 0;
            opacity: 0.9;
            font-size: 1.1em;
        }
        .redoc-container {
            margin-top: 0;
        }
    </style>
</head>
<body>
    <div class="header">
        <h1>WhatsApp Web Multi-Device API</h1>
        <p>Comprehensive API Documentation</p>
    </div>
    <div class="redoc-container">
        <redoc spec-url="/openapi.yaml" theme='{
            "colors": {
                "primary": {
                    "main": "#667eea"
                }
            },
            "typography": {
                "fontSize": "14px",
                "lineHeight": "1.5em",
                "code": {
                    "fontSize": "13px"
                },
                "headings": {
                    "fontFamily": "Montserrat, sans-serif",
                    "fontWeight": "400"
                }
            },
            "sidebar": {
                "backgroundColor": "#fafafa",
                "width": "300px"
            },
            "rightPanel": {
                "backgroundColor": "#263238",
                "width": "40%"
            }
        }'></redoc>
    </div>
    <script src="https://cdn.jsdelivr.net/npm/redoc@2.1.3/bundles/redoc.standalone.js"></script>
</body>
</html>`

	c.Set("Content-Type", "text/html")
	return c.SendString(html)
}

// serveOpenAPISpec serves the openapi.yaml file
func serveOpenAPISpec(c *fiber.Ctx) error {
	// Try multiple paths to find the openapi.yaml file
	possiblePaths := []string{
		"docs/openapi.yaml",       // From project root
		"/app/docs/openapi.yaml",  // Docker container path
		"../docs/openapi.yaml",    // One level up from src
		"../../docs/openapi.yaml", // Two levels up from src/ui
		"./docs/openapi.yaml",     // Current directory
	}

	var yamlContent []byte
	var err error

	for _, yamlPath := range possiblePaths {
		yamlContent, err = ioutil.ReadFile(yamlPath)
		if err == nil {
			// File found and read successfully
			break
		}
	}

	if err != nil {
		// If still not found, return error instead of fallback
		return c.Status(500).JSON(fiber.Map{
			"error":       "OpenAPI specification file not found",
			"paths_tried": possiblePaths,
		})
	}

	c.Set("Content-Type", "application/x-yaml")
	c.Set("Access-Control-Allow-Origin", "*")
	return c.Send(yamlContent)
}
