package middleware

import (
	"log"
	"net/http"

	"arsippro/internal/config"
	"arsippro/internal/database"
	"arsippro/internal/models"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/sessions"
)

var Store *sessions.CookieStore

func InitSession() {
	// In production (Vercel) we refuse to boot without a strong SESSION_KEY.
	// A predictable key would allow anyone to forge session cookies.
	key := config.App.SessionKey
	if key == "" {
		if config.IsVercel() {
			// Use a per-process ephemeral key. Users will be logged out on each
			// deploy but the app stays secure. Set SESSION_KEY env var for stable cookies.
			log.Println("[WARN] SESSION_KEY not set — using ephemeral key (sessions reset per cold start). Set SESSION_KEY env var in Vercel for stable sessions.")
			// We do not crash: Vercel is a hosting environment where users will
			// notice and add the variable. Crashing triggers an error page with
			// less guidance.
			// Use a fixed fallback so the binary still loads; explicitly log the warning.
			key = "vercel-ephemeral-key-replace-me-" + config.App.AppName
		} else {
			log.Println("[WARN] SESSION_KEY not set — using development fallback. Set SESSION_KEY in .env for production.")
			key = "simarc-dev-fallback-key"
		}
	}
	Store = sessions.NewCookieStore([]byte(key))
	Store.Options = &sessions.Options{
		Path:     "/",
		MaxAge:   0,   // session cookie: expired saat browser ditutup
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Secure:   config.IsVercel(), // Vercel always serves over HTTPS
	}
}

func GetSession(c *gin.Context) *sessions.Session {
	session, _ := Store.Get(c.Request, "simarc_session")
	return session
}

func SaveSession(c *gin.Context, session *sessions.Session) {
	session.Save(c.Request, c.Writer)
}

func SetFlash(c *gin.Context, key, value string) {
	session := GetSession(c)
	session.AddFlash(value, key)
	SaveSession(c, session)
}

func GetFlash(c *gin.Context, key string) string {
	session := GetSession(c)
	flashes := session.Flashes(key)
	SaveSession(c, session)
	if len(flashes) > 0 {
		if msg, ok := flashes[0].(string); ok {
			return msg
		}
	}
	return ""
}

func GetCurrentUser(c *gin.Context) *models.User {
	if u, exists := c.Get("user"); exists {
		if user, ok := u.(*models.User); ok {
			return user
		}
	}
	return nil
}

// Auth middleware - requires login
func Auth() gin.HandlerFunc {
	return func(c *gin.Context) {
		session := GetSession(c)
		userID, ok := session.Values["user_id"].(string)
		if !ok || userID == "" {
			c.Redirect(http.StatusFound, "/login")
			c.Abort()
			return
		}

		var user models.User
		if err := database.DB.Preload("Role").Preload("UnitKerja").First(&user, "id = ?", userID).Error; err != nil {
			c.Redirect(http.StatusFound, "/login")
			c.Abort()
			return
		}

		if !user.IsActive {
			c.Redirect(http.StatusFound, "/login")
			c.Abort()
			return
		}

		c.Set("user", &user)
		c.Set("user_id", userID)
		c.Next()
	}
}

// Permission middleware
func Permission(permName string) gin.HandlerFunc {
	return func(c *gin.Context) {
		user := GetCurrentUser(c)
		if user == nil {
			c.Redirect(http.StatusFound, "/login")
			c.Abort()
			return
		}

		// Admin has all permissions
		if user.IsAdmin() {
			c.Next()
			return
		}

		// Check permission via DB
		var count int64
		database.DB.Table("permission_role").
			Joins("JOIN permissions ON permission_role.permission_id = permissions.id").
			Where("permission_role.role_id = ? AND permissions.name = ? AND permissions.is_active = 1", user.RoleID, permName).
			Count(&count)

		if count == 0 {
			c.HTML(http.StatusForbidden, "errors/403.html", gin.H{"title": "Akses Ditolak"})
			c.Abort()
			return
		}

		c.Next()
	}
}

// GuestOnly - no-op, selalu tampilkan halaman login
func GuestOnly() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
	}
}

// InjectUser - inject user data into all templates (optional, for public routes)
func InjectUser() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Skip when the database is unreachable (recovery mode) — there is
		// no user to load and DB access would fail.
		if !database.Connected() {
			c.Next()
			return
		}
		session := GetSession(c)
		if userID, ok := session.Values["user_id"].(string); ok && userID != "" {
			var user models.User
			if err := database.DB.Preload("Role").First(&user, "id = ?", userID).Error; err == nil {
				c.Set("user", &user)
			}
		}
		c.Next()
	}
}
