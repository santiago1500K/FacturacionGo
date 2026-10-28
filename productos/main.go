// Servicio de Productos: CRUD de productos y gestión de precios y stock.
package main

import (
	"errors"
	"log"
	"math"
	"net/http"
	"strings"

	"facturacion/common"
)

// Producto tal como se guarda en data/productos.json.
type Producto struct {
	ID     int     `json:"id"`
	Nombre string  `json:"nombre"`
	Precio float64 `json:"precio"`
	Stock  int     `json:"stock"`
}

type precioRequest struct {
	Precio float64 `json:"precio"`
}

// stockRequest ajusta el stock sumando "cantidad" (negativa para descontar).
type stockRequest struct {
	Cantidad int `json:"cantidad"`
}

var (
	errNoEncontrado     = errors.New("producto no encontrado")
	errStockInsuficiente = errors.New("stock insuficiente")
)

const primerID = 101

type server struct {
	productos *common.JSONStore[Producto]
}

func main() {
	productos, err := common.NewJSONStore[Producto](common.Env("DATA_FILE", "data/productos.json"))
	if err != nil {
		log.Fatal(err)
	}
	s := &server{productos: productos}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/productos", s.listar)
	mux.HandleFunc("GET /api/productos/{id}", s.obtener)
	mux.HandleFunc("POST /api/productos", s.crear)
	mux.HandleFunc("PUT /api/productos/{id}", s.actualizar)
	mux.HandleFunc("DELETE /api/productos/{id}", s.eliminar)
	mux.HandleFunc("PATCH /api/productos/{id}/precio", s.cambiarPrecio)
	mux.HandleFunc("PATCH /api/productos/{id}/stock", s.ajustarStock)
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		common.WriteJSON(w, http.StatusOK, map[string]string{"estado": "ok", "servicio": "productos"})
	})

	common.Serve("productos", common.Env("PORT", "8082"), mux)
}

// listar admite ?q=texto para filtrar por nombre.
func (s *server) listar(w http.ResponseWriter, r *http.Request) {
	q := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
	lista := []Producto{}
	for _, p := range s.productos.All() {
		if q == "" || strings.Contains(strings.ToLower(p.Nombre), q) {
			lista = append(lista, p)
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
	for _, p := range s.productos.All() {
		if p.ID == id {
			common.WriteJSON(w, http.StatusOK, p)
			return
		}
	}
	common.WriteError(w, http.StatusNotFound, errNoEncontrado.Error())
}

func (s *server) crear(w http.ResponseWriter, r *http.Request) {
	var p Producto
	if err := common.DecodeJSON(r, &p); err != nil {
		common.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if msg := validar(&p); msg != "" {
		common.WriteError(w, http.StatusBadRequest, msg)
		return
	}
	err := s.productos.Update(func(items []Producto) ([]Producto, error) {
		maxID := primerID - 1
		for _, x := range items {
			maxID = max(maxID, x.ID)
		}
		p.ID = maxID + 1
		return append(items, p), nil
	})
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusCreated, p)
}

func (s *server) actualizar(w http.ResponseWriter, r *http.Request) {
	id, err := common.PathID(r, "id")
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	var p Producto
	if err := common.DecodeJSON(r, &p); err != nil {
		common.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if msg := validar(&p); msg != "" {
		common.WriteError(w, http.StatusBadRequest, msg)
		return
	}
	p.ID = id
	s.modificar(w, id, func(actual *Producto) error {
		*actual = p
		return nil
	})
}

func (s *server) cambiarPrecio(w http.ResponseWriter, r *http.Request) {
	id, err := common.PathID(r, "id")
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	var req precioRequest
	if err := common.DecodeJSON(r, &req); err != nil {
		common.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Precio <= 0 {
		common.WriteError(w, http.StatusBadRequest, "el precio debe ser mayor que cero")
		return
	}
	s.modificar(w, id, func(actual *Producto) error {
		actual.Precio = redondear(req.Precio)
		return nil
	})
}

// ajustarStock suma la cantidad indicada al stock; lo usa el servicio de
// facturación para descontar unidades vendidas (cantidad negativa).
func (s *server) ajustarStock(w http.ResponseWriter, r *http.Request) {
	id, err := common.PathID(r, "id")
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	var req stockRequest
	if err := common.DecodeJSON(r, &req); err != nil {
		common.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.modificar(w, id, func(actual *Producto) error {
		if actual.Stock+req.Cantidad < 0 {
			return errStockInsuficiente
		}
		actual.Stock += req.Cantidad
		return nil
	})
}

func (s *server) eliminar(w http.ResponseWriter, r *http.Request) {
	id, err := common.PathID(r, "id")
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	err = s.productos.Update(func(items []Producto) ([]Producto, error) {
		for i, p := range items {
			if p.ID == id {
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

// modificar busca el producto, aplica fn y responde con el producto resultante.
func (s *server) modificar(w http.ResponseWriter, id int, fn func(*Producto) error) {
	var resultado Producto
	err := s.productos.Update(func(items []Producto) ([]Producto, error) {
		for i := range items {
			if items[i].ID == id {
				if err := fn(&items[i]); err != nil {
					return nil, err
				}
				resultado = items[i]
				return items, nil
			}
		}
		return nil, errNoEncontrado
	})
	if err != nil {
		responderError(w, err)
		return
	}
	common.WriteJSON(w, http.StatusOK, resultado)
}

func responderError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, errNoEncontrado):
		common.WriteError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, errStockInsuficiente):
		common.WriteError(w, http.StatusConflict, err.Error())
	default:
		common.WriteError(w, http.StatusInternalServerError, err.Error())
	}
}

func validar(p *Producto) string {
	p.Nombre = strings.TrimSpace(p.Nombre)
	p.Precio = redondear(p.Precio)
	switch {
	case p.Nombre == "":
		return "el nombre es obligatorio"
	case p.Precio <= 0:
		return "el precio debe ser mayor que cero"
	case p.Stock < 0:
		return "el stock no puede ser negativo"
	}
	return ""
}

func redondear(v float64) float64 {
	return math.Round(v*100) / 100
}
