package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/gorilla/mux"
	"github.com/redis/go-redis/v9"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Contexto para Redis y MongoDB
var ctx = context.Background()

// Configuración de Redis
var rdb *redis.Client

// Configuración de MongoDB
var mongoClient *mongo.Client
var userCollection *mongo.Collection

// User estructura para representar un usuario
type User struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

// Inicializa la conexión con Redis
func initRedis() {
	redisHost := os.Getenv("REDIS_HOST")
	if redisHost == "" {
		redisHost = "localhost:6379"
	}

	rdb = redis.NewClient(&redis.Options{
		Addr:     redisHost,
		Password: "", // Sin contraseña por defecto
		DB:       0,  // Base de datos por defecto
	})

	_, err := rdb.Ping(ctx).Result()
	if err != nil {
		log.Fatalf("No se pudo conectar a Redis: %v", err)
	}
	fmt.Println("Conectado a Redis")
}

// Inicializa la conexión con MongoDB
func initMongoDB() {
	mongoURI := os.Getenv("MONGO_URI")
	if mongoURI == "" {
		mongoURI = "mongodb://mongo:27017"
	}

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(mongoURI))
	if err != nil {
		log.Fatalf("No se pudo conectar a MongoDB: %v", err)
	}
	mongoClient = client
	userCollection = mongoClient.Database("userdb").Collection("users")
	fmt.Println("Conectado a MongoDB")
}

// Obtener usuarios desde Redis o MongoDB
func getUsers(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Intentar obtener los datos desde el caché de Redis
	cacheKey := "users"
	cachedData, err := rdb.Get(ctx, cacheKey).Result()
	if err == nil {
		// Si hay datos en el caché, devolverlos
		w.Write([]byte(cachedData))
		return
	}

	// Si no hay datos en el caché, obtener los datos de MongoDB
	cursor, err := userCollection.Find(ctx, bson.D{})
	if err != nil {
		http.Error(w, "Error al consultar MongoDB", http.StatusInternalServerError)
		return
	}
	defer cursor.Close(ctx)

	var users []User
	for cursor.Next(ctx) {
		var user User
		if err := cursor.Decode(&user); err != nil {
			http.Error(w, "Error al leer datos de MongoDB", http.StatusInternalServerError)
			return
		}
		users = append(users, user)
	}

	// Serializar los usuarios y almacenarlos en Redis
	data, _ := json.Marshal(users)
	rdb.Set(ctx, cacheKey, data, 10*time.Minute)

	// Enviar los datos de los usuarios
	w.Write(data)
}

// Crear un nuevo usuario y actualizar MongoDB y el caché
func createUser(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var user User
	_ = json.NewDecoder(r.Body).Decode(&user)

	// Guardar el usuario en MongoDB
	_, err := userCollection.InsertOne(ctx, user)
	if err != nil {
		http.Error(w, "Error al guardar el usuario en MongoDB", http.StatusInternalServerError)
		return
	}

	// Actualizar el caché en Redis
	cacheKey := "users"
	// Obtener todos los usuarios actuales de MongoDB
	cursor, err := userCollection.Find(ctx, bson.D{})
	if err != nil {
		http.Error(w, "Error al consultar MongoDB", http.StatusInternalServerError)
		return
	}
	defer cursor.Close(ctx)

	var users []User
	for cursor.Next(ctx) {
		var user User
		if err := cursor.Decode(&user); err != nil {
			http.Error(w, "Error al leer datos de MongoDB", http.StatusInternalServerError)
			return
		}
		users = append(users, user)
	}

	// Serializar los usuarios y almacenarlos en Redis
	data, _ := json.Marshal(users)
	rdb.Set(ctx, cacheKey, data, 10*time.Minute)

	// Devolver los datos del usuario recién creado
	w.Write(data)
}

func main() {
	initRedis()
	initMongoDB()

	router := mux.NewRouter()
	router.HandleFunc("/users", getUsers).Methods("GET")
	router.HandleFunc("/users", createUser).Methods("POST")

	log.Println("Servidor ejecutándose en el puerto 8080")
	log.Fatal(http.ListenAndServe(":8080", router))
}
