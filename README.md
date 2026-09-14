# Alice

Un chatbot TUI para DeepSeek, diseñado primero para **Termux**: rápido, adaptable,
minimalista y completamente manejable con teclado.

> Estado: primera versión en desarrollo.

## Características

- Respuestas de DeepSeek en streaming.
- Historial de múltiples conversaciones en SQLite.
- Diseño compacto en teléfonos y barra lateral en terminales amplias.
- Scroll con gesto/rueda, `PgUp` y `PgDn`.
- Arquitectura preparada para incorporar más proveedores.
- La clave API permanece fuera de la base de datos.

## Instalar en Termux

```sh
pkg install golang
go install github.com/volcanic001/alice/cmd/alice@latest
```

Durante el desarrollo:

```sh
git clone https://github.com/volcanic001/alice.git
cd alice
go build ./cmd/alice
export DEEPSEEK_API_KEY="tu_clave"
./alice
```

Alice guarda el historial en `~/.config/alice/alice.db`. Puedes cambiar el endpoint
compatible mediante `DEEPSEEK_BASE_URL`.

## Atajos

| Tecla | Acción |
|---|---|
| `Enter` | Enviar |
| `Shift+Enter` o `Ctrl+J` | Nueva línea |
| `Ctrl+N` | Nueva conversación |
| `Alt+↑` / `Alt+↓` | Cambiar conversación |
| `PgUp` / `PgDn` | Recorrer el chat |
| `Esc` | Cancelar una respuesta |
| `Ctrl+C` | Salir |

## Licencia

[MIT](LICENSE)
