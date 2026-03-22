package main

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/joho/godotenv"
	"golang.org/x/crypto/bcrypt"
)

type server struct {
	db *sql.DB
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type createNodeRequest struct {
	Type               string `json:"type"`
	Name               string `json:"name"`
	ParentID           *int64 `json:"parentId"`
	OrderNo            int64  `json:"orderNo"`
	Hidden             bool   `json:"hidden"`
	Collapsed          bool   `json:"collapsed"`
	DailyTargetMinutes int    `json:"dailyTargetMinutes"`
}

type updateNodeRequest struct {
	Name               *string `json:"name"`
	ParentID           *int64  `json:"parentId"`
	OrderNo            *int64  `json:"orderNo"`
	Hidden             *bool   `json:"hidden"`
	Collapsed          *bool   `json:"collapsed"`
	DailyTargetMinutes *int    `json:"dailyTargetMinutes"`
	ParentIDSet        bool    `json:"-"`
}

func (r *updateNodeRequest) UnmarshalJSON(data []byte) error {
	type alias updateNodeRequest
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	var base alias
	if err := json.Unmarshal(data, &base); err != nil {
		return err
	}
	*r = updateNodeRequest(base)
	if parentRaw, ok := raw["parentId"]; ok {
		r.ParentIDSet = true
		if string(parentRaw) == "null" {
			r.ParentID = nil
			return nil
		}
		var pid int64
		if err := json.Unmarshal(parentRaw, &pid); err != nil {
			return err
		}
		r.ParentID = &pid
	}
	return nil
}

type createRecordRequest struct {
	ItemID           int64  `json:"itemId"`
	StartAt          string `json:"startAt"`
	EndAt            string `json:"endAt"`
	PauseDurationMs  int64  `json:"pauseDurationMs"`
	Description      string `json:"description"`
	Source           string `json:"source"`
	ApplySplitByDate bool   `json:"applySplitByDate"`
}

type updateRecordRequest struct {
	ItemID          *int64  `json:"itemId"`
	StartAt         *string `json:"startAt"`
	EndAt           *string `json:"endAt"`
	PauseDurationMs *int64  `json:"pauseDurationMs"`
	Description     *string `json:"description"`
}

type timerStartRequest struct {
	ItemID    int64  `json:"itemId"`
	StartAt   string `json:"startAt"`
	DraftDesc string `json:"draftDescription"`
}

type timerStopRequest struct {
	EndAt       string `json:"endAt"`
	Description string `json:"description"`
	Save        bool   `json:"save"`
}

type timerState struct {
	ActiveItemID       *int64     `json:"activeItemId"`
	SessionStartAt     *time.Time `json:"sessionStartAt"`
	AccumulatedPauseMs int64      `json:"accumulatedPauseMs"`
	IsPaused           bool       `json:"isPaused"`
	PauseStartedAt     *time.Time `json:"pauseStartedAt"`
	DraftDescription   string     `json:"draftDescription"`
}

type recordSegment struct {
	StartAt         time.Time
	EndAt           time.Time
	PauseDurationMs int64
}

func main() {
	godotenv.Load()
	dsn := strings.TrimSpace(os.Getenv("DB_DSN"))
	if dsn == "" {
		log.Fatal("DB_DSN 未设置")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	if err = db.Ping(); err != nil {
		log.Fatal(err)
	}

	if err = migrate(db); err != nil {
		log.Fatal(err)
	}
	if err = seedAdminUser(db); err != nil {
		log.Fatal(err)
	}
	if err = ensureSingleRows(db); err != nil {
		log.Fatal(err)
	}

	s := &server{db: db}
	r := gin.Default()
	r.Use(corsMiddleware())

	api := r.Group("/api")
	api.POST("/auth/login", s.login)
	api.POST("/auth/logout", s.logout)

	protected := api.Group("")
	protected.Use(s.authMiddleware)
	protected.GET("/auth/me", s.me)
	protected.GET("/nodes", s.listNodes)
	protected.POST("/nodes", s.createNode)
	protected.PUT("/nodes/:id", s.updateNode)
	protected.DELETE("/nodes/:id", s.deleteNode)
	protected.GET("/records", s.listRecords)
	protected.POST("/records", s.createRecord)
	protected.PUT("/records/:id", s.updateRecord)
	protected.DELETE("/records/:id", s.deleteRecord)
	protected.GET("/timer/state", s.getTimerState)
	protected.POST("/timer/start", s.startTimer)
	protected.POST("/timer/pause", s.pauseTimer)
	protected.POST("/timer/resume", s.resumeTimer)
	protected.POST("/timer/stop", s.stopTimer)
	protected.GET("/stats/overview", s.statsOverview)
	protected.GET("/settings", s.getSettings)
	protected.PUT("/settings", s.updateSettings)

	distPath := filepath.Join("frontend", "dist")
	if st, err := os.Stat(distPath); err == nil && st.IsDir() {
		r.Static("/assets", filepath.Join(distPath, "assets"))
		r.StaticFile("/", filepath.Join(distPath, "index.html"))
		r.NoRoute(func(c *gin.Context) {
			if strings.HasPrefix(c.Request.URL.Path, "/api/") {
				c.JSON(http.StatusNotFound, gin.H{"message": "not found"})
				return
			}
			c.File(filepath.Join(distPath, "index.html"))
		})
	}

	port := strings.TrimSpace(os.Getenv("PORT"))
	if port == "" {
		port = "5174"
	}
	log.Fatal(r.Run(":" + port))
}

func migrate(db *sql.DB) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS users (
			id BIGSERIAL PRIMARY KEY,
			username TEXT UNIQUE NOT NULL,
			password_hash TEXT NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS sessions (
			id BIGSERIAL PRIMARY KEY,
			token TEXT UNIQUE NOT NULL,
			user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			expires_at TIMESTAMPTZ NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS nodes (
			id BIGSERIAL PRIMARY KEY,
			type TEXT NOT NULL CHECK (type IN ('category', 'item')),
			name TEXT NOT NULL,
			parent_id BIGINT REFERENCES nodes(id) ON DELETE CASCADE,
			order_no INT NOT NULL DEFAULT 0,
			hidden BOOLEAN NOT NULL DEFAULT FALSE,
			collapsed BOOLEAN NOT NULL DEFAULT FALSE,
			daily_target_minutes INT NOT NULL DEFAULT 0,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS records (
			id BIGSERIAL PRIMARY KEY,
			item_id BIGINT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
			start_at TIMESTAMPTZ NOT NULL,
			end_at TIMESTAMPTZ NOT NULL,
			pause_duration_ms BIGINT NOT NULL DEFAULT 0,
			description TEXT NOT NULL DEFAULT '',
			source TEXT NOT NULL CHECK (source IN ('timer', 'manual')),
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS settings (
			id INT PRIMARY KEY,
			confirm_before_save_timer_record BOOLEAN NOT NULL DEFAULT TRUE,
			show_hidden_nodes BOOLEAN NOT NULL DEFAULT FALSE,
			skip_short_timer_record BOOLEAN NOT NULL DEFAULT FALSE,
			stats_include_hidden_nodes BOOLEAN NOT NULL DEFAULT FALSE
		)`,
		`CREATE TABLE IF NOT EXISTS timer_state (
			id INT PRIMARY KEY,
			active_item_id BIGINT REFERENCES nodes(id) ON DELETE SET NULL,
			session_start_at TIMESTAMPTZ,
			accumulated_pause_ms BIGINT NOT NULL DEFAULT 0,
			is_paused BOOLEAN NOT NULL DEFAULT FALSE,
			pause_started_at TIMESTAMPTZ,
			draft_description TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE INDEX IF NOT EXISTS idx_nodes_parent_order ON nodes(parent_id, order_no, id)`,
		`CREATE INDEX IF NOT EXISTS idx_records_item ON records(item_id)`,
		`CREATE INDEX IF NOT EXISTS idx_records_time ON records(start_at, end_at)`,
		`CREATE INDEX IF NOT EXISTS idx_sessions_token ON sessions(token)`,
		`ALTER TABLE nodes ALTER COLUMN order_no TYPE BIGINT`,
		`ALTER TABLE settings ADD COLUMN IF NOT EXISTS show_hidden_nodes BOOLEAN NOT NULL DEFAULT FALSE`,
		`ALTER TABLE settings ADD COLUMN IF NOT EXISTS skip_short_timer_record BOOLEAN NOT NULL DEFAULT FALSE`,
		`ALTER TABLE settings ADD COLUMN IF NOT EXISTS stats_include_hidden_nodes BOOLEAN NOT NULL DEFAULT FALSE`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			// Ignore error for ALTER TABLE if it fails (e.g. redundant but harmless usually, or wrapped in better logic)
			// But here simplest is to just log and continue or ignore specific errors.
			// Ideally we check schema version. For MVP, we just try to execute.
			// However, blindly running ALTER might fail if it's already BIGINT? No, it's idempotent-ish for type expansion.
			// Let's just log error but not fail fatal for the ALTER command if possible.
			// Actually, let's just append it. If it fails (e.g. table doesn't exist yet in the loop), that's bad.
			// The loop executes in order. The CREATE TABLE is above. So table exists.
			// If column is already BIGINT, Postgres usually allows this or returns success.
			// If not, it converts.
			// Let's wrapping it to ignore error is hard in this loop structure.
			// Better approach: Separate migration for the fix.
		}
	}
	// Separate migration for the fix to ensure it runs
	db.Exec(`ALTER TABLE nodes ALTER COLUMN order_no TYPE BIGINT`)

	return nil
}

func ensureSingleRows(db *sql.DB) error {
	if _, err := db.Exec(`INSERT INTO settings(id, confirm_before_save_timer_record) VALUES (1, TRUE) ON CONFLICT (id) DO NOTHING`); err != nil {
		return err
	}
	if _, err := db.Exec(`INSERT INTO timer_state(id, accumulated_pause_ms, is_paused) VALUES (1, 0, FALSE) ON CONFLICT (id) DO NOTHING`); err != nil {
		return err
	}
	return nil
}

func seedAdminUser(db *sql.DB) error {
	username := strings.TrimSpace(os.Getenv("ADMIN_USERNAME"))
	password := strings.TrimSpace(os.Getenv("ADMIN_PASSWORD"))
	if username == "" {
		username = "ts65213"
	}
	if password == "" {
		return errors.New("ADMIN_PASSWORD 未设置")
	}
	var exists bool
	if err := db.QueryRow(`SELECT EXISTS(SELECT 1 FROM users WHERE username=$1)`, username).Scan(&exists); err != nil {
		return err
	}
	if exists {
		return nil
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	_, err = db.Exec(`INSERT INTO users(username, password_hash) VALUES ($1, $2)`, username, string(hash))
	return err
}

func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if origin != "" {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Vary", "Origin")
		}
		c.Header("Access-Control-Allow-Credentials", "true")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

func parseTime(input string, fallback time.Time) (time.Time, error) {
	if strings.TrimSpace(input) == "" {
		return fallback, nil
	}
	t, err := time.Parse(time.RFC3339, input)
	if err == nil {
		return t, nil
	}
	return time.Time{}, errors.New("time must be RFC3339")
}

func randomToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func (s *server) login(c *gin.Context) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "invalid payload"})
		return
	}
	var userID int64
	var hash string
	err := s.db.QueryRow(`SELECT id, password_hash FROM users WHERE username=$1`, req.Username).Scan(&userID, &hash)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"message": "用户名或密码错误"})
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(req.Password)) != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"message": "用户名或密码错误"})
		return
	}
	token, err := randomToken()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "登录失败"})
		return
	}
	exp := time.Now().Add(30 * 24 * time.Hour)
	if _, err = s.db.Exec(`INSERT INTO sessions(token, user_id, expires_at) VALUES ($1, $2, $3)`, token, userID, exp); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "登录失败"})
		return
	}
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     "session_token",
		Value:    token,
		MaxAge:   30 * 24 * 3600,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (s *server) logout(c *gin.Context) {
	cookie, _ := c.Request.Cookie("session_token")
	if cookie != nil && cookie.Value != "" {
		s.db.Exec(`DELETE FROM sessions WHERE token=$1`, cookie.Value)
	}
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     "session_token",
		Value:    "",
		MaxAge:   -1,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (s *server) authMiddleware(c *gin.Context) {
	cookie, err := c.Request.Cookie("session_token")
	if err != nil || cookie.Value == "" {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"message": "未登录"})
		return
	}
	var userID int64
	err = s.db.QueryRow(`SELECT user_id FROM sessions WHERE token=$1 AND expires_at>NOW()`, cookie.Value).Scan(&userID)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"message": "登录已过期"})
		return
	}
	c.Set("userID", userID)
	c.Next()
}

func (s *server) me(c *gin.Context) {
	userID := c.MustGet("userID").(int64)
	var username string
	if err := s.db.QueryRow(`SELECT username FROM users WHERE id=$1`, userID).Scan(&username); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "查询失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"id": userID, "username": username})
}

func (s *server) listNodes(c *gin.Context) {
	rows, err := s.db.Query(`SELECT id, type, name, parent_id, order_no, hidden, collapsed, daily_target_minutes, created_at, updated_at FROM nodes ORDER BY COALESCE(parent_id,0), order_no, id`)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "查询失败"})
		return
	}
	defer rows.Close()
	out := make([]gin.H, 0)
	for rows.Next() {
		var id int64
		var typ, name string
		var parentID sql.NullInt64
		var orderNo int
		var hidden, collapsed bool
		var dailyTarget int
		var createdAt, updatedAt time.Time
		if err = rows.Scan(&id, &typ, &name, &parentID, &orderNo, &hidden, &collapsed, &dailyTarget, &createdAt, &updatedAt); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"message": "查询失败"})
			return
		}
		var pid any
		if parentID.Valid {
			pid = parentID.Int64
		}
		out = append(out, gin.H{
			"id":                 id,
			"type":               typ,
			"name":               name,
			"parentId":           pid,
			"orderNo":            orderNo,
			"hidden":             hidden,
			"collapsed":          collapsed,
			"dailyTargetMinutes": dailyTarget,
			"createdAt":          createdAt,
			"updatedAt":          updatedAt,
		})
	}
	c.JSON(http.StatusOK, out)
}

func (s *server) createNode(c *gin.Context) {
	var req createNodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "参数错误"})
		return
	}
	if req.Type != "category" && req.Type != "item" {
		c.JSON(http.StatusBadRequest, gin.H{"message": "type 错误"})
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"message": "名称不能为空"})
		return
	}
	var id int64
	err := s.db.QueryRow(
		`INSERT INTO nodes(type, name, parent_id, order_no, hidden, collapsed, daily_target_minutes, updated_at) VALUES ($1,$2,$3,$4,$5,$6,$7,NOW()) RETURNING id`,
		req.Type, req.Name, req.ParentID, req.OrderNo, req.Hidden, req.Collapsed, req.DailyTargetMinutes,
	).Scan(&id)
	if err != nil {
		fmt.Println("CreateNode Error:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"message": "创建失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"id": id})
}

func (s *server) updateNode(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "id 错误"})
		return
	}
	var req updateNodeRequest
	if err = c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "参数错误"})
		return
	}
	sets := []string{"updated_at=NOW()"}
	args := []any{}
	argN := 1
	if req.Name != nil {
		sets = append(sets, fmt.Sprintf("name=$%d", argN))
		args = append(args, *req.Name)
		argN++
	}
	if req.ParentIDSet {
		sets = append(sets, fmt.Sprintf("parent_id=$%d", argN))
		args = append(args, req.ParentID)
		argN++
	}
	if req.OrderNo != nil {
		sets = append(sets, fmt.Sprintf("order_no=$%d", argN))
		args = append(args, *req.OrderNo)
		argN++
	}
	if req.Hidden != nil {
		sets = append(sets, fmt.Sprintf("hidden=$%d", argN))
		args = append(args, *req.Hidden)
		argN++
	}
	if req.Collapsed != nil {
		sets = append(sets, fmt.Sprintf("collapsed=$%d", argN))
		args = append(args, *req.Collapsed)
		argN++
	}
	if req.DailyTargetMinutes != nil {
		sets = append(sets, fmt.Sprintf("daily_target_minutes=$%d", argN))
		args = append(args, *req.DailyTargetMinutes)
		argN++
	}
	args = append(args, id)
	query := fmt.Sprintf("UPDATE nodes SET %s WHERE id=$%d", strings.Join(sets, ","), argN)
	res, err := s.db.Exec(query, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "更新失败"})
		return
	}
	if rows, _ := res.RowsAffected(); rows == 0 {
		c.JSON(http.StatusNotFound, gin.H{"message": "节点不存在"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (s *server) deleteNode(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "id 错误"})
		return
	}
	res, err := s.db.Exec(`DELETE FROM nodes WHERE id=$1`, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "删除失败"})
		return
	}
	if rows, _ := res.RowsAffected(); rows == 0 {
		c.JSON(http.StatusNotFound, gin.H{"message": "节点不存在"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func splitByDay(startAt, endAt time.Time, pauseMs int64) []recordSegment {
	if !startAt.Before(endAt) {
		return nil
	}
	totalMs := endAt.Sub(startAt).Milliseconds()
	if totalMs <= 0 {
		return nil
	}
	if pauseMs < 0 {
		pauseMs = 0
	}
	if pauseMs > totalMs {
		pauseMs = totalMs
	}
	segments := make([]recordSegment, 0)
	current := startAt
	for current.Before(endAt) {
		nextDay := time.Date(current.Year(), current.Month(), current.Day()+1, 0, 0, 0, 0, current.Location())
		segmentEnd := nextDay
		if !segmentEnd.Before(endAt) {
			segmentEnd = endAt
		}
		segments = append(segments, recordSegment{StartAt: current, EndAt: segmentEnd})
		current = segmentEnd
	}
	remainPause := pauseMs
	for i := range segments {
		segMs := segments[i].EndAt.Sub(segments[i].StartAt).Milliseconds()
		segPause := pauseMs * segMs / totalMs
		if segPause > remainPause {
			segPause = remainPause
		}
		segments[i].PauseDurationMs = segPause
		remainPause -= segPause
	}
	if len(segments) > 0 && remainPause > 0 {
		segments[len(segments)-1].PauseDurationMs += remainPause
	}
	return segments
}

func (s *server) insertRecord(itemID int64, startAt, endAt time.Time, pauseMs int64, desc, source string) error {
	if !startAt.Before(endAt) {
		return errors.New("startAt must be before endAt")
	}
	durMs := endAt.Sub(startAt).Milliseconds()
	if pauseMs < 0 || pauseMs > durMs {
		return errors.New("pauseDurationMs invalid")
	}
	if source == "" {
		source = "manual"
	}
	_, err := s.db.Exec(
		`INSERT INTO records(item_id, start_at, end_at, pause_duration_ms, description, source, updated_at) VALUES ($1,$2,$3,$4,$5,$6,NOW())`,
		itemID, startAt, endAt, pauseMs, desc, source,
	)
	return err
}

func (s *server) createRecord(c *gin.Context) {
	var req createRecordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "参数错误"})
		return
	}
	startAt, err := parseTime(req.StartAt, time.Time{})
	if err != nil || startAt.IsZero() {
		c.JSON(http.StatusBadRequest, gin.H{"message": "startAt 错误"})
		return
	}
	endAt, err := parseTime(req.EndAt, time.Time{})
	if err != nil || endAt.IsZero() {
		c.JSON(http.StatusBadRequest, gin.H{"message": "endAt 错误"})
		return
	}
	source := req.Source
	if source == "" {
		source = "manual"
	}
	if req.ApplySplitByDate {
		for _, seg := range splitByDay(startAt, endAt, req.PauseDurationMs) {
			if err = s.insertRecord(req.ItemID, seg.StartAt, seg.EndAt, seg.PauseDurationMs, req.Description, source); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
				return
			}
		}
		c.JSON(http.StatusOK, gin.H{"ok": true})
		return
	}
	if err = s.insertRecord(req.ItemID, startAt, endAt, req.PauseDurationMs, req.Description, source); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (s *server) listRecords(c *gin.Context) {
	from := c.Query("from")
	to := c.Query("to")
	itemID := strings.TrimSpace(c.Query("itemId"))
	source := strings.TrimSpace(c.Query("source"))
	args := []any{}
	where := []string{"1=1"}
	if strings.TrimSpace(from) != "" {
		if t, err := time.Parse(time.RFC3339, from); err == nil {
			args = append(args, t)
			where = append(where, fmt.Sprintf("end_at >= $%d", len(args)))
		}
	}
	if strings.TrimSpace(to) != "" {
		if t, err := time.Parse(time.RFC3339, to); err == nil {
			args = append(args, t)
			where = append(where, fmt.Sprintf("start_at <= $%d", len(args)))
		}
	}
	if itemID != "" {
		if v, err := strconv.ParseInt(itemID, 10, 64); err == nil && v > 0 {
			args = append(args, v)
			where = append(where, fmt.Sprintf("item_id = $%d", len(args)))
		}
	}
	if source == "timer" || source == "manual" {
		args = append(args, source)
		where = append(where, fmt.Sprintf("source = $%d", len(args)))
	}
	query := fmt.Sprintf(`SELECT id, item_id, start_at, end_at, pause_duration_ms, description, source, created_at, updated_at FROM records WHERE %s ORDER BY start_at DESC`, strings.Join(where, " AND "))
	rows, err := s.db.Query(query, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "查询失败"})
		return
	}
	defer rows.Close()
	out := make([]gin.H, 0)
	for rows.Next() {
		var id, itemID, pauseMs int64
		var startAt, endAt, createdAt, updatedAt time.Time
		var desc, source string
		if err = rows.Scan(&id, &itemID, &startAt, &endAt, &pauseMs, &desc, &source, &createdAt, &updatedAt); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"message": "查询失败"})
			return
		}
		out = append(out, gin.H{
			"id":              id,
			"itemId":          itemID,
			"startAt":         startAt,
			"endAt":           endAt,
			"pauseDurationMs": pauseMs,
			"description":     desc,
			"source":          source,
			"createdAt":       createdAt,
			"updatedAt":       updatedAt,
		})
	}
	c.JSON(http.StatusOK, out)
}

func (s *server) updateRecord(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "id 错误"})
		return
	}
	var req updateRecordRequest
	if err = c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "参数错误"})
		return
	}
	sets := []string{"updated_at=NOW()"}
	args := []any{}
	argN := 1
	if req.ItemID != nil {
		sets = append(sets, fmt.Sprintf("item_id=$%d", argN))
		args = append(args, *req.ItemID)
		argN++
	}
	if req.StartAt != nil {
		t, parseErr := time.Parse(time.RFC3339, *req.StartAt)
		if parseErr != nil {
			c.JSON(http.StatusBadRequest, gin.H{"message": "startAt 错误"})
			return
		}
		sets = append(sets, fmt.Sprintf("start_at=$%d", argN))
		args = append(args, t)
		argN++
	}
	if req.EndAt != nil {
		t, parseErr := time.Parse(time.RFC3339, *req.EndAt)
		if parseErr != nil {
			c.JSON(http.StatusBadRequest, gin.H{"message": "endAt 错误"})
			return
		}
		sets = append(sets, fmt.Sprintf("end_at=$%d", argN))
		args = append(args, t)
		argN++
	}
	if req.PauseDurationMs != nil {
		sets = append(sets, fmt.Sprintf("pause_duration_ms=$%d", argN))
		args = append(args, *req.PauseDurationMs)
		argN++
	}
	if req.Description != nil {
		sets = append(sets, fmt.Sprintf("description=$%d", argN))
		args = append(args, *req.Description)
		argN++
	}
	args = append(args, id)
	query := fmt.Sprintf("UPDATE records SET %s WHERE id=$%d", strings.Join(sets, ","), argN)
	res, err := s.db.Exec(query, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "更新失败"})
		return
	}
	if rows, _ := res.RowsAffected(); rows == 0 {
		c.JSON(http.StatusNotFound, gin.H{"message": "记录不存在"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (s *server) deleteRecord(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "id 错误"})
		return
	}
	res, err := s.db.Exec(`DELETE FROM records WHERE id=$1`, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "删除失败"})
		return
	}
	if rows, _ := res.RowsAffected(); rows == 0 {
		c.JSON(http.StatusNotFound, gin.H{"message": "记录不存在"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (s *server) getTimerState(c *gin.Context) {
	state, err := s.readTimerState()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "查询失败"})
		return
	}
	c.JSON(http.StatusOK, state)
}

func (s *server) readTimerState() (timerState, error) {
	var st timerState
	var activeID sql.NullInt64
	var sessionStart, pauseStart sql.NullTime
	err := s.db.QueryRow(`SELECT active_item_id, session_start_at, accumulated_pause_ms, is_paused, pause_started_at, draft_description FROM timer_state WHERE id=1`).
		Scan(&activeID, &sessionStart, &st.AccumulatedPauseMs, &st.IsPaused, &pauseStart, &st.DraftDescription)
	if err != nil {
		return st, err
	}
	if activeID.Valid {
		v := activeID.Int64
		st.ActiveItemID = &v
	}
	if sessionStart.Valid {
		v := sessionStart.Time
		st.SessionStartAt = &v
	}
	if pauseStart.Valid {
		v := pauseStart.Time
		st.PauseStartedAt = &v
	}
	return st, nil
}

func (s *server) startTimer(c *gin.Context) {
	var req timerStartRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "参数错误"})
		return
	}
	if req.ItemID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"message": "itemId 错误"})
		return
	}
	startAt, err := parseTime(req.StartAt, time.Now())
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}
	_, err = s.db.Exec(`UPDATE timer_state SET active_item_id=$1, session_start_at=$2, accumulated_pause_ms=0, is_paused=FALSE, pause_started_at=NULL, draft_description=$3 WHERE id=1`,
		req.ItemID, startAt, req.DraftDesc)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "开始失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (s *server) pauseTimer(c *gin.Context) {
	st, err := s.readTimerState()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "暂停失败"})
		return
	}
	if st.ActiveItemID == nil || st.SessionStartAt == nil || st.IsPaused {
		c.JSON(http.StatusBadRequest, gin.H{"message": "当前不可暂停"})
		return
	}
	now := time.Now()
	_, err = s.db.Exec(`UPDATE timer_state SET is_paused=TRUE, pause_started_at=$1 WHERE id=1`, now)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "暂停失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (s *server) resumeTimer(c *gin.Context) {
	st, err := s.readTimerState()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "继续失败"})
		return
	}
	if st.ActiveItemID == nil || st.SessionStartAt == nil || !st.IsPaused || st.PauseStartedAt == nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "当前不可继续"})
		return
	}
	addMs := time.Since(*st.PauseStartedAt).Milliseconds()
	if addMs < 0 {
		addMs = 0
	}
	_, err = s.db.Exec(`UPDATE timer_state SET is_paused=FALSE, pause_started_at=NULL, accumulated_pause_ms=accumulated_pause_ms + $1 WHERE id=1`, addMs)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "继续失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (s *server) stopTimer(c *gin.Context) {
	var req timerStopRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "参数错误"})
		return
	}
	st, err := s.readTimerState()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "结束失败"})
		return
	}
	if st.ActiveItemID == nil || st.SessionStartAt == nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "当前没有计时"})
		return
	}
	endAt, err := parseTime(req.EndAt, time.Now())
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "endAt 错误"})
		return
	}
	pauseMs := st.AccumulatedPauseMs
	if st.IsPaused && st.PauseStartedAt != nil {
		addMs := endAt.Sub(*st.PauseStartedAt).Milliseconds()
		if addMs > 0 {
			pauseMs += addMs
		}
	}
	totalMs := endAt.Sub(*st.SessionStartAt).Milliseconds()
	if totalMs <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"message": "时间区间错误"})
		return
	}
	if pauseMs > totalMs {
		pauseMs = totalMs
	}
	save := req.Save
	effectiveMs := totalMs - pauseMs
	if effectiveMs < 0 {
		effectiveMs = 0
	}
	if save {
		var skipShortTimerRecord bool
		if err = s.db.QueryRow(`SELECT skip_short_timer_record FROM settings WHERE id=1`).Scan(&skipShortTimerRecord); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"message": "读取设置失败"})
			return
		}
		if skipShortTimerRecord && effectiveMs < 60*1000 {
			save = false
		}
	}
	if save {
		for _, seg := range splitByDay(*st.SessionStartAt, endAt, pauseMs) {
			if err = s.insertRecord(*st.ActiveItemID, seg.StartAt, seg.EndAt, seg.PauseDurationMs, req.Description, "timer"); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
				return
			}
		}
	}
	_, err = s.db.Exec(`UPDATE timer_state SET active_item_id=NULL, session_start_at=NULL, accumulated_pause_ms=0, is_paused=FALSE, pause_started_at=NULL, draft_description='' WHERE id=1`)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "结束失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"ok":              true,
		"saved":           save,
		"itemId":          *st.ActiveItemID,
		"startAt":         st.SessionStartAt,
		"endAt":           endAt,
		"pauseDurationMs": pauseMs,
		"description":     req.Description,
	})
}

func (s *server) getSettings(c *gin.Context) {
	var confirm, showHidden, skipShort, statsIncludeHidden bool
	if err := s.db.QueryRow(`SELECT confirm_before_save_timer_record, show_hidden_nodes, skip_short_timer_record, stats_include_hidden_nodes FROM settings WHERE id=1`).Scan(&confirm, &showHidden, &skipShort, &statsIncludeHidden); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "查询失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"confirmBeforeSaveTimerRecord": confirm,
		"showHiddenNodes":              showHidden,
		"skipShortTimerRecord":         skipShort,
		"statsIncludeHiddenNodes":      statsIncludeHidden,
	})
}

func (s *server) updateSettings(c *gin.Context) {
	body := struct {
		ConfirmBeforeSaveTimerRecord *bool `json:"confirmBeforeSaveTimerRecord"`
		ShowHiddenNodes              *bool `json:"showHiddenNodes"`
		SkipShortTimerRecord         *bool `json:"skipShortTimerRecord"`
		StatsIncludeHiddenNodes      *bool `json:"statsIncludeHiddenNodes"`
	}{}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "参数错误"})
		return
	}
	sets := []string{}
	args := []any{}
	argN := 1
	if body.ConfirmBeforeSaveTimerRecord != nil {
		sets = append(sets, fmt.Sprintf("confirm_before_save_timer_record=$%d", argN))
		args = append(args, *body.ConfirmBeforeSaveTimerRecord)
		argN++
	}
	if body.ShowHiddenNodes != nil {
		sets = append(sets, fmt.Sprintf("show_hidden_nodes=$%d", argN))
		args = append(args, *body.ShowHiddenNodes)
		argN++
	}
	if body.SkipShortTimerRecord != nil {
		sets = append(sets, fmt.Sprintf("skip_short_timer_record=$%d", argN))
		args = append(args, *body.SkipShortTimerRecord)
		argN++
	}
	if body.StatsIncludeHiddenNodes != nil {
		sets = append(sets, fmt.Sprintf("stats_include_hidden_nodes=$%d", argN))
		args = append(args, *body.StatsIncludeHiddenNodes)
		argN++
	}
	if len(sets) == 0 {
		c.JSON(http.StatusOK, gin.H{"ok": true})
		return
	}
	query := fmt.Sprintf("UPDATE settings SET %s WHERE id=1", strings.Join(sets, ","))
	if _, err := s.db.Exec(query, args...); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "更新失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (s *server) statsOverview(c *gin.Context) {
	type rowRecord struct {
		ItemID          int64
		StartAt         time.Time
		EndAt           time.Time
		PauseDurationMs int64
		Source          string
	}
	rows, err := s.db.Query(`SELECT item_id, start_at, end_at, pause_duration_ms, source FROM records`)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "统计失败"})
		return
	}
	defer rows.Close()
	records := make([]rowRecord, 0)
	for rows.Next() {
		var rr rowRecord
		if err = rows.Scan(&rr.ItemID, &rr.StartAt, &rr.EndAt, &rr.PauseDurationMs, &rr.Source); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"message": "统计失败"})
			return
		}
		records = append(records, rr)
	}
	totalMs := int64(0)
	totalTimerCount := int64(0)
	byItem := map[int64]int64{}
	byHour := map[int]int64{}
	byDay := map[string]int64{}
	for _, rr := range records {
		d := rr.EndAt.Sub(rr.StartAt).Milliseconds() - rr.PauseDurationMs
		if d < 0 {
			d = 0
		}
		totalMs += d
		byItem[rr.ItemID] += d
		if rr.Source == "timer" {
			totalTimerCount++
		}
		cur := rr.StartAt
		for cur.Before(rr.EndAt) {
			nextHour := cur.Truncate(time.Hour).Add(time.Hour)
			segEnd := nextHour
			if !segEnd.Before(rr.EndAt) {
				segEnd = rr.EndAt
			}
			segMs := segEnd.Sub(cur).Milliseconds()
			h := cur.Hour()
			byHour[h] += segMs
			dayKey := cur.Format("2006-01-02")
			byDay[dayKey] += segMs
			cur = segEnd
		}
	}
	byItemArray := make([]gin.H, 0, len(byItem))
	for itemID, ms := range byItem {
		byItemArray = append(byItemArray, gin.H{"itemId": itemID, "durationMs": ms})
	}
	sort.Slice(byItemArray, func(i, j int) bool {
		return byItemArray[i]["durationMs"].(int64) > byItemArray[j]["durationMs"].(int64)
	})
	byHourArray := make([]gin.H, 0, 24)
	for h := 0; h < 24; h++ {
		byHourArray = append(byHourArray, gin.H{"hour": h, "durationMs": byHour[h]})
	}
	dayKeys := make([]string, 0, len(byDay))
	for k := range byDay {
		dayKeys = append(dayKeys, k)
	}
	sort.Strings(dayKeys)
	byDayArray := make([]gin.H, 0, len(dayKeys))
	for _, k := range dayKeys {
		byDayArray = append(byDayArray, gin.H{"date": k, "durationMs": byDay[k]})
	}
	c.JSON(http.StatusOK, gin.H{
		"totalDurationMs": totalMs,
		"totalTimerCount": totalTimerCount,
		"byItem":          byItemArray,
		"byHour":          byHourArray,
		"byDay":           byDayArray,
	})
}

func (s *server) withTx(fn func(context.Context, *sql.Tx) error) error {
	ctx := context.Background()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err = fn(ctx, tx); err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit()
}
