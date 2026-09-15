# Alice

Alice es un chatbot TUI para DeepSeek, diseñado primero para **Termux**:
rápido, adaptable, minimalista y manejable con teclado.

> Estado: primera versión en desarrollo.

## Características

- Respuestas de DeepSeek en streaming.
- Historial de múltiples conversaciones en SQLite.
- Chat centrado y limpio, sin barra lateral permanente.
- Renderizado Markdown para las respuestas.
- Arquitectura preparada para incorporar más proveedores.
- La clave API permanece fuera de la base de datos.

## Diseño

Alice usa un diseño keyboard-first. El historial vive en una pantalla dedicada
para conservar la pantalla principal enfocada en el chat.

En Termux y terminales estrechas, el chat aprovecha prácticamente todo el ancho
disponible. En desktop o terminales grandes, el contenido queda centrado con un
ancho máximo razonable y márgenes laterales simétricos.

La TUI está construida con:

- Bubble Tea para estado, eventos y navegación.
- Lip Gloss para layout, dimensiones y estilos.
- Glamour para renderizar Markdown.

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

Alice guarda el historial en `~/.config/alice/alice.db`. Puedes cambiar el
endpoint compatible mediante `DEEPSEEK_BASE_URL`.

## Atajos

| Tecla | Acción |
|---|---|
| `Enter` | Enviar mensaje o abrir la conversación seleccionada en Historial |
| `Shift+Enter` o `Ctrl+J` | Nueva línea en el input |
| `Ctrl+N` | Nueva conversación |
| `Ctrl+H` | Abrir Historial |
| `↑` / `↓` | Seleccionar conversación dentro de Historial |
| `Esc` | Volver de Historial al chat o cancelar una respuesta en curso |
| `PgUp` / `PgDn` | Desplazarse por el contenido del chat |
| `Home` / `End` | Moverse al inicio o final de la línea dentro del input |
| `Ctrl+C` | Salir |

## Licencia

[MIT](LICENSE)
