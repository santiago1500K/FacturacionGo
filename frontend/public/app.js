// Frontend del sistema de facturación. Consume los 4 microservicios por REST.

const API = {
  auth: 'http://localhost:8081/api/auth',
  productos: 'http://localhost:8082/api/productos',
  clientes: 'http://localhost:8083/api/clientes',
  facturas: 'http://localhost:8084/api/facturas',
};

const estado = {
  token: null,
  usuario: null,
  productos: [],
  clientes: [],
  carrito: [], // [{ id, cantidad }]
  facturaActual: null,
};

const $ = (sel) => document.querySelector(sel);

// ---------------------------------------------------------------- utilidades

const formatoCOP = new Intl.NumberFormat('es-CO', { style: 'currency', currency: 'COP', minimumFractionDigits: 2 });
const moneda = (v) => formatoCOP.format(v);

function esc(texto) {
  return String(texto ?? '').replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));
}

function toast(mensaje, tipo = 'success') {
  const el = document.createElement('div');
  el.className = `toast align-items-center text-bg-${tipo} border-0`;
  el.innerHTML = `<div class="d-flex"><div class="toast-body">${esc(mensaje)}</div>
    <button type="button" class="btn-close btn-close-white me-2 m-auto" data-bs-dismiss="toast"></button></div>`;
  $('#toasts').appendChild(el);
  el.addEventListener('hidden.bs.toast', () => el.remove());
  new bootstrap.Toast(el, { delay: tipo === 'danger' ? 6000 : 3000 }).show();
}

function descargar(blob, nombre) {
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = nombre;
  a.click();
  setTimeout(() => URL.revokeObjectURL(url), 1000);
}

/**
 * Llama a un microservicio. Lanza Error con el mensaje que devuelve el
 * servicio ({"error": "..."}) o uno legible si el servicio no responde.
 */
async function api(url, { method = 'GET', body, blob = false } = {}) {
  const headers = {};
  if (body !== undefined) headers['Content-Type'] = 'application/json';
  if (estado.token) headers.Authorization = `Bearer ${estado.token}`;

  let res;
  try {
    res = await fetch(url, { method, headers, body: body !== undefined ? JSON.stringify(body) : undefined });
  } catch {
    const servicio = Object.keys(API).find((k) => url.startsWith(API[k])) ?? 'remoto';
    throw new Error(`No se pudo conectar con el servicio de ${servicio}. ¿Está en ejecución?`);
  }

  if (!res.ok) {
    let mensaje = `Error ${res.status}`;
    try { mensaje = (await res.json()).error || mensaje; } catch { /* sin cuerpo JSON */ }
    if (res.status === 401 && estado.token) {
      cerrarSesion();
      mensaje = 'Su sesión expiró: ' + mensaje;
    }
    throw new Error(mensaje);
  }
  if (res.status === 204) return null;
  return blob ? res.blob() : res.json();
}

// ---------------------------------------------------------------- sesión

function guardarSesion(token, usuario) {
  estado.token = token;
  estado.usuario = usuario;
  try { localStorage.setItem('sesion', JSON.stringify({ token, usuario })); } catch { /* almacenamiento no disponible */ }
}

function cerrarSesion() {
  estado.token = null;
  estado.usuario = null;
  estado.carrito = [];
  try { localStorage.removeItem('sesion'); } catch { /* almacenamiento no disponible */ }
  $('#vista-app').classList.add('d-none');
  $('#vista-login').classList.remove('d-none');
}

async function entrar() {
  $('#vista-login').classList.add('d-none');
  $('#vista-app').classList.remove('d-none');
  $('#usuario-nombre').textContent = estado.usuario.nombre;
  $('#usuario-rol').textContent = estado.usuario.rol;
  mostrarSeccion('productos');
  await Promise.allSettled([cargarProductos(), cargarClientes()]);
}

$('#form-login').addEventListener('submit', async (e) => {
  e.preventDefault();
  try {
    const r = await api(`${API.auth}/login`, {
      method: 'POST',
      body: { email: $('#login-email').value, password: $('#login-password').value },
    });
    guardarSesion(r.token, r.usuario);
    $('#form-login').reset();
    toast(`Bienvenido, ${r.usuario.nombre}`);
    entrar();
  } catch (err) {
    toast(err.message, 'danger');
  }
});

