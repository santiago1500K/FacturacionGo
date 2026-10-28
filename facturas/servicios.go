package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"facturacion/common"
)

// apiError representa una respuesta de error de otro microservicio
// (o la imposibilidad de contactarlo, con Status 502).
type apiError struct {
	Status  int
	Mensaje string
}

func (e *apiError) Error() string { return e.Mensaje }

type productoRemoto struct {
	ID     int     `json:"id"`
	Nombre string  `json:"nombre"`
	Precio float64 `json:"precio"`
	Stock  int     `json:"stock"`
}

// servicios agrupa las llamadas REST a los otros tres microservicios.
type servicios struct {
	authURL      string
	productosURL string
	clientesURL  string
	http         *http.Client
}

func nuevosServicios() *servicios {
	return &servicios{
		authURL:      common.Env("AUTH_URL", "http://localhost:8081"),
		productosURL: common.Env("PRODUCTOS_URL", "http://localhost:8082"),
		clientesURL:  common.Env("CLIENTES_URL", "http://localhost:8083"),
		http:         &http.Client{Timeout: 5 * time.Second},
	}
}

// usuarioActual valida el token con el servicio de autenticación.
func (s *servicios) usuarioActual(ctx context.Context, token string) (UsuarioFactura, error) {
	var u UsuarioFactura
	err := s.llamar(ctx, http.MethodGet, s.authURL+"/api/auth/me", token, nil, &u)
	return u, err
}

func (s *servicios) cliente(ctx context.Context, id int) (ClienteFactura, error) {
	var c ClienteFactura
	err := s.llamar(ctx, http.MethodGet, fmt.Sprintf("%s/api/clientes/%d", s.clientesURL, id), "", nil, &c)
	return c, err
}

func (s *servicios) producto(ctx context.Context, id int) (productoRemoto, error) {
	var p productoRemoto
	err := s.llamar(ctx, http.MethodGet, fmt.Sprintf("%s/api/productos/%d", s.productosURL, id), "", nil, &p)
	return p, err
}

// ajustarStock suma cantidad al stock del producto (negativa para descontar).
func (s *servicios) ajustarStock(ctx context.Context, id, cantidad int) error {
	url := fmt.Sprintf("%s/api/productos/%d/stock", s.productosURL, id)
	return s.llamar(ctx, http.MethodPatch, url, "", map[string]int{"cantidad": cantidad}, nil)
}

func (s *servicios) llamar(ctx context.Context, metodo, url, token string, body, out any) error {
	var lector io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		lector = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, metodo, url, lector)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := s.http.Do(req)
	if err != nil {
		return &apiError{Status: http.StatusBadGateway, Mensaje: "no se pudo conectar con " + url}
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		var e common.ErrorResponse
		_ = json.NewDecoder(resp.Body).Decode(&e)
		if e.Error == "" {
			e.Error = resp.Status
		}
		return &apiError{Status: resp.StatusCode, Mensaje: e.Error}
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}
