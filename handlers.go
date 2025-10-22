package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"net/smtp"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/xuri/excelize/v2"
)

var templates *template.Template

func initTemplates() error {
	// Просто парсим шаблоны без функций
	var err error
	templates, err = template.ParseFiles("templates/index.html", "templates/edit.html")
	if err != nil {
		return err
	}

	return nil
}

func formatDateForTemplate(dateStr string) string {
	if dateStr == "" {
		return "N/A"
	}

	// Пробуем разные форматы
	formats := []string{
		"2006-01-02 15:04:05",
		"2006-01-02",
		"02.01.2006",
		"02.01.2006 15:04:05",
		time.RFC3339,
	}

	for _, format := range formats {
		if t, err := time.Parse(format, dateStr); err == nil {
			return t.Format("02.01.2006")
		}
	}

	// Если не удалось распарсить, возвращаем как есть
	return dateStr
}

func indexHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	tasks, err := getTasks("в работе", 7)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	err = templates.ExecuteTemplate(w, "index.html", map[string]interface{}{
		"Tasks": tasks,
		"Filter": map[string]string{
			"Status": "в работе",
			"Days":   "7",
		},
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func uploadHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Увеличиваем лимит размера файла
	r.ParseMultipartForm(32 << 20) // 32 MB

	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "Error retrieving file: "+err.Error(), http.StatusBadRequest)
		return
	}
	defer file.Close()

	// Проверка расширения файла
	if filepath.Ext(header.Filename) != ".xlsx" {
		http.Error(w, "File must be .xlsx", http.StatusBadRequest)
		return
	}

	f, err := excelize.OpenReader(file)
	if err != nil {
		http.Error(w, "Error reading Excel file: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer f.Close()

	// Чтение данных из первого листа
	sheetName := f.GetSheetName(0)
	rows, err := f.GetRows(sheetName)
	if err != nil {
		http.Error(w, "Error reading sheet: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if len(rows) < 2 {
		http.Error(w, "File must contain at least 2 rows", http.StatusBadRequest)
		return
	}

	// Первая строка - дата собрания (берем только дату, без времени)
	meetingDate := ""
	if len(rows[0]) > 0 {
		meetingDate = formatDateFromExcel(rows[0][0])
	}

	// Обработка задач
	importedCount := 0
	for i := 1; i < len(rows); i++ {
		if len(rows[i]) < 4 {
			log.Printf("Skipping row %d: insufficient columns", i+1)
			continue
		}

		// Пропускаем пустые строки
		if isRowEmpty(rows[i]) {
			continue
		}

		task := Task{
			MeetingDate:  meetingDate,
			TaskNumber:   strings.TrimSpace(safeGet(rows[i], 0)),
			TaskText:     strings.TrimSpace(safeGet(rows[i], 1)),
			Responsibles: strings.TrimSpace(safeGet(rows[i], 2)),
			Deadline:     formatDateFromExcel(strings.TrimSpace(safeGet(rows[i], 3))),
			Comment:      "",
			Status:       "в работе",
		}

		// Валидация обязательных полей
		if task.TaskText == "" {
			log.Printf("Skipping row %d: empty task text", i+1)
			continue
		}

		err := insertTask(task)
		if err != nil {
			log.Printf("Error inserting task at row %d: %v", i+1, err)
			continue
		}
		importedCount++
	}

	log.Printf("Successfully imported %d tasks", importedCount)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// Функция для форматирования даты из Excel
func formatDateFromExcel(dateStr string) string {
	if dateStr == "" {
		return ""
	}

	// Пытаемся разобрать различные форматы дат
	formats := []string{
		"2006-01-02 15:04:05",
		"2006-01-02",
		"02.01.2006",
		"02.01.2006 15:04:05",
		time.RFC3339,
	}

	for _, format := range formats {
		if t, err := time.Parse(format, dateStr); err == nil {
			return t.Format("02.01.2006")
		}
	}

	// Если не удалось распарсить, возвращаем как есть (обрезаем время если есть)
	if strings.Contains(dateStr, " ") {
		parts := strings.Split(dateStr, " ")
		return parts[0] // Берем только дату
	}

	return dateStr
}

// Функция проверки пустой строки
func isRowEmpty(row []string) bool {
	for _, cell := range row {
		if strings.TrimSpace(cell) != "" {
			return false
		}
	}
	return true
}

// Вспомогательная функция для безопасного получения элемента массива
func safeGet(slice []string, index int) string {
	if index < len(slice) {
		return slice[index]
	}
	return ""
}

func tasksHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	status := r.URL.Query().Get("status")
	if status == "" {
		status = "в работе"
	}

	daysStr := r.URL.Query().Get("days")
	if daysStr == "" {
		daysStr = "7"
	}
	days, err := strconv.Atoi(daysStr)
	if err != nil {
		days = 7
	}

	log.Printf("Fetching tasks: status=%s, days=%d", status, days)

	tasks, err := getTasks(status, days)
	if err != nil {
		log.Printf("Error getting tasks: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	log.Printf("Found %d tasks", len(tasks))

	// Преобразуем задачи в JSON с правильными именами полей
	type JSONTask struct {
		ID           int    `json:"id"`
		MeetingDate  string `json:"meetingDate"`
		TaskNumber   string `json:"taskNumber"`
		TaskText     string `json:"taskText"`
		Responsibles string `json:"responsibles"`
		Deadline     string `json:"deadline"`
		Comment      string `json:"comment"`
		Status       string `json:"status"`
		StatusDate   string `json:"statusDate"`
	}

	var jsonTasks []JSONTask
	for _, task := range tasks {
		jsonTasks = append(jsonTasks, JSONTask{
			ID:           task.ID,
			MeetingDate:  task.MeetingDate,
			TaskNumber:   task.TaskNumber,
			TaskText:     task.TaskText,
			Responsibles: task.Responsibles,
			Deadline:     task.Deadline,
			Comment:      task.Comment,
			Status:       task.Status,
			StatusDate:   task.StatusDate,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(jsonTasks)
}

func editTaskHandler(w http.ResponseWriter, r *http.Request) {
	idStr := strings.TrimPrefix(r.URL.Path, "/edit/")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "Invalid task ID", http.StatusBadRequest)
		return
	}

	task, err := getTaskByID(id)
	if err != nil {
		http.Error(w, "Task not found", http.StatusNotFound)
		return
	}

	err = templates.ExecuteTemplate(w, "edit.html", task)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func updateTaskHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	idStr := strings.TrimPrefix(r.URL.Path, "/update/")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "Invalid task ID", http.StatusBadRequest)
		return
	}

	comment := r.FormValue("comment")
	status := r.FormValue("status")

	err = updateTask(id, comment, status)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func sendEmailHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	idStr := strings.TrimPrefix(r.URL.Path, "/send-email/")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "Invalid task ID", http.StatusBadRequest)
		return
	}

	task, err := getTaskByID(id)
	if err != nil {
		http.Error(w, "Task not found", http.StatusNotFound)
		return
	}

	// Отправка email
	err = sendEmailToResponsibles(task)
	if err != nil {
		log.Printf("Error sending email for task %d: %v", task.ID, err)
		http.Error(w, "Error sending email: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Write([]byte("Email sent successfully"))
}

func sendEmailToResponsibles(task Task) error {
	// Получение параметров SMTP из переменных окружения
	smtpHost := getEnvLocal("SMTP_HOST", "")
	smtpPort := getEnvLocal("SMTP_PORT", "")
	smtpUsername := getEnvLocal("SMTP_USERNAME", "")
	smtpPassword := getEnvLocal("SMTP_PASSWORD", "")
	smtpFrom := getEnvLocal("SMTP_FROM", "")

	// Проверка обязательных параметров
	if smtpHost == "" || smtpPort == "" || smtpUsername == "" || smtpPassword == "" || smtpFrom == "" {
		return fmt.Errorf("SMTP configuration is incomplete. Please check your .env file")
	}

	// Разделение email адресов
	emails := strings.Split(task.Responsibles, ",")
	var validEmails []string
	for _, email := range emails {
		email = strings.TrimSpace(email)
		if email != "" && strings.Contains(email, "@") {
			validEmails = append(validEmails, email)
		}
	}

	if len(validEmails) == 0 {
		return fmt.Errorf("no valid email addresses found")
	}

	// Получение хоста сервера
	serverHost := getEnvLocal("SERVER_HOST", "localhost:8700")

	// Формирование ссылки для редактирования
	editURL := fmt.Sprintf("http://%s/edit/%d", serverHost, task.ID)

	// Формирование сообщения
	subject := "Task Update Required"
	body := fmt.Sprintf(`
Task: %s
Deadline: %s
Meeting Date: %s
Status: %s

Please update the task status by following this link:
%s

Best regards,
Task Management System
`, task.TaskText, task.Deadline, task.MeetingDate, task.Status, editURL)

	// Формирование email сообщения
	message := bytes.NewBuffer(nil)
	message.WriteString(fmt.Sprintf("From: %s\r\n", smtpFrom))
	message.WriteString(fmt.Sprintf("To: %s\r\n", strings.Join(validEmails, ",")))
	message.WriteString(fmt.Sprintf("Subject: %s\r\n", subject))
	message.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
	message.WriteString("\r\n")
	message.WriteString(body)

	// Аутентификация и отправка
	auth := smtp.PlainAuth("", smtpUsername, smtpPassword, smtpHost)
	err := smtp.SendMail(smtpHost+":"+smtpPort, auth, smtpFrom, validEmails, message.Bytes())

	if err != nil {
		return fmt.Errorf("failed to send email: %v", err)
	}

	log.Printf("Email sent successfully for task %d to: %s", task.ID, strings.Join(validEmails, ", "))
	return nil
}

// Добавьте эту функцию в handlers.go
func getEnvLocal(key, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value
}
