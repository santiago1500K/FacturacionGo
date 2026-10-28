// Servicio de Facturación: consume los servicios de autenticación, clientes y
// productos para armar facturas, las guarda en data/facturas.json y las
// entrega en JSON o PDF.
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math"
	"net/http"
	"slices"
	"strconv"
	"time"

	"facturacion/common"
)

type ClienteFactura struct {
	ID        int    `json:"id"`
	Nombre    string `json:"nombre"`
	CC        string `json:"cc"`
	Email     string `json:"email"`
	Direccion string `json:"direccion"`
}

type UsuarioFactura struct {
	ID     int    `json:"id"`
	Nombre string `json:"nombre"`
	Rol    string `json:"rol"`
}

type ItemFactura struct {
	ID             int     `json:"id"`
	Nombre         string  `json:"nombre"`
	Cantidad       int     `json:"cantidad"`
	PrecioUnitario float64 `json:"precio_unitario"`
	Subtotal       float64 `json:"subtotal"`
}

// Factura tal como se guarda en data/facturas.json.
type Factura struct {
	FacturaID int            `json:"factura_id"`
	Cliente   ClienteFactura `json:"cliente"`
	Usuario   UsuarioFactura `json:"usuario"`
	Productos []ItemFactura  `json:"productos"`
	Total     float64        `json:"total"`
	Fecha     string         `json:"fecha"`
}

type crearFacturaRequest struct {
	ClienteID int `json:"cliente_id"`
	Productos []struct {
		ID       int `json:"id"`
		Cantidad int `json:"cantidad"`
	} `json:"productos"`
}

const primerID = 5001

type server struct {
	facturas *common.JSONStore[Factura]
	api      *servicios
}

func main() {
	facturas, err := common.NewJSONStore[Factura](common.Env("DATA_FILE", "data/facturas.json"))
	if err != nil {
		log.Fatal(err)
	}
	s := &server{facturas: facturas, api: nuevosServicios()}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/facturas", s.autenticado(s.crear))
	mux.HandleFunc("GET /api/facturas", s.autenticado(s.listar))
	mux.HandleFunc("GET /api/facturas/{id}", s.autenticado(s.obtener))
	mux.HandleFunc("GET /api/facturas/{id}/pdf", s.autenticado(s.pdf))
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		common.WriteJSON(w, http.StatusOK, map[string]string{"estado": "ok", "servicio": "facturas"})
	})

	common.Serve("facturación", common.Env("PORT", "8084"), mux)
}

type handlerAutenticado func(w http.ResponseWriter, r *http.Request, u UsuarioFactura)

// autenticado exige un token válido, que se verifica con el servicio de auth.
func (s *server) autenticado(next handlerAutenticado) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := common.BearerToken(r)
		if token == "" {
			common.WriteError(w, http.StatusUnauthorized, "debe iniciar sesión (falta el token)")
			return
		}
		u, err := s.api.usuarioActual(r.Context(), token)
		if err != nil {
			responderErrorRemoto(w, err, "autenticación")
			return
		}
		next(w, r, u)
	}
}