$('#form-registro').addEventListener('submit', async (e) => {
  e.preventDefault();
  try {
    const r = await api(`${API.auth}/register`, {
      method: 'POST',
      body: {
        nombre: $('#reg-nombre').value,
        email: $('#reg-email').value,
        password: $('#reg-password').value,
        rol: $('#reg-rol').value,
      },
    });
    guardarSesion(r.token, r.usuario);
    $('#form-registro').reset();
    toast('Cuenta creada correctamente');
    entrar();
  } catch (err) {
    toast(err.message, 'danger');
  }
});

$('#btn-salir').addEventListener('click', cerrarSesion);

// ---------------------------------------------------------------- navegación

function mostrarSeccion(nombre) {
  document.querySelectorAll('.seccion').forEach((s) => s.classList.toggle('d-none', s.id !== `sec-${nombre}`));
  document.querySelectorAll('[data-seccion]').forEach((a) => a.classList.toggle('active', a.dataset.seccion === nombre));
  if (nombre === 'nueva') renderNuevaFactura();
  if (nombre === 'facturas') cargarFacturas();
  bootstrap.Collapse.getInstance($('#menu'))?.hide();
}

document.querySelectorAll('[data-seccion]').forEach((a) =>
  a.addEventListener('click', (e) => {
    e.preventDefault();
    mostrarSeccion(a.dataset.seccion);
  }),
);

// ---------------------------------------------------------------- productos

const modalProducto = new bootstrap.Modal('#modal-producto');

async function cargarProductos() {
  try {
    estado.productos = await api(API.productos);
  } catch (err) {
    estado.productos = [];
    toast(err.message, 'danger');
  }
  renderProductos();
  renderNuevaFactura();
}

function renderProductos() {
  const q = $('#buscar-producto').value.trim().toLowerCase();
  const lista = estado.productos.filter((p) => p.nombre.toLowerCase().includes(q));
  $('#tabla-productos').innerHTML = lista.length
    ? lista.map((p) => `
      <tr>
        <td>${p.id}</td>
        <td>${esc(p.nombre)}</td>
        <td class="text-end">${moneda(p.precio)}</td>
        <td class="text-end"><span class="badge ${p.stock === 0 ? 'bg-danger' : p.stock < 5 ? 'bg-warning text-dark' : 'bg-success'}">${p.stock}</span></td>
        <td class="text-end text-nowrap">
          <button class="btn btn-sm btn-outline-success" data-accion="carrito" data-id="${p.id}" title="Agregar a la factura" ${p.stock === 0 ? 'disabled' : ''}><i class="bi bi-cart-plus"></i></button>
          <button class="btn btn-sm btn-outline-primary" data-accion="editar" data-id="${p.id}" title="Editar"><i class="bi bi-pencil"></i></button>
          <button class="btn btn-sm btn-outline-danger" data-accion="eliminar" data-id="${p.id}" title="Eliminar"><i class="bi bi-trash"></i></button>
        </td>
      </tr>`).join('')
    : '<tr><td colspan="5" class="text-center text-secondary py-4">No hay productos</td></tr>';
}

$('#buscar-producto').addEventListener('input', renderProductos);

$('#btn-nuevo-producto').addEventListener('click', () => abrirProducto());

function abrirProducto(p = null) {
  $('#titulo-producto').textContent = p ? `Editar producto ${p.id}` : 'Nuevo producto';
  $('#prod-id').value = p?.id ?? '';
  $('#prod-nombre').value = p?.nombre ?? '';
  $('#prod-precio').value = p?.precio ?? '';
  $('#prod-stock').value = p?.stock ?? 0;
  modalProducto.show();
}

