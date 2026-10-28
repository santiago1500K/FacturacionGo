// Servicio de Autenticación de Usuarios: registro, login y validación de tokens.
package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/mail"
	"strings"
	"time"

	"facturacion/common"
)

// Usuario tal como se guarda en data/usuarios.json.
type Usuario struct {
	ID       int    `json:"id"`
	Nombre   string `json:"nombre"`
	Email    string `json:"email"`
	Password string `json:"password"`
	Rol      string `json:"rol"`
}

// UsuarioPublico es lo que se expone por la API (sin contraseña).
type UsuarioPublico struct {
	ID     int    `json:"id"`
	Nombre string `json:"nombre"`
	Email  string `json:"email"`
	Rol    string `json:"rol"`
}

func (u Usuario) publico() UsuarioPublico {
	return UsuarioPublico{ID: u.ID, Nombre: u.Nombre, Email: u.Email, Rol: u.Rol}
}

type registroRequest struct {
	Nombre   string `json:"nombre"`
	Email    string `json:"email"`
	Password string `json:"password"`
	Rol      string `json:"rol"`
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type authResponse struct {
	Token   string         `json:"token"`
	Usuario UsuarioPublico `json:"usuario"`
}

// Roles que un usuario puede elegir al registrarse. "admin" solo se asigna
// directamente en el archivo usuarios.json.
var rolesRegistro = map[string]bool{"cliente": true, "vendedor": true}

const duracionToken = 8 * time.Hour

type server struct {
	usuarios *common.JSONStore[Usuario]
	secreto  []byte
}

func main() {
	hash := flag.String("hash", "", "imprime el hash de la contraseña indicada y termina")
	flag.Parse()
	if *hash != "" {
		h, err := hashPassword(*hash)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println(h)
		return
	}

	usuarios, err := common.NewJSONStore[Usuario](common.Env("DATA_FILE", "data/usuarios.json"))
	if err != nil {
		log.Fatal(err)
	}
	s := &server{usuarios: usuarios, secreto: []byte(common.Env("JWT_SECRET", "clave-secreta-desarrollo"))}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/auth/register", s.register)
	mux.HandleFunc("POST /api/auth/login", s.login)
	mux.HandleFunc("GET /api/auth/me", s.me)
	mux.HandleFunc("GET /api/auth/usuarios", s.listar)
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		common.WriteJSON(w, http.StatusOK, map[string]string{"estado": "ok", "servicio": "auth"})
	})

	common.Serve("autenticación", common.Env("PORT", "8081"), mux)
}

func (s *server) register(w http.ResponseWriter, r *http.Request) {
	var req registroRequest
	if err := common.DecodeJSON(r, &req); err != nil {
		common.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	req.Nombre = strings.TrimSpace(req.Nombre)
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	req.Rol = strings.ToLower(strings.TrimSpace(req.Rol))
	if req.Rol == "" {
		req.Rol = "vendedor"
	}

	switch {
	case req.Nombre == "":
		common.WriteError(w, http.StatusBadRequest, "el nombre es obligatorio")
		return
	case !emailValido(req.Email):
		common.WriteError(w, http.StatusBadRequest, "el email no es válido")
		return
	case len(req.Password) < 6:
		common.WriteError(w, http.StatusBadRequest, "la contraseña debe tener al menos 6 caracteres")
		return
	case !rolesRegistro[req.Rol]:
		common.WriteError(w, http.StatusBadRequest, "rol no permitido (use 'vendedor' o 'cliente')")
		return
	}

	hash, err := hashPassword(req.Password)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "no se pudo procesar la contraseña")
		return
	}

	var nuevo Usuario
	errDuplicado := errors.New("ya existe un usuario con ese email")
	err = s.usuarios.Update(func(items []Usuario) ([]Usuario, error) {
		maxID := 0
		for _, u := range items {
			if strings.EqualFold(u.Email, req.Email) {
				return nil, errDuplicado
			}
			maxID = max(maxID, u.ID)
		}
		nuevo = Usuario{ID: maxID + 1, Nombre: req.Nombre, Email: req.Email, Password: hash, Rol: req.Rol}
		return append(items, nuevo), nil
	})
	if errors.Is(err, errDuplicado) {
		common.WriteError(w, http.StatusConflict, err.Error())
		return
	}
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.responderConToken(w, http.StatusCreated, nuevo)
}

func (s *server) login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := common.DecodeJSON(r, &req); err != nil {
		common.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	email := strings.ToLower(strings.TrimSpace(req.Email))
	for _, u := range s.usuarios.All() {
		if strings.EqualFold(u.Email, email) && checkPassword(req.Password, u.Password) {
			s.responderConToken(w, http.StatusOK, u)
			return
		}
	}
	common.WriteError(w, http.StatusUnauthorized, "email o contraseña incorrectos")
}

// me valida el token Bearer y devuelve el usuario dueño del token.
// Los demás servicios (p. ej. facturación) lo usan para autenticar peticiones.
func (s *server) me(w http.ResponseWriter, r *http.Request) {
	token := common.BearerToken(r)
	if token == "" {
		common.WriteError(w, http.StatusUnauthorized, "falta el encabezado Authorization: Bearer <token>")
		return
	}
	claims, err := validarToken(token, s.secreto)
	if err != nil {
		common.WriteError(w, http.StatusUnauthorized, err.Error())
		return
	}
	for _, u := range s.usuarios.All() {
		if u.ID == claims.Sub {
			common.WriteJSON(w, http.StatusOK, u.publico())
			return
		}
	}
	common.WriteError(w, http.StatusUnauthorized, "el usuario del token ya no existe")
}

func (s *server) listar(w http.ResponseWriter, _ *http.Request) {
	todos := s.usuarios.All()
	lista := make([]UsuarioPublico, 0, len(todos))
	for _, u := range todos {
		lista = append(lista, u.publico())
	}
	common.WriteJSON(w, http.StatusOK, lista)
}

func (s *server) responderConToken(w http.ResponseWriter, status int, u Usuario) {
	token, err := crearToken(u, s.secreto, duracionToken)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "no se pudo generar el token")
		return
	}
	common.WriteJSON(w, status, authResponse{Token: token, Usuario: u.publico()})
}

func emailValido(email string) bool {
	addr, err := mail.ParseAddress(email)
	return err == nil && addr.Address == email && strings.Contains(email, ".")
}
