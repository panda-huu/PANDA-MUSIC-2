package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"strconv"
	"sync"
	"time"

	_ "github.com/lib/pq"
)

var (
	memMu          sync.Mutex
	servedChats    = map[int64]struct{}{}
	servedUsers    = map[int64]struct{}{}
	playCounts     = map[int64]int64{}
	broadcastChats = map[int64]string{}
	blockedGroups  = map[int64]struct{}{}
	blockedUsers   = map[int64]struct{}{}
	chatEffects    = map[int64]effectState{}
	storeDB        *sql.DB
)

const dbPrefix = "radhika_"

func startStore() error {
	host := os.Getenv("DB_HOST")
	user := os.Getenv("DB_USER")
	password := os.Getenv("DB_PASSWORD")
	name := os.Getenv("DB_NAME")
	port := os.Getenv("DB_PORT")
	if port == "" { port = "6543" }
	if name == "" { name = "postgres" }
	if host == "" || user == "" || password == "" {
		return fmt.Errorf("PostgreSQL config missing: DB_HOST, DB_USER, DB_PASSWORD required")
	}
	portNum, err := strconv.Atoi(port)
	if err != nil || portNum < 1 || portNum > 65535 {
		return fmt.Errorf("invalid DB_PORT %q", port)
	}
	dsn := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=require connect_timeout=10", host, portNum, user, password, name)
	db, err := sql.Open("postgres", dsn)
	if err != nil { return fmt.Errorf("open PostgreSQL: %w", err) }
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(3)
	db.SetConnMaxLifetime(30*time.Minute)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err = db.PingContext(ctx); err != nil {
		_ = db.Close()
		return fmt.Errorf("connect PostgreSQL: %w", err)
	}
	storeDB = db
	if err = createStoreTables(ctx); err != nil {
		_ = db.Close()
		storeDB = nil
		return err
	}
	if err = loadStoreData(ctx); err != nil {
		_ = db.Close()
		storeDB = nil
		return err
	}
	log.Printf("PostgreSQL connected; database tables use prefix %q", dbPrefix)
	return nil
}

func createStoreTables(ctx context.Context) error {
	stmts := []string{
		"CREATE TABLE IF NOT EXISTS " + dbPrefix + "served_users (user_id BIGINT PRIMARY KEY, added_at TIMESTAMPTZ NOT NULL DEFAULT NOW())",
		"CREATE TABLE IF NOT EXISTS " + dbPrefix + "served_chats (chat_id BIGINT PRIMARY KEY, added_at TIMESTAMPTZ NOT NULL DEFAULT NOW())",
		"CREATE TABLE IF NOT EXISTS " + dbPrefix + "play_counts (chat_id BIGINT PRIMARY KEY, plays BIGINT NOT NULL DEFAULT 0)",
		"CREATE TABLE IF NOT EXISTS " + dbPrefix + "broadcast_chats (chat_id BIGINT PRIMARY KEY, kind TEXT NOT NULL DEFAULT 'group')",
		"CREATE TABLE IF NOT EXISTS " + dbPrefix + "blocked_groups (chat_id BIGINT PRIMARY KEY)",
		"CREATE TABLE IF NOT EXISTS " + dbPrefix + "blocked_users (user_id BIGINT PRIMARY KEY)",
		"CREATE TABLE IF NOT EXISTS " + dbPrefix + "chat_effects (chat_id BIGINT PRIMARY KEY, speed DOUBLE PRECISION NOT NULL DEFAULT 1, bass INTEGER NOT NULL DEFAULT 0, enabled BOOLEAN NOT NULL DEFAULT FALSE)",
	}
	for _, q := range stmts {
		if _, err := storeDB.ExecContext(ctx, q); err != nil { return fmt.Errorf("create database schema: %w", err) }
	}
	return nil
}