$('#tabla-productos').addEventListener('click', async (e) => {
  const btn = e.target.closest('button[data-accion]');
  if (!btn) return;
  const p = estado.productos.find((x) => x.id === Number(btn.dataset.id));
  if (btn.dataset.accion === 'editar') abrirProducto(p);
  if (btn.dataset.accion === 'carrito') {
    agregarAlCarrito(p.id, 1);
  }
  if (btn.dataset.accion === 'eliminar' && confirm(`¿Eliminar el producto "${p.nombre}"?`)) {
    try {
      await api(`${API.productos}/${p.id}`, { method: 'DELETE' });
      estado.carrito = estado.carrito.filter((c) => c.id !== p.id);
      toast('Producto eliminado');
      cargarProductos();
    } catch (err) {
      toast(err.message, 'danger');
    }
  }
});

$('#form-producto').addEventListener('submit', async (e) => {
  e.preventDefault();
  const id = $('#prod-id').value;
  const body = {
    nombre: $('#prod-nombre').value,
    precio: Number($('#prod-precio').value),
    stock: Number($('#prod-stock').value),
  };
  try {
    await api(id ? `${API.productos}/${id}` : API.productos, { method: id ? 'PUT' : 'POST', body });
    modalProducto.hide();
    toast(id ? 'Producto actualizado' : 'Producto creado');
    cargarProductos();
  } catch (err) {
    toast(err.message, 'danger');
  }
});

// ---------------------------------------------------------------- clientes

const modalCliente = new bootstrap.Modal('#modal-cliente');
let seleccionarClienteAlGuardar = false;

async function cargarClientes() {
  try {
    estado.clientes = await api(API.clientes);
  } catch (err) {
    estado.clientes = [];
    toast(err.message, 'danger');
  }
  renderClientes();
  renderNuevaFactura();
}

function renderClientes() {
  const q = $('#buscar-cliente').value.trim().toLowerCase();
  const lista = estado.clientes.filter((c) => `${c.nombre} ${c.cc} ${c.email}`.toLowerCase().includes(q));
  $('#tabla-clientes').innerHTML = lista.length
    ? lista.map((c) => `
      <tr>
        <td>${c.id}</td>
        <td>${esc(c.nombre)}</td>
        <td>${esc(c.cc)}</td>
        <td>${esc(c.email)}</td>
        <td>${esc(c.direccion)}</td>
        <td class="text-end text-nowrap">
          <button class="btn btn-sm btn-outline-primary" data-accion="editar" data-id="${c.id}" title="Editar"><i class="bi bi-pencil"></i></button>
          <button class="btn btn-sm btn-outline-danger" data-accion="eliminar" data-id="${c.id}" title="Eliminar"><i class="bi bi-trash"></i></button>
        </td>
      </tr>`).join('')
    : '<tr><td colspan="6" class="text-center text-secondary py-4">No hay clientes</td></tr>';
}

$('#buscar-cliente').addEventListener('input', renderClientes);

$('#btn-nuevo-cliente').addEventListener('click', () => abrirCliente());
$('#btn-cliente-rapido').addEventListener('click', () => {
  seleccionarClienteAlGuardar = true;
  abrirCliente();
});

function abrirCliente(c = null) {
  $('#titulo-cliente').textContent = c ? `Editar cliente ${c.id}` : 'Registrar cliente';
  $('#cli-id').value = c?.id ?? '';
  $('#cli-nombre').value = c?.nombre ?? '';
  $('#cli-cc').value = c?.cc ?? '';
  $('#cli-email').value = c?.email ?? '';
  $('#cli-direccion').value = c?.direccion ?? '';
  modalCliente.show();
}

$('#modal-cliente').addEventListener('hidden.bs.modal', () => { seleccionarClienteAlGuardar = false; });

$('#tabla-clientes').addEventListener('click', async (e) => {
  const btn = e.target.closest('button[data-accion]');
  if (!btn) return;
  const c = estado.clientes.find((x) => x.id === Number(btn.dataset.id));
  if (btn.dataset.accion === 'editar') abrirCliente(c);
  if (btn.dataset.accion === 'eliminar' && confirm(`¿Eliminar al cliente "${c.nombre}"?`)) {
    try {
      await api(`${API.clientes}/${c.id}`, { method: 'DELETE' });
      toast('Cliente eliminado');
      cargarClientes();
    } catch (err) {
      toast(err.message, 'danger');
    }
  }
});

$('#form-cliente').addEventListener('submit', async (e) => {
  e.preventDefault();
  const id = $('#cli-id').value;
  const body = {
    nombre: $('#cli-nombre').value,
    cc: $('#cli-cc').value,
    email: $('#cli-email').value,
    direccion: $('#cli-direccion').value,
  };
  try {
    const guardado = await api(id ? `${API.clientes}/${id}` : API.clientes, { method: id ? 'PUT' : 'POST', body });
    const seleccionar = seleccionarClienteAlGuardar;
    modalCliente.hide();
    toast(id ? 'Cliente actualizado' : 'Cliente registrado');
    await cargarClientes();
    if (seleccionar) {
      $('#sel-cliente').value = guardado.id;
      renderInfoCliente();
    }
  } catch (err) {
    toast(err.message, 'danger');
  }
});

// ---------------------------------------------------------------- nueva factura

function agregarAlCarrito(id, cantidad) {
  const p = estado.productos.find((x) => x.id === id);
  if (!p) return;
  const item = estado.carrito.find((c) => c.id === id);
  const total = (item?.cantidad ?? 0) + cantidad;
  if (total > p.stock) {
    toast(`Solo hay ${p.stock} unidades de "${p.nombre}"`, 'warning');
    return;
  }
  if (item) item.cantidad = total;
  else estado.carrito.push({ id, cantidad });
  toast(`"${p.nombre}" agregado a la factura`);
  renderNuevaFactura();
}

function renderInfoCliente() {
  const c = estado.clientes.find((x) => x.id === Number($('#sel-cliente').value));
  $('#info-cliente').innerHTML = c
    ? `C.C. ${esc(c.cc)}<br>${esc(c.email)}<br>${esc(c.direccion)}`
    : '';
}

function renderNuevaFactura() {
  // Selector de clientes (conserva la selección actual)
  const selCli = $('#sel-cliente');
  const clienteActual = selCli.value;
  selCli.innerHTML = '<option value="">Seleccione un cliente...</option>' +
    estado.clientes.map((c) => `<option value="${c.id}">${esc(c.nombre)} — C.C. ${esc(c.cc)}</option>`).join('');
  selCli.value = clienteActual;
  renderInfoCliente();

  // Selector de productos
  const selProd = $('#sel-producto');
  const productoActual = selProd.value;
  selProd.innerHTML = estado.productos
    .map((p) => `<option value="${p.id}" ${p.stock === 0 ? 'disabled' : ''}>${esc(p.nombre)} — ${moneda(p.precio)} (stock ${p.stock})</option>`)
    .join('');
  if (productoActual) selProd.value = productoActual;

  // Quita del carrito productos que ya no existen
  estado.carrito = estado.carrito.filter((c) => estado.productos.some((p) => p.id === c.id));

  let total = 0;
  $('#tabla-carrito').innerHTML = estado.carrito.length
    ? estado.carrito.map((c) => {
      const p = estado.productos.find((x) => x.id === c.id);
      const subtotal = p.precio * c.cantidad;
      total += subtotal;
      return `
        <tr>
          <td>${esc(p.nombre)}<br><small class="text-secondary">Código ${p.id}</small></td>
          <td class="text-end">${moneda(p.precio)}</td>
          <td><input type="number" class="form-control form-control-sm" min="1" max="${p.stock}" value="${c.cantidad}" data-cantidad="${p.id}"></td>
          <td class="text-end">${moneda(subtotal)}</td>
          <td class="text-end"><button class="btn btn-sm btn-outline-danger" data-quitar="${p.id}" title="Quitar"><i class="bi bi-x-lg"></i></button></td>
        </tr>`;
    }).join('')
    : '<tr><td colspan="5" class="text-center text-secondary py-4">Agregue productos a la factura</td></tr>';
  $('#total-carrito').textContent = moneda(total);

  const unidades = estado.carrito.reduce((s, c) => s + c.cantidad, 0);
  $('#badge-carrito').textContent = unidades;
  $('#badge-carrito').classList.toggle('d-none', unidades === 0);
}

$('#sel-cliente').addEventListener('change', renderInfoCliente);

$('#btn-agregar').addEventListener('click', () => {
  const id = Number($('#sel-producto').value);
  const cantidad = Number($('#cant-producto').value);
  if (!id) return toast('Seleccione un producto', 'warning');
  if (!Number.isInteger(cantidad) || cantidad < 1) return toast('La cantidad debe ser un entero mayor que cero', 'warning');
  agregarAlCarrito(id, cantidad);
  $('#cant-producto').value = 1;
});

$('#tabla-carrito').addEventListener('change', (e) => {
  const id = Number(e.target.dataset.cantidad);
  if (!id) return;
  const p = estado.productos.find((x) => x.id === id);
  const item = estado.carrito.find((c) => c.id === id);
  let cantidad = Math.floor(Number(e.target.value)) || 1;
  if (cantidad > p.stock) {
    toast(`Solo hay ${p.stock} unidades de "${p.nombre}"`, 'warning');
    cantidad = p.stock;
  }
  item.cantidad = Math.max(1, cantidad);
  renderNuevaFactura();
});

$('#tabla-carrito').addEventListener('click', (e) => {
  const btn = e.target.closest('[data-quitar]');
  if (!btn) return;
  estado.carrito = estado.carrito.filter((c) => c.id !== Number(btn.dataset.quitar));
  renderNuevaFactura();
});

$('#btn-vaciar').addEventListener('click', () => {
  estado.carrito = [];
  renderNuevaFactura();
});

$('#btn-generar').addEventListener('click', async () => {
  const clienteId = Number($('#sel-cliente').value);
  if (!clienteId) return toast('Seleccione el cliente de la factura', 'warning');
  if (!estado.carrito.length) return toast('Agregue al menos un producto', 'warning');

  const btn = $('#btn-generar');
  btn.disabled = true;
  try {
    const factura = await api(API.facturas, {
      method: 'POST',
      body: { cliente_id: clienteId, productos: estado.carrito },
    });
    estado.carrito = [];
    $('#sel-cliente').value = '';
    toast(`Factura No. ${factura.factura_id} generada`);
    mostrarFactura(factura);
    cargarProductos(); // el stock cambió
  } catch (err) {
    toast(err.message, 'danger');
  } finally {
    btn.disabled = false;
  }
});

// ---------------------------------------------------------------- facturas

const modalFactura = new bootstrap.Modal('#modal-factura');

async function cargarFacturas() {
  $('#tabla-facturas').innerHTML = '<tr><td colspan="7" class="text-center py-4"><div class="spinner-border spinner-border-sm"></div></td></tr>';
  try {
    const facturas = await api(API.facturas);
    $('#tabla-facturas').innerHTML = facturas.length
      ? facturas.map((f) => `
        <tr>
          <td class="fw-semibold">${f.factura_id}</td>
          <td>${esc(f.fecha)}</td>
          <td>${esc(f.cliente.nombre)}</td>
          <td>${esc(f.usuario.nombre)}</td>
          <td class="text-end">${f.productos.reduce((s, p) => s + p.cantidad, 0)}</td>
          <td class="text-end">${moneda(f.total)}</td>
          <td class="text-end"><button class="btn btn-sm btn-outline-primary" data-ver="${f.factura_id}"><i class="bi bi-eye"></i> Ver</button></td>
        </tr>`).join('')
      : '<tr><td colspan="7" class="text-center text-secondary py-4">Aún no hay facturas</td></tr>';
  } catch (err) {
    $('#tabla-facturas').innerHTML = `<tr><td colspan="7" class="text-center text-danger py-4">${esc(err.message)}</td></tr>`;
  }
}

$('#btn-recargar-facturas').addEventListener('click', cargarFacturas);

$('#tabla-facturas').addEventListener('click', async (e) => {
  const btn = e.target.closest('[data-ver]');
  if (!btn) return;
  try {
    mostrarFactura(await api(`${API.facturas}/${btn.dataset.ver}`));
  } catch (err) {
    toast(err.message, 'danger');
  }
});

