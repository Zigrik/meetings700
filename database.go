package main

import (
	"database/sql"
	"log"
	"time"

	_ "github.com/glebarez/go-sqlite"
)

var db *sql.DB

type Task struct {
	ID           int
	MeetingDate  string
	TaskNumber   string
	TaskText     string
	Responsibles string
	Deadline     string
	Comment      string
	Status       string
	StatusDate   string
}

func initDB() error {
	var err error
	// Используем go-sqlite (чистый Go, без CGO)
	db, err = sql.Open("sqlite", "./tasks.db")
	if err != nil {
		return err
	}

	// Создание таблицы
	createTableSQL := `
	CREATE TABLE IF NOT EXISTS tasks (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		meeting_date TEXT,
		task_number TEXT,
		task_text TEXT,
		responsibles TEXT,
		deadline TEXT,
		comment TEXT,
		status TEXT DEFAULT 'в работе',
		status_date TEXT DEFAULT CURRENT_TIMESTAMP
	);`

	_, err = db.Exec(createTableSQL)
	if err != nil {
		return err
	}

	log.Println("SQLite database initialized successfully (using go-sqlite)")
	return nil
}

func insertTask(task Task) error {
	query := `INSERT INTO tasks 
		(meeting_date, task_number, task_text, responsibles, deadline, comment, status, status_date) 
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`

	_, err := db.Exec(query,
		task.MeetingDate,
		task.TaskNumber,
		task.TaskText,
		task.Responsibles,
		task.Deadline,
		task.Comment,
		task.Status,
		time.Now().Format("2006-01-02 15:04:05"))

	return err
}

func getTasks(status string, days int) ([]Task, error) {
	var query string
	var rows *sql.Rows
	var err error

	// Расчет даты для фильтрации
	cutoffDate := time.Now().AddDate(0, 0, -days)
	cutoffDateStr := cutoffDate.Format("2006-01-02")

	log.Printf("Filter cutoff date: %s", cutoffDateStr)

	if status == "все" {
		query = `SELECT id, meeting_date, task_number, task_text, responsibles, deadline, comment, status, status_date 
                FROM tasks 
                WHERE date(status_date) >= ?
                ORDER BY deadline`
		rows, err = db.Query(query, cutoffDateStr)
	} else {
		query = `SELECT id, meeting_date, task_number, task_text, responsibles, deadline, comment, status, status_date 
                FROM tasks 
                WHERE status = ? AND date(status_date) >= ?
                ORDER BY deadline`
		rows, err = db.Query(query, status, cutoffDateStr)
	}

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []Task
	for rows.Next() {
		var task Task
		err := rows.Scan(&task.ID, &task.MeetingDate, &task.TaskNumber, &task.TaskText,
			&task.Responsibles, &task.Deadline, &task.Comment, &task.Status, &task.StatusDate)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, task)
	}

	return tasks, nil
}

func getTaskByID(id int) (Task, error) {
	var task Task
	query := `SELECT id, meeting_date, task_number, task_text, responsibles, deadline, comment, status, status_date 
			 FROM tasks WHERE id = ?`

	err := db.QueryRow(query, id).Scan(&task.ID, &task.MeetingDate, &task.TaskNumber,
		&task.TaskText, &task.Responsibles, &task.Deadline, &task.Comment, &task.Status, &task.StatusDate)

	return task, err
}

func updateTask(id int, comment, status string) error {
	query := `UPDATE tasks SET comment = ?, status = ?, status_date = ? WHERE id = ?`
	_, err := db.Exec(query, comment, status, time.Now().Format("2006-01-02 15:04:05"), id)
	return err
}

// Функция для очистки базы данных (опционально, для тестирования)
func clearTasks() error {
	_, err := db.Exec("DELETE FROM tasks")
	return err
}
