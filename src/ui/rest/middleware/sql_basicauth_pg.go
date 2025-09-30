package middleware

import (
    "database/sql"
    "encoding/base64"
    "strings"

    "github.com/gofiber/fiber/v2"
    "golang.org/x/crypto/bcrypt"
)

// SQLBasicAuthPostgres validates HTTP Basic Auth against Postgres table app_users.
func SQLBasicAuthPostgres(db *sql.DB) fiber.Handler {
    return func(c *fiber.Ctx) error {
        auth := c.Get("Authorization")
        if strings.HasPrefix(auth, "Basic ") {
            payload := strings.TrimPrefix(auth, "Basic ")
            decoded, err := base64.StdEncoding.DecodeString(payload)
            if err == nil {
                parts := strings.SplitN(string(decoded), ":", 2)
                if len(parts) == 2 {
                    username, password := parts[0], parts[1]
                    var hash string
                    var enabled bool
                    err := db.QueryRow("SELECT password_hash, enabled FROM app_users WHERE username = $1", username).Scan(&hash, &enabled)
                    if err == nil && enabled {
                        if bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil {
                            return c.Next()
                        }
                    }
                }
            }
        }

        c.Set(fiber.HeaderWWWAuthenticate, "Basic realm=\"Restricted\"")
        return c.SendStatus(fiber.StatusUnauthorized)
    }
}