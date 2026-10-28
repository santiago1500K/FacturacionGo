# Sistema de Facturación Digital (microservicios en Go)

Proyecto 1 de Programación Avanzada 2026-2.

Cuatro microservicios en Go (solo librería estándar, `net/http`) más un frontend en
JavaScript con Bootstrap. La base de datos son archivos JSON dentro de cada servicio.

| Servicio      | Carpeta       | Puerto | Archivo de datos              |
|---------------|---------------|--------|-------------------------------|
| Autenticación | `auth/`       | 8081   | `auth/data/usuarios.json`     |
| Productos     | `productos/`  | 8082   | `productos/data/productos.json` |
| Clientes      | `clientes/`   | 8083   | `clientes/data/clientes.json` |
| Facturación   | `facturas/`   | 8084   | `facturas/data/facturas.json` |
| Frontend      | `frontend/`   | 8080   | `frontend/public/`            |

`common/` contiene código compartido (lectura/escritura de JSON, respuestas HTTP, CORS).

## Requisitos

- Go 1.24 o superior (`go version`)
- Un navegador (el frontend carga Bootstrap desde CDN, necesita internet)

## Cómo ejecutar

**Opción rápida (Windows):** doble clic en `iniciar.bat`. Abre una ventana por servicio y
luego el navegador en http://localhost:8080.

**Manual:** abrir 5 terminales y en cada una:

```bash
cd auth && go run .
```

```bash
cd productos && go run .
```

```bash
cd clientes && go run .
```

```bash
cd facturas && go run .
```

```bash
cd frontend && go run .
```

Luego abrir http://localhost:8080.

## Usuarios de prueba

| Email                 | Contraseña | Rol     |
|-----------------------|------------|---------|
| ricardo@example.com   | admin123   | admin   |
| maria@example.com     | maria123   | cliente |

También se puede crear una cuenta nueva desde la pestaña **Registrarse**.

Para generar el hash de una contraseña y escribirlo a mano en `usuarios.json`:

```bash
cd auth && go run . -hash miContraseña
```

## Configuración (variables de entorno opcionales)

| Variable        | Servicio  | Valor por defecto           |
|-----------------|-----------|-----------------------------|
| `PORT`          | todos     | 8081 / 8082 / 8083 / 8084 / 8080 |
| `DATA_FILE`     | auth, productos, clientes, facturas | `data/<nombre>.json` |
| `JWT_SECRET`    | auth      | `clave-secreta-desarrollo`  |
| `AUTH_URL`      | facturas  | `http://localhost:8081`     |
| `PRODUCTOS_URL` | facturas  | `http://localhost:8082`     |
| `CLIENTES_URL`  | facturas  | `http://localhost:8083`     |

## Documentación

El documento técnico (arquitectura, endpoints, flujos y ejemplo de factura) está en
[docs/documento-tecnico.md](docs/documento-tecnico.md). Ejemplos de factura generada:
[docs/ejemplo-factura.json](docs/ejemplo-factura.json) y [docs/ejemplo-factura.pdf](docs/ejemplo-factura.pdf).