func loadStoreData(ctx context.Context) error {
	loadIDs := func(table, column string, target map[int64]struct{}) error {
		rows, err := storeDB.QueryContext(ctx, "SELECT "+column+" FROM "+table)
		if err != nil { return err }
		defer rows.Close()
		for rows.Next() { var id int64; if err := rows.Scan(&id); err != nil { return err }; target[id] = struct{}{} }
		return rows.Err()
	}
	memMu.Lock()
	defer memMu.Unlock()
	if err := loadIDs(dbPrefix+"served_users", "user_id", servedUsers); err != nil { return fmt.Errorf("load served users: %w", err) }
	rowsChats, err := storeDB.QueryContext(ctx, "SELECT chat_id FROM "+dbPrefix+"served_chats WHERE chat_id < 0")
	if err != nil { return fmt.Errorf("load served chats: %w", err) }
	for rowsChats.Next() { var id int64; if err := rowsChats.Scan(&id); err != nil { rowsChats.Close(); return err }; servedChats[id] = struct{}{} }
	if err := rowsChats.Err(); err != nil { rowsChats.Close(); return err }; rowsChats.Close()
	if err := loadIDs(dbPrefix+"blocked_groups", "chat_id", blockedGroups); err != nil { return fmt.Errorf("load blocked groups: %w", err) }
	if err := loadIDs(dbPrefix+"blocked_users", "user_id", blockedUsers); err != nil { return fmt.Errorf("load blocked users: %w", err) }
	rows, err := storeDB.QueryContext(ctx, "SELECT chat_id, plays FROM "+dbPrefix+"play_counts")
	if err != nil { return fmt.Errorf("load play counts: %w", err) }
	for rows.Next() { var id, n int64; if err := rows.Scan(&id, &n); err != nil { rows.Close(); return err }; playCounts[id] = n }
	if err := rows.Err(); err != nil { rows.Close(); return err }; rows.Close()
	rows, err = storeDB.QueryContext(ctx, "SELECT chat_id, kind FROM "+dbPrefix+"broadcast_chats")
	if err != nil { return fmt.Errorf("load broadcast chats: %w", err) }
	for rows.Next() { var id int64; var kind string; if err := rows.Scan(&id, &kind); err != nil { rows.Close(); return err }; broadcastChats[id] = kind }
	if err := rows.Err(); err != nil { rows.Close(); return err }; rows.Close()
	rows, err = storeDB.QueryContext(ctx, "SELECT chat_id, speed, bass, enabled FROM "+dbPrefix+"chat_effects")
	if err != nil { return fmt.Errorf("load chat effects: %w", err) }
	for rows.Next() { var id int64; var s effectState; if err := rows.Scan(&id, &s.Speed, &s.Bass, &s.Enabled); err != nil { rows.Close(); return err }; chatEffects[id] = s }
	if err := rows.Err(); err != nil { rows.Close(); return err }; rows.Close()
	return nil
}

func storeExec(query string, args ...any) {
	if storeDB == nil { return }
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := storeDB.ExecContext(ctx, query, args...); err != nil { log.Printf("[database] write failed: %v", err) }
}

func addServedChat(chatID int64) {
	if chatID == 0 { return }
	memMu.Lock(); _, exists := servedChats[chatID]; servedChats[chatID] = struct{}{}; memMu.Unlock()
	if !exists { storeExec("INSERT INTO "+dbPrefix+"served_chats(chat_id) VALUES($1) ON CONFLICT DO NOTHING", chatID) }
}

func addServedUser(userID int64) bool {
	if userID <= 0 { return false }
	memMu.Lock(); _, exists := servedUsers[userID]; servedUsers[userID] = struct{}{}; memMu.Unlock()
	if !exists { storeExec("INSERT INTO "+dbPrefix+"served_users(user_id) VALUES($1) ON CONFLICT DO NOTHING", userID) }
	return !exists
}

func incrementPlayCount(chatID int64) {
	memMu.Lock(); playCounts[chatID]++; memMu.Unlock()
	storeExec("INSERT INTO "+dbPrefix+"play_counts(chat_id, plays) VALUES($1, 1) ON CONFLICT(chat_id) DO UPDATE SET plays="+dbPrefix+"play_counts.plays+1", chatID)
}

func getServedChatsCount() int64 { memMu.Lock(); defer memMu.Unlock(); return int64(len(servedChats)) }

