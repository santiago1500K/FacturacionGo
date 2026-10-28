package main

import (
	"bytes"
	"fmt"
	"math"
	"strconv"
	"strings"
)

// Generador mínimo de PDF (A4, fuentes Helvetica estándar) sin librerías
// externas. Suficiente para imprimir la factura.

const (
	anchoPagina = 595.0
	altoPagina  = 842.0
	margenIzq   = 50.0
	margenDer   = 545.0
	fuente      = "F1" // Helvetica
	negrita     = "F2" // Helvetica-Bold
)

type pdfPagina struct {
	contenido bytes.Buffer
}

func (p *pdfPagina) texto(x, y float64, f string, tam float64, s string) {
	fmt.Fprintf(&p.contenido, "BT /%s %.1f Tf %.2f %.2f Td (%s) Tj ET\n", f, tam, x, y, cadenaPDF(s))
}

func (p *pdfPagina) textoDerecha(xDer, y float64, f string, tam float64, s string) {
	p.texto(xDer-anchoTexto(s, tam), y, f, tam, s)
}

func (p *pdfPagina) gris(g float64) {
	fmt.Fprintf(&p.contenido, "%.2f g\n", g)
}

func (p *pdfPagina) linea(x1, y1, x2, y2, gris float64) {
	fmt.Fprintf(&p.contenido, "%.2f G 0.6 w %.2f %.2f m %.2f %.2f l S 0 G\n", gris, x1, y1, x2, y2)
}

func (p *pdfPagina) rectangulo(x, y, w, h, gris float64) {
	fmt.Fprintf(&p.contenido, "%.2f g %.2f %.2f %.2f %.2f re f 0 g\n", gris, x, y, w, h)
}

type pdfDocumento struct {
	paginas []*pdfPagina
}

func (d *pdfDocumento) nuevaPagina() *pdfPagina {
	p := &pdfPagina{}
	d.paginas = append(d.paginas, p)
	return p
}

// bytes arma el archivo: catálogo, árbol de páginas, fuentes, una página y un
// flujo de contenido por hoja, tabla xref y trailer.
func (d *pdfDocumento) bytes() []byte {
	var buf bytes.Buffer
	var offsets []int
	objeto := func(cuerpo string) {
		offsets = append(offsets, buf.Len())
		fmt.Fprintf(&buf, "%d 0 obj\n%s\nendobj\n", len(offsets), cuerpo)
	}

	buf.WriteString("%PDF-1.4\n%\xe2\xe3\xcf\xd3\n")

	// Objetos 1-4 fijos; luego, por cada página i: página (5+2i) y contenido (6+2i).
	kids := make([]string, len(d.paginas))
	for i := range d.paginas {
		kids[i] = fmt.Sprintf("%d 0 R", 5+2*i)
	}
	objeto("<< /Type /Catalog /Pages 2 0 R >>")
	objeto(fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", strings.Join(kids, " "), len(d.paginas)))
	objeto("<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding >>")
	objeto("<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica-Bold /Encoding /WinAnsiEncoding >>")
	for i, p := range d.paginas {
		objeto(fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 %.0f %.0f] "+
			"/Resources << /Font << /F1 3 0 R /F2 4 0 R >> >> /Contents %d 0 R >>", anchoPagina, altoPagina, 6+2*i))
		c := p.contenido.Bytes()
		objeto(fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(c), c))
	}

	xref := buf.Len()
	fmt.Fprintf(&buf, "xref\n0 %d\n0000000000 65535 f \n", len(offsets)+1)
	for _, o := range offsets {
		fmt.Fprintf(&buf, "%010d 00000 n \n", o)
	}
	fmt.Fprintf(&buf, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(offsets)+1, xref)
	return buf.Bytes()
}

// cadenaPDF convierte a WinAnsi (tildes y ñ incluidas) y escapa \ ( ).
func cadenaPDF(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == '\\' || r == '(' || r == ')':
			b.WriteByte('\\')
			b.WriteByte(byte(r))
		case r >= 32 && r < 127, r >= 0xA0 && r <= 0xFF:
			b.WriteByte(byte(r))
		case r == '–' || r == '—':
			b.WriteByte('-')
		default:
			b.WriteByte('?')
		}
	}
	return b.String()
}

// anchoTexto aproxima el ancho en Helvetica; es exacto para cifras, puntos,
// comas, espacios y "$", que es lo que se alinea a la derecha.
func anchoTexto(s string, tam float64) float64 {
	unidades := 0
	for _, r := range s {
		switch {
		case r == ' ' || r == '.' || r == ',' || r == ':':
			unidades += 278
		case r == 'i' || r == 'l' || r == 'I':
			unidades += 250
		case r >= 'A' && r <= 'Z':
			unidades += 722
		default:
			unidades += 556
		}
	}
	return float64(unidades) / 1000 * tam
}

