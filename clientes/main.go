// Servicio de Clientes: CRUD de clientes (nombre, cédula, correo, dirección).
package main

import (
	"errors"
	"log"
	"net/http"
	"net/mail"
	"strings"
	"unicode"

	"facturacion/common"
)

// Cliente tal como se guarda en data/clientes.json.
type Cliente struct {
	ID        int    `json:"id"`
	Nombre    string `json:"nombre"`
	CC        string `json:"cc"`
	Email     string `json:"email"`
	Direccion string `json:"direccion"`
}

var (
	errNoEncontrado = errors.New("cliente no encontrado")
	errCCDuplicada  = errors.New("ya existe un cliente con esa cédula")
)

const primerID = 201

type server struct {
	clientes *common.JSONStore[Cliente]
}

func main() {
	clientes, err := common.NewJSONStore[Cliente](common.Env("DATA_FILE", "data/clientes.json"))
	if err != nil {
		log.Fatal(err)
	}
	s := &server{clientes: clientes}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/clientes", s.listar)
	mux.HandleFunc("GET /api/clientes/{id}", s.obtener)
	mux.HandleFunc("POST /api/clientes", s.crear)
	mux.HandleFunc("PUT /api/clientes/{id}", s.actualizar)
	mux.HandleFunc("DELETE /api/clientes/{id}", s.eliminar)
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		common.WriteJSON(w, http.StatusOK, map[string]string{"estado": "ok", "servicio": "clientes"})
	})

	common.Serve("clientes", common.Env("PORT", "8083"), mux)
}

// listar admite ?q=texto para buscar por nombre, cédula o email.
func (s *server) listar(w http.ResponseWriter, r *http.Request) {
	q := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
	lista := []Cliente{}
	for _, c := range s.clientes.All() {
		texto := strings.ToLower(c.Nombre + " " + c.CC + " " + c.Email)
		if q == "" || strings.Contains(texto, q) {
			lista = append(lista, c)
		}
	}
	common.WriteJSON(w, http.StatusOK, lista)
}

func (s *server) obtener(w http.ResponseWriter, r *http.Request) {
	id, err := common.PathID(r, "id")
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	for _, c := range s.clientes.All() {
		if c.ID == id {
			common.WriteJSON(w, http.StatusOK, c)
			return
		}
	}
	common.WriteError(w, http.StatusNotFound, errNoEncontrado.Error())
}

func (s *server) crear(w http.ResponseWriter, r *http.Request) {
	var c Cliente
	if err := common.DecodeJSON(r, &c); err != nil {
		common.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if msg := validar(&c); msg != "" {
		common.WriteError(w, http.StatusBadRequest, msg)
		return
	}
	err := s.clientes.Update(func(items []Cliente) ([]Cliente, error) {
		maxID := primerID - 1
		for _, x := range items {
			if x.CC == c.CC {
				return nil, errCCDuplicada
			}
			maxID = max(maxID, x.ID)
		}
		c.ID = maxID + 1
		return append(items, c), nil
	})
	if err != nil {
		responderError(w, err)
		return
	}
	common.WriteJSON(w, http.StatusCreated, c)
}

func (s *server) actualizar(w http.ResponseWriter, r *http.Request) {
	id, err := common.PathID(r, "id")
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	var c Cliente
	if err := common.DecodeJSON(r, &c); err != nil {
		common.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if msg := validar(&c); msg != "" {
		common.WriteError(w, http.StatusBadRequest, msg)
		return
	}
	c.ID = id
	err = s.clientes.Update(func(items []Cliente) ([]Cliente, error) {
		pos := -1
		for i, x := range items {
			if x.ID == id {
				pos = i
			} else if x.CC == c.CC {
				return nil, errCCDuplicada
			}
		}
		if pos < 0 {
			return nil, errNoEncontrado
		}
		items[pos] = c
		return items, nil
	})
	if err != nil {
		responderError(w, err)
		return
	}
	common.WriteJSON(w, http.StatusOK, c)
}

func (s *server) eliminar(w http.ResponseWriter, r *http.Request) {
	id, err := common.PathID(r, "id")
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	err = s.clientes.Update(func(items []Cliente) ([]Cliente, error) {
		for i, c := range items {
			if c.ID == id {
				return append(items[:i], items[i+1:]...), nil
			}
		}
		return nil, errNoEncontrado
	})
	if err != nil {
		responderError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func responderError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, errNoEncontrado):
		common.WriteError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, errCCDuplicada):
		common.WriteError(w, http.StatusConflict, err.Error())
	default:
		common.WriteError(w, http.StatusInternalServerError, err.Error())
	}
}

func validar(c *Cliente) string {
	c.Nombre = strings.TrimSpace(c.Nombre)
	c.CC = strings.TrimSpace(c.CC)
	c.Email = strings.ToLower(strings.TrimSpace(c.Email))
	c.Direccion = strings.TrimSpace(c.Direccion)
	switch {
	case c.Nombre == "":
		return "el nombre es obligatorio"
	case c.CC == "" || strings.IndexFunc(c.CC, func(r rune) bool { return !unicode.IsDigit(r) }) >= 0:
		return "la cédula es obligatoria y solo debe contener números"
	case !emailValido(c.Email):
		return "el email no es válido"
	case c.Direccion == "":
		return "la dirección es obligatoria"
	}
	return ""
}

func emailValido(email string) bool {
	addr, err := mail.ParseAddress(email)
	return err == nil && addr.Address == email && strings.Contains(email, ".")
}