func getSudoersCount() int64 {
	if storeDB == nil { return 0 }
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var count sql.NullInt64
	err := storeDB.QueryRowContext(ctx, "SELECT COALESCE(array_length(sudoers, 1), 0) FROM "+dbPrefix+"sudoers WHERE id='sudo'").Scan(&count)
	if err != nil { return 0 }
	n := count.Int64
	if owner, err := strconv.ParseInt(os.Getenv("OWNER_ID"), 10, 64); err == nil && owner > 0 {
		var exists bool
		_ = storeDB.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM "+dbPrefix+"sudoers WHERE id='sudo' AND $1 = ANY(sudoers))", owner).Scan(&exists)
		if !exists { n++ }
	}
	return n
}
func getServedUsersCount() int64 { memMu.Lock(); defer memMu.Unlock(); return int64(len(servedUsers)) }
func getTotalPlays() int64 { memMu.Lock(); defer memMu.Unlock(); var n int64; for _, v := range playCounts { n += v }; return n }

func addBroadcastChat(chatID int64, kind string) {
	if chatID == 0 { return }; if kind == "" { kind = "group" }
	memMu.Lock(); broadcastChats[chatID] = kind; memMu.Unlock()
	storeExec("INSERT INTO "+dbPrefix+"broadcast_chats(chat_id, kind) VALUES($1,$2) ON CONFLICT(chat_id) DO UPDATE SET kind=EXCLUDED.kind", chatID, kind)
}
func getBroadcastChats() []int64 { memMu.Lock(); defer memMu.Unlock(); ids:=make([]int64,0,len(broadcastChats)); for id:=range broadcastChats { ids=append(ids,id) }; return ids }

func isGroupBlocked(chatID int64) bool { memMu.Lock(); defer memMu.Unlock(); _,ok:=blockedGroups[chatID]; return ok }
func blockGroup(chatID int64) { memMu.Lock(); blockedGroups[chatID]=struct{}{}; memMu.Unlock(); storeExec("INSERT INTO "+dbPrefix+"blocked_groups(chat_id) VALUES($1) ON CONFLICT DO NOTHING",chatID) }
func unblockGroup(chatID int64) { memMu.Lock(); delete(blockedGroups,chatID); memMu.Unlock(); storeExec("DELETE FROM "+dbPrefix+"blocked_groups WHERE chat_id=$1",chatID) }
func getBlockedGroups() []int64 { memMu.Lock(); defer memMu.Unlock(); ids:=make([]int64,0,len(blockedGroups)); for id:=range blockedGroups { ids=append(ids,id) }; return ids }

func isUserBlockedDB(userID int64) bool { memMu.Lock(); defer memMu.Unlock(); _,ok:=blockedUsers[userID]; return ok }
func blockUser(userID int64) { memMu.Lock(); blockedUsers[userID]=struct{}{}; memMu.Unlock(); storeExec("INSERT INTO "+dbPrefix+"blocked_users(user_id) VALUES($1) ON CONFLICT DO NOTHING",userID) }
func unblockUser(userID int64) { memMu.Lock(); delete(blockedUsers,userID); memMu.Unlock(); storeExec("DELETE FROM "+dbPrefix+"blocked_users WHERE user_id=$1",userID) }
func getBlockedUsers() []int64 { memMu.Lock(); defer memMu.Unlock(); ids:=make([]int64,0,len(blockedUsers)); for id:=range blockedUsers { ids=append(ids,id) }; return ids }

func saveChatEffects(chatID int64, speed float64, bass int, enabled bool) {
	s:=effectState{Speed:speed,Bass:bass,Enabled:enabled}
	memMu.Lock(); chatEffects[chatID]=s; memMu.Unlock()
	storeExec("INSERT INTO "+dbPrefix+"chat_effects(chat_id,speed,bass,enabled) VALUES($1,$2,$3,$4) ON CONFLICT(chat_id) DO UPDATE SET speed=EXCLUDED.speed,bass=EXCLUDED.bass,enabled=EXCLUDED.enabled",chatID,speed,bass,enabled)
}
func loadChatEffects(chatID int64) (speed float64, bass int, enabled bool) {
	memMu.Lock(); defer memMu.Unlock()
	if s,ok:=chatEffects[chatID]; ok { return s.Speed,s.Bass,s.Enabled }
	return 1,0,false
}
