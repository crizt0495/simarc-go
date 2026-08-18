package handlers

import (
	"fmt"
	"net/http"

	"arsippro/internal/config"
	"time"

	"github.com/google/uuid"

	"arsippro/internal/database"
	"arsippro/internal/middleware"
	"arsippro/internal/models"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
)

// ShowLogin displays the login form
func ShowLogin(c *gin.Context) {
	// If the database is unreachable, send the user to the recovery page
	// instead of a login form that cannot work.
	if !database.Connected() {
		c.Redirect(http.StatusFound, "/database-setup")
		return
	}

	session := middleware.GetSession(c)
	data := gin.H{
		"title":     "Masuk - SIMARC",
		"pageTitle": "Selamat Datang Kembali",
		"subtitle":  "Masuk untuk mengakses sistem kearsipan elektronik Anda",
		"AppName":   config.App.AppName,
		"Year":      time.Now().Year(),
		"CSRFToken": middleware.GetCSRFToken(c),
		"LANIP":    GetLANIP(),
	}

	// get old username from session
	if old, ok := session.Values["old_username"].(string); ok {
		data["OldUsername"] = old
		delete(session.Values, "old_username")
		middleware.SaveSession(c, session)
	}

	// get validation errors
	if errMsg, ok := session.Values["auth_error"].(string); ok {
		data["AuthError"] = errMsg
		delete(session.Values, "auth_error")
		middleware.SaveSession(c, session)
	}

	middleware.SaveSession(c, session)
	Render(c, http.StatusOK, "auth/login.html", data)
}

const (
	lockoutThreshold = 5
	lockoutDuration  = 15 * time.Minute
	minPasswordLen   = 5
)

// Login processes the login form
func Login(c *gin.Context) {
	username := c.PostForm("username")
	password := c.PostForm("password")

	if username == "" || password == "" {
		session := middleware.GetSession(c)
		session.Values["auth_error"] = "Username dan password wajib diisi."
		session.Values["old_username"] = username
		middleware.SaveSession(c, session)
		c.Redirect(http.StatusFound, "/login")
		return
	}

	if len(password) < minPasswordLen {
		session := middleware.GetSession(c)
		session.Values["auth_error"] = "Password minimal 5 karakter."
		session.Values["old_username"] = username
		middleware.SaveSession(c, session)
		logLogin(nil, username, "weak_password", c.ClientIP(), c.Request.UserAgent())
		c.Redirect(http.StatusFound, "/login")
		return
	}

	var user models.User
	if err := database.DB.Preload("Role").Preload("UnitKerja").
		Where("username = ?", username).First(&user).Error; err != nil {
		session := middleware.GetSession(c)
		session.Values["auth_error"] = "Username atau password salah."
		session.Values["old_username"] = username
		middleware.SaveSession(c, session)
		logLogin(nil, username, "failed", c.ClientIP(), c.Request.UserAgent())
		c.Redirect(http.StatusFound, "/login")
		return
	}

	if !user.IsActive {
		session := middleware.GetSession(c)
		session.Values["auth_error"] = "Akun Anda tidak aktif. Hubungi administrator."
		session.Values["old_username"] = username
		middleware.SaveSession(c, session)
		logLogin(&user.ID, username, "blocked", c.ClientIP(), c.Request.UserAgent())
		c.Redirect(http.StatusFound, "/login")
		return
	}

	now := time.Now()

	// Check if account is locked
	if user.LockedUntil != nil && now.Before(*user.LockedUntil) {
		remaining := time.Until(*user.LockedUntil).Minutes()
		session := middleware.GetSession(c)
		session.Values["auth_error"] = fmt.Sprintf("Akun terkunci. Coba lagi dalam %.0f menit.", remaining+1)
		session.Values["old_username"] = username
		middleware.SaveSession(c, session)
		logLogin(&user.ID, username, "locked", c.ClientIP(), c.Request.UserAgent())
		c.Redirect(http.StatusFound, "/login")
		return
	}

	// Clear lockout if duration has passed
	if user.LockedUntil != nil && now.After(*user.LockedUntil) {
		database.DB.Model(&user).Updates(map[string]interface{}{
			"failed_attempts": 0,
			"locked_until":    nil,
		})
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password)); err != nil {
		failed := user.FailedAttempts + 1
		updates := map[string]interface{}{"failed_attempts": failed}

		if failed >= lockoutThreshold {
			until := now.Add(lockoutDuration)
			updates["locked_until"] = until
			updates["failed_attempts"] = 0
		}
		database.DB.Model(&user).Updates(updates)

		session := middleware.GetSession(c)
		session.Values["auth_error"] = "Username atau password salah."
		session.Values["old_username"] = username
		middleware.SaveSession(c, session)
		logLogin(&user.ID, username, "failed", c.ClientIP(), c.Request.UserAgent())
		c.Redirect(http.StatusFound, "/login")
		return
	}

	// Reset failed attempts on success
	database.DB.Model(&user).Updates(map[string]interface{}{
		"failed_attempts": 0,
		"locked_until":    nil,
	})

	// Success — set session
	session := middleware.GetSession(c)
	session.Values["user_id"] = user.ID
	middleware.SaveSession(c, session)

	// Update last login
	database.DB.Model(&user).Update("last_login_at", now)

	// Log success
	logLogin(&user.ID, username, "success", c.ClientIP(), c.Request.UserAgent())

	c.Redirect(http.StatusFound, "/dashboard")
}

// Logout logs the user out
func Logout(c *gin.Context) {
	session := middleware.GetSession(c)

	// Record logout time
	if userID, ok := session.Values["user_id"].(string); ok && userID != "" {
		now := time.Now()
		database.DB.Model(&models.LoginLog{}).
			Where("user_id = ? AND logout_time IS NULL", userID).
			Order("login_time DESC").
			Limit(1).
			Update("logout_time", now)
	}

	session.Values["user_id"] = nil
	session.Options.MaxAge = -1
	middleware.SaveSession(c, session)

	c.Header("Cache-Control", "no-cache, no-store, max-age=0, must-revalidate")
	c.Header("Pragma", "no-cache")
	c.Redirect(http.StatusFound, "/login")
}

func logLogin(userID *string, username, status, ip, ua string) {
	log := models.LoginLog{
		ID:        uuid.New().String(),
		UserID:    userID,
		Username:  username,
		IPAddress: ip,
		UserAgent: ua,
		Status:    status,
		LoginTime: time.Now(),
	}
	database.DB.Create(&log)
}
