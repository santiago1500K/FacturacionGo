package common

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

// ErrorResponse es el formato de error que devuelven todos los servicios.
type ErrorResponse struct {
	Error string `json:"error"`
}

// WriteJSON serializa v como JSON con el código de estado indicado.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if v != nil {
		_ = json.NewEncoder(w).Encode(v)
	}
}

// WriteError responde {"error": msg}.
func WriteError(w http.ResponseWriter, status int, msg string) {
	WriteJSON(w, status, ErrorResponse{Error: msg})
}

// DecodeJSON lee el cuerpo de la petición (máximo 1 MB) en v.
func DecodeJSON(r *http.Request, v any) error {
	r.Body = http.MaxBytesReader(nil, r.Body, 1<<20)
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		return errors.New("JSON inválido: " + err.Error())
	}
	return nil
}

// PathID convierte el comodín {name} de la ruta en entero.
func PathID(r *http.Request, name string) (int, error) {
	id, err := strconv.Atoi(r.PathValue(name))
	if err != nil {
		return 0, errors.New("el id debe ser un número entero")
	}
	return id, nil
}

// BearerToken extrae el token del encabezado "Authorization: Bearer <token>".
func BearerToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if len(h) > 7 && strings.EqualFold(h[:7], "Bearer ") {
		return strings.TrimSpace(h[7:])
	}
	return ""
}

// Env devuelve la variable de entorno key o def si no está definida.
func Env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// CORS permite que el frontend (servido en otro puerto) llame al servicio.
func CORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Access-Control-Allow-Origin", "*")
		h.Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		h.Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

// Logger registra método, ruta, código de respuesta y duración.
func Logger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		log.Printf("%s %s -> %d (%s)", r.Method, r.URL.Path, rec.status, time.Since(start).Round(time.Millisecond))
	})
}

// Serve arranca el servidor HTTP con CORS y logs activados.
func Serve(nombre, port string, mux http.Handler) {
	addr := ":" + port
	log.Printf("Servicio de %s escuchando en http://localhost%s", nombre, addr)
	srv := &http.Server{
		Addr:              addr,
		Handler:           Logger(CORS(mux)),
		ReadHeaderTimeout: 10 * time.Second,
	}
	log.Fatal(srv.ListenAndServe())
}
