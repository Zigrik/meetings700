package main

import (
	"log"
	"net/http"
	"os"

	"github.com/joho/godotenv"
)

func main() {
	// Загрузка .env файла
	log.Println("Loading environment variables...")
	err := godotenv.Load()
	if err != nil {
		log.Printf("Warning: .env file not found: %v", err)
		log.Println("Using system environment variables instead")
	}

	// Инициализация шаблонов
	log.Println("Initializing templates...")
	err = initTemplates()
	if err != nil {
		log.Fatal("Error initializing templates:", err)
	}

	// Инициализация базы данных
	log.Println("Initializing database with go-sqlite...")
	err = initDB()
	if err != nil {
		log.Fatal("Error initializing database:", err)
	}
	defer db.Close()

	// Настройка маршрутов
	http.HandleFunc("/", indexHandler)
	http.HandleFunc("/upload", uploadHandler)
	http.HandleFunc("/tasks", tasksHandler)
	http.HandleFunc("/edit/", editTaskHandler)
	http.HandleFunc("/update/", updateTaskHandler)
	http.HandleFunc("/send-email/", sendEmailHandler)

	// Статические файлы
	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))

	log.Println("Server starting on port 8700...")
	log.Println("Using pure Go SQLite driver (no CGO required)")
	log.Fatal(http.ListenAndServe(":8700", nil))
}

// Вспомогательная функция для получения переменных окружения
func getEnv(key, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value
}