func (s *server) crear(w http.ResponseWriter, r *http.Request, usuario UsuarioFactura) {
	var req crearFacturaRequest
	if err := common.DecodeJSON(r, &req); err != nil {
		common.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.ClienteID <= 0 {
		common.WriteError(w, http.StatusBadRequest, "debe indicar cliente_id")
		return
	}
	if len(req.Productos) == 0 {
		common.WriteError(w, http.StatusBadRequest, "la factura debe tener al menos un producto")
		return
	}

	// Agrupa productos repetidos conservando el orden en que se enviaron.
	var orden []int
	cantidades := map[int]int{}
	for _, p := range req.Productos {
		if p.Cantidad <= 0 {
			common.WriteError(w, http.StatusBadRequest, fmt.Sprintf("la cantidad del producto %d debe ser mayor que cero", p.ID))
			return
		}
		if _, ok := cantidades[p.ID]; !ok {
			orden = append(orden, p.ID)
		}
		cantidades[p.ID] += p.Cantidad
	}

	ctx := r.Context()

	// 1. Datos del cliente (servicio de clientes).
	cliente, err := s.api.cliente(ctx, req.ClienteID)
	if err != nil {
		responderErrorRemoto(w, err, "clientes")
		return
	}

	// 2. Datos y stock de cada producto (servicio de productos).
	items := make([]ItemFactura, 0, len(orden))
	total := 0.0
	for _, id := range orden {
		p, err := s.api.producto(ctx, id)
		if err != nil {
			responderErrorRemoto(w, err, "productos")
			return
		}
		cant := cantidades[id]
		if p.Stock < cant {
			common.WriteError(w, http.StatusConflict,
				fmt.Sprintf("stock insuficiente para %q: disponible %d, solicitado %d", p.Nombre, p.Stock, cant))
			return
		}
		subtotal := redondear(p.Precio * float64(cant))
		items = append(items, ItemFactura{ID: p.ID, Nombre: p.Nombre, Cantidad: cant, PrecioUnitario: p.Precio, Subtotal: subtotal})
		total += subtotal
	}

	// 3. Descuenta el stock; si algo falla, devuelve lo ya descontado.
	var descontados []ItemFactura
	for _, it := range items {
		if err := s.api.ajustarStock(ctx, it.ID, -it.Cantidad); err != nil {
			s.revertirStock(descontados)
			responderErrorRemoto(w, err, "productos")
			return
		}
		descontados = append(descontados, it)
	}

	// 4. Guarda la factura con el siguiente consecutivo.
	factura := Factura{
		Cliente:   cliente,
		Usuario:   usuario,
		Productos: items,
		Total:     redondear(total),
		Fecha:     time.Now().Format("2006-01-02"),
	}
	err = s.facturas.Update(func(lista []Factura) ([]Factura, error) {
		maxID := primerID - 1
		for _, f := range lista {
			maxID = max(maxID, f.FacturaID)
		}
		factura.FacturaID = maxID + 1
		return append(lista, factura), nil
	})
	if err != nil {
		s.revertirStock(descontados)
		common.WriteError(w, http.StatusInternalServerError, "no se pudo guardar la factura: "+err.Error())
		return
	}

	log.Printf("Factura %d creada por %s para %s: total %.2f", factura.FacturaID, usuario.Nombre, cliente.Nombre, factura.Total)
	common.WriteJSON(w, http.StatusCreated, factura)
}

// listar devuelve las facturas más recientes primero; admite ?cliente_id=.
func (s *server) listar(w http.ResponseWriter, r *http.Request, _ UsuarioFactura) {
	filtro, _ := strconv.Atoi(r.URL.Query().Get("cliente_id"))
	lista := []Factura{}
	for _, f := range s.facturas.All() {
		if filtro == 0 || f.Cliente.ID == filtro {
			lista = append(lista, f)
		}
	}
	slices.SortFunc(lista, func(a, b Factura) int { return b.FacturaID - a.FacturaID })
	common.WriteJSON(w, http.StatusOK, lista)
}

func (s *server) obtener(w http.ResponseWriter, r *http.Request, _ UsuarioFactura) {
	f, ok := s.buscar(w, r)
	if ok {
		common.WriteJSON(w, http.StatusOK, f)
	}
}

func (s *server) pdf(w http.ResponseWriter, r *http.Request, _ UsuarioFactura) {
	f, ok := s.buscar(w, r)
	if !ok {
		return
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="factura-%d.pdf"`, f.FacturaID))
	_, _ = w.Write(generarPDF(f))
}

func (s *server) buscar(w http.ResponseWriter, r *http.Request) (Factura, bool) {
	id, err := common.PathID(r, "id")
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, err.Error())
		return Factura{}, false
	}
	for _, f := range s.facturas.All() {
		if f.FacturaID == id {
			return f, true
		}
	}
	common.WriteError(w, http.StatusNotFound, "factura no encontrada")
	return Factura{}, false
}

func (s *server) revertirStock(items []ItemFactura) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for _, it := range items {
		if err := s.api.ajustarStock(ctx, it.ID, it.Cantidad); err != nil {
			log.Printf("ERROR: no se pudo devolver el stock del producto %d (%d uds): %v", it.ID, it.Cantidad, err)
		}
	}
}

// responderErrorRemoto traduce el error de otro servicio a una respuesta.
func responderErrorRemoto(w http.ResponseWriter, err error, servicio string) {
	var e *apiError
	if !errors.As(err, &e) {
		common.WriteError(w, http.StatusBadGateway, "error comunicándose con el servicio de "+servicio+": "+err.Error())
		return
	}
	switch {
	case e.Status == http.StatusBadGateway:
		common.WriteError(w, http.StatusBadGateway, "el servicio de "+servicio+" no está disponible")
	case e.Status == http.StatusNotFound:
		// Un cliente o producto inexistente es un error en los datos enviados.
		common.WriteError(w, http.StatusBadRequest, e.Mensaje)
	default:
		common.WriteError(w, e.Status, e.Mensaje)
	}
}

func redondear(v float64) float64 {
	return math.Round(v*100) / 100
}