function htmlFactura(f) {
  return `
  <div class="factura p-2">
    <div class="d-flex justify-content-between align-items-start border-bottom pb-2 mb-3">
      <div>
        <div class="factura-titulo">FACTURA DE VENTA</div>
        <div class="text-secondary small">Sistema de Facturación Digital</div>
      </div>
      <div class="text-end">
        <div class="factura-numero">No. ${f.factura_id}</div>
        <div class="text-secondary small">Fecha: ${esc(f.fecha)}</div>
      </div>
    </div>
    <div class="row mb-3">
      <div class="col-7">
        <div class="etiqueta">Cliente</div>
        <div class="fw-semibold">${esc(f.cliente.nombre)}</div>
        ${f.cliente.cc ? `<div>C.C. ${esc(f.cliente.cc)}</div>` : ''}
        <div>${esc(f.cliente.email)}</div>
        ${f.cliente.direccion ? `<div>${esc(f.cliente.direccion)}</div>` : ''}
      </div>
      <div class="col-5">
        <div class="etiqueta">Atendido por</div>
        <div class="fw-semibold">${esc(f.usuario.nombre)}</div>
        <div>Rol: ${esc(f.usuario.rol)}</div>
        <div>ID usuario: ${f.usuario.id}</div>
      </div>
    </div>
    <table class="table table-sm">
      <thead><tr><th>Código</th><th>Producto</th><th class="text-end">Cant.</th><th class="text-end">Precio unit.</th><th class="text-end">Subtotal</th></tr></thead>
      <tbody>
        ${f.productos.map((p) => `
          <tr><td>${p.id}</td><td>${esc(p.nombre)}</td><td class="text-end">${p.cantidad}</td>
          <td class="text-end">${moneda(p.precio_unitario)}</td><td class="text-end">${moneda(p.subtotal)}</td></tr>`).join('')}
      </tbody>
      <tfoot><tr><td colspan="4" class="text-end total">TOTAL A PAGAR</td><td class="text-end total">${moneda(f.total)}</td></tr></tfoot>
    </table>
    <p class="text-secondary small mb-0">Documento generado por el Sistema de Facturación Digital (simulación académica).</p>
  </div>`;
}

function mostrarFactura(f) {
  estado.facturaActual = f;
  $('#factura-vista').innerHTML = htmlFactura(f);
  $('#factura-json').textContent = JSON.stringify(f, null, 2);
  $('#factura-json').classList.add('d-none');
  $('#btn-ver-json').innerHTML = '<i class="bi bi-braces"></i> Ver JSON';
  modalFactura.show();
}

$('#btn-ver-json').addEventListener('click', () => {
  const oculto = $('#factura-json').classList.toggle('d-none');
  $('#btn-ver-json').innerHTML = oculto ? '<i class="bi bi-braces"></i> Ver JSON' : '<i class="bi bi-receipt"></i> Ver factura';
});

$('#btn-descargar-json').addEventListener('click', () => {
  const f = estado.facturaActual;
  descargar(new Blob([JSON.stringify(f, null, 2)], { type: 'application/json' }), `factura-${f.factura_id}.json`);
});

$('#btn-descargar-pdf').addEventListener('click', async () => {
  const f = estado.facturaActual;
  try {
    descargar(await api(`${API.facturas}/${f.factura_id}/pdf`, { blob: true }), `factura-${f.factura_id}.pdf`);
  } catch (err) {
    toast(err.message, 'danger');
  }
});

// Imprime solo la factura; el diálogo del navegador permite "Guardar como PDF".
$('#btn-imprimir').addEventListener('click', () => {
  $('#zona-impresion').innerHTML = htmlFactura(estado.facturaActual);
  window.print();
});

// ---------------------------------------------------------------- inicio

(async function iniciar() {
  let sesion = null;
  try { sesion = JSON.parse(localStorage.getItem('sesion')); } catch { /* sin sesión guardada */ }
  if (sesion?.token) {
    estado.token = sesion.token;
    try {
      // Verifica que el token siga siendo válido.
      estado.usuario = await api(`${API.auth}/me`);
      return entrar();
    } catch {
      cerrarSesion();
      return;
    }
  }
  cerrarSesion();
})();