func formatoMoneda(v float64) string {
	s := strconv.FormatFloat(math.Abs(v), 'f', 2, 64)
	entero, dec := s[:len(s)-3], s[len(s)-2:]
	var b strings.Builder
	for i, c := range entero {
		if i > 0 && (len(entero)-i)%3 == 0 {
			b.WriteByte('.')
		}
		b.WriteRune(c)
	}
	signo := ""
	if v < 0 {
		signo = "-"
	}
	return signo + "$ " + b.String() + "," + dec
}

func recortar(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max-3]) + "..."
}

// Columnas de la tabla de productos.
const (
	colCodigo   = margenIzq + 6
	colProducto = margenIzq + 60
	colCant     = 355.0 // borde derecho
	colPrecio   = 450.0 // borde derecho
	colSubtotal = margenDer - 6
)

func encabezadoTabla(p *pdfPagina, y float64) {
	p.rectangulo(margenIzq, y-6, margenDer-margenIzq, 20, 0.88)
	p.texto(colCodigo, y, negrita, 9.5, "Código")
	p.texto(colProducto, y, negrita, 9.5, "Producto")
	p.textoDerecha(colCant, y, negrita, 9.5, "Cant.")
	p.textoDerecha(colPrecio, y, negrita, 9.5, "Precio unit.")
	p.textoDerecha(colSubtotal, y, negrita, 9.5, "Subtotal")
}

func piePagina(p *pdfPagina, f Factura, num int) {
	p.linea(margenIzq, 55, margenDer, 55, 0.75)
	p.gris(0.4)
	p.texto(margenIzq, 40, fuente, 8, "Documento generado por el Sistema de Facturación Digital (simulación académica).")
	p.textoDerecha(margenDer, 40, fuente, 8, fmt.Sprintf("Factura %d - pág. %d", f.FacturaID, num))
	p.gris(0)
}

func generarPDF(f Factura) []byte {
	doc := &pdfDocumento{}
	p := doc.nuevaPagina()

	// Encabezado
	y := 790.0
	p.texto(margenIzq, y, negrita, 22, "FACTURA DE VENTA")
	p.textoDerecha(margenDer, y, negrita, 16, fmt.Sprintf("No. %d", f.FacturaID))
	y -= 20
	p.gris(0.4)
	p.texto(margenIzq, y, fuente, 10, "Sistema de Facturación Digital")
	p.textoDerecha(margenDer, y, fuente, 10, "Fecha: "+f.Fecha)
	p.gris(0)
	y -= 14
	p.linea(margenIzq, y, margenDer, y, 0.2)

	// Datos del cliente y del usuario que factura
	y -= 26
	col2 := 330.0
	p.texto(margenIzq, y, negrita, 10, "CLIENTE")
	p.texto(col2, y, negrita, 10, "ATENDIDO POR")
	y -= 16
	cliente := []string{f.Cliente.Nombre, "C.C. " + f.Cliente.CC, f.Cliente.Email, recortar(f.Cliente.Direccion, 50)}
	usuario := []string{f.Usuario.Nombre, "Rol: " + f.Usuario.Rol, fmt.Sprintf("ID usuario: %d", f.Usuario.ID)}
	for i := 0; i < len(cliente) || i < len(usuario); i++ {
		if i < len(cliente) {
			p.texto(margenIzq, y, fuente, 10, cliente[i])
		}
		if i < len(usuario) {
			p.texto(col2, y, fuente, 10, usuario[i])
		}
		y -= 14
	}

	// Tabla de productos
	y -= 20
	encabezadoTabla(p, y)
	y -= 22
	for _, it := range f.Productos {
		if y < 110 {
			piePagina(p, f, len(doc.paginas))
			p = doc.nuevaPagina()
			y = 790
			encabezadoTabla(p, y)
			y -= 22
		}
		p.texto(colCodigo, y, fuente, 9.5, strconv.Itoa(it.ID))
		p.texto(colProducto, y, fuente, 9.5, recortar(it.Nombre, 38))
		p.textoDerecha(colCant, y, fuente, 9.5, strconv.Itoa(it.Cantidad))
		p.textoDerecha(colPrecio, y, fuente, 9.5, formatoMoneda(it.PrecioUnitario))
		p.textoDerecha(colSubtotal, y, fuente, 9.5, formatoMoneda(it.Subtotal))
		p.linea(margenIzq, y-6, margenDer, y-6, 0.85)
		y -= 20
	}

	// Total
	if y < 110 {
		piePagina(p, f, len(doc.paginas))
		p = doc.nuevaPagina()
		y = 790
	}
	y -= 10
	p.rectangulo(330, y-8, margenDer-330, 24, 0.92)
	p.texto(340, y, negrita, 12, "TOTAL A PAGAR")
	p.textoDerecha(colSubtotal, y, negrita, 12, formatoMoneda(f.Total))

	piePagina(p, f, len(doc.paginas))
	return doc.bytes()
}
