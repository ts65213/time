package main

import (
	"database/sql"
	"fmt"
	"log"
	"math/rand"
	"os"
	"time"

	"github.com/joho/godotenv"
	_ "github.com/jackc/pgx/v5/stdlib"
)

func ensureCategoryID(tx *sql.Tx) (int64, error) {
	var categoryID int64
	err := tx.QueryRow(`SELECT id FROM nodes WHERE type='category' AND name='测试数据' ORDER BY id LIMIT 1`).Scan(&categoryID)
	if err == nil {
		return categoryID, nil
	}
	if err != sql.ErrNoRows {
		return 0, err
	}
	if err = tx.QueryRow(`INSERT INTO nodes(type, name, parent_id, order_no, hidden, collapsed, daily_target_minutes) VALUES ('category', '测试数据', NULL, 99999, FALSE, FALSE, 0) RETURNING id`).Scan(&categoryID); err != nil {
		return 0, err
	}
	return categoryID, nil
}

func ensureItemID(tx *sql.Tx, categoryID int64, name string, orderNo int64) (int64, error) {
	var itemID int64
	err := tx.QueryRow(`SELECT id FROM nodes WHERE type='item' AND name=$1 ORDER BY id LIMIT 1`, name).Scan(&itemID)
	if err == nil {
		return itemID, nil
	}
	if err != sql.ErrNoRows {
		return 0, err
	}
	if err = tx.QueryRow(`INSERT INTO nodes(type, name, parent_id, order_no, hidden, collapsed, daily_target_minutes) VALUES ('item', $1, $2, $3, FALSE, FALSE, 60) RETURNING id`, name, categoryID, orderNo).Scan(&itemID); err != nil {
		return 0, err
	}
	return itemID, nil
}

func main() {
	_ = godotenv.Load()
	dsn := os.Getenv("DB_DSN")
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

	tx, err := db.Begin()
	if err != nil {
		log.Fatal(err)
	}
	defer tx.Rollback()

	categoryID, err := ensureCategoryID(tx)
	if err != nil {
		log.Fatal(err)
	}

	testItemID, err := ensureItemID(tx, categoryID, "测试事项", 1001)
	if err != nil {
		log.Fatal(err)
	}
	item2ID, err := ensureItemID(tx, categoryID, "事项2", 1002)
	if err != nil {
		log.Fatal(err)
	}

	if _, err = tx.Exec(`DELETE FROM records WHERE description LIKE '[seed-random-2024-now]%'`); err != nil {
		log.Fatal(err)
	}

	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	now := time.Now()
	startYear := 2024
	items := []struct {
		id   int64
		name string
	}{
		{id: testItemID, name: "测试事项"},
		{id: item2ID, name: "事项2"},
	}
	insertedByItem := map[string]int{
		"测试事项": 0,
		"事项2":  0,
	}
	totalInserted := 0

	for _, item := range items {
		for year := startYear; year <= now.Year(); year++ {
			maxMonth := 12
			if year == now.Year() {
				maxMonth = int(now.Month())
			}
			for month := 1; month <= maxMonth; month++ {
				recordCount := 1 + rng.Intn(4)
				for i := 0; i < recordCount; i++ {
					maxDay := time.Date(year, time.Month(month)+1, 0, 0, 0, 0, 0, time.Local).Day()
					day := 1 + rng.Intn(maxDay)
					hour := 7 + rng.Intn(14)
					minute := []int{0, 10, 20, 30, 40, 50}[rng.Intn(6)]
					startAt := time.Date(year, time.Month(month), day, hour, minute, 0, 0, time.Local)
					if startAt.After(now) {
						startAt = now.Add(-time.Duration(1+rng.Intn(72)) * time.Hour)
					}
					durationMin := 20 + rng.Intn(341)
					pauseMin := rng.Intn(21)
					if pauseMin >= durationMin {
						pauseMin = durationMin / 3
					}
					endAt := startAt.Add(time.Duration(durationMin) * time.Minute)
					if endAt.After(now) {
						endAt = now.Add(-time.Duration(rng.Intn(60)) * time.Minute)
						if !endAt.After(startAt) {
							endAt = startAt.Add(30 * time.Minute)
						}
					}
					source := "manual"
					if rng.Intn(2) == 0 {
						source = "timer"
					}
					desc := fmt.Sprintf("[seed-random-2024-now] %s %04d-%02d #%d", item.name, year, month, i+1)
					_, err = tx.Exec(
						`INSERT INTO records(item_id, start_at, end_at, pause_duration_ms, description, source) VALUES ($1, $2, $3, $4, $5, $6)`,
						item.id,
						startAt,
						endAt,
						int64(pauseMin)*60*1000,
						desc,
						source,
					)
					if err != nil {
						log.Fatal(err)
					}
					insertedByItem[item.name]++
					totalInserted++
				}
			}
		}
	}

	if err = tx.Commit(); err != nil {
		log.Fatal(err)
	}

	fmt.Printf("seed complete: 测试事项(item_id=%d)=%d, 事项2(item_id=%d)=%d, total=%d\n",
		testItemID, insertedByItem["测试事项"], item2ID, insertedByItem["事项2"], totalInserted)
}
