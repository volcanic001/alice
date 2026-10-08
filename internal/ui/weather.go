package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/volcanic001/alice/internal/config"
	"github.com/volcanic001/alice/internal/weather"
)

// Integración con Open-Meteo (ver internal/weather). A diferencia de mem0 y
// Brave, Open-Meteo no pide API key, así que /weather está siempre
// disponible — no hay comando de activación ni clave que guardar.
//
// /weather nunca llega a DeepSeek: es una consulta directa (como /help o
// /stats), no una pregunta de chat. El resultado se muestra en la línea de
// estado, no se guarda en el historial de la conversación.
//
// Las ciudades guardadas no usan alias inventado: se identifican por su
// nombre real (el que devuelve el geocoding), y se consultan escribiendo
// cualquier parte de ese nombre — ver config.FindWeatherCities.
const weatherTimeout = 8 * time.Second

type weatherGeocodeMsg struct {
	query  string
	cities []weather.City
	err    error
}

type weatherForecastMsg struct {
	city     config.WeatherCity
	forecast weather.Forecast
	err      error
}

// SetWeather activa el clima para esta sesión. A diferencia de SetMemory y
// SetWebSearch, se llama siempre al arrancar — no depende de ninguna key.
func (m *Model) SetWeather(client *weather.Client) { m.weather = client }

// handleWeatherCommand resuelve "/weather", "/weather <ciudad o parte de
// ella>", "/weather add <ciudad>", "/weather list" y "/weather remove
// <ciudad>". Se parte por la primera palabra en vez de usar HasPrefix con el
// espacio incluido — "add" sin nada detrás también debe caer en el aviso de
// uso, no tratarse como una búsqueda de ciudad llamada "add".
func (m Model) handleWeatherCommand(argument string) (tea.Model, tea.Cmd) {
	fields := strings.Fields(argument)
	var head string
	if len(fields) > 0 {
		head = fields[0]
	}
	switch {
	case argument == "":
		return m.showDefaultWeather()
	case head == "list":
		return m.listWeatherCities()
	case head == "remove":
		query := strings.TrimSpace(strings.TrimPrefix(argument, "remove"))
		return m.removeWeatherCity(query)
	case head == "add":
		query := strings.TrimSpace(strings.TrimPrefix(argument, "add"))
		return m.addWeatherCity(query)
	default:
		return m.showWeatherForQuery(argument)
	}
}

func (m Model) addWeatherCity(query string) (tea.Model, tea.Cmd) {
	if query == "" {
		m.status = "Uso: /weather add <ciudad>\nEjemplo: /weather add San Salvador, El Salvador"
		return m, nil
	}
	m.status = "Buscando " + query + "…"
	return m, m.fetchGeocode(query)
}

func (m Model) fetchGeocode(query string) tea.Cmd {
	client := m.weather
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), weatherTimeout)
		defer cancel()
		cities, err := client.Geocode(ctx, query, 5)
		return weatherGeocodeMsg{query: query, cities: cities, err: err}
	}
}

// listWeatherCities no toca la red — las ciudades guardadas viven en disco.
func (m Model) listWeatherCities() (tea.Model, tea.Cmd) {
	cities, err := config.LoadWeatherCities()
	if err != nil {
		m.status = err.Error()
		return m, nil
	}
	if len(cities) == 0 {
		m.status = "No tienes ciudades guardadas.\nUsa /weather add <ciudad> para agregar una."
		return m, nil
	}
	var text strings.Builder
	text.WriteString("Ciudades guardadas (la primera es tu default):\n")
	for i, city := range cities {
		marker := ""
		if i == 0 {
			marker = " (default)"
		}
		fmt.Fprintf(&text, "- %s, %s%s\n", city.Name, city.Country, marker)
	}
	m.status = strings.TrimRight(text.String(), "\n")
	return m, nil
}

func (m Model) removeWeatherCity(query string) (tea.Model, tea.Cmd) {
	if query == "" {
		m.status = "Uso: /weather remove <ciudad>"
		return m, nil
	}
	matches, err := config.FindWeatherCities(query)
	if err != nil {
		m.status = err.Error()
		return m, nil
	}
	switch len(matches) {
	case 0:
		m.status = "No encontré \"" + query + "\" entre tus ciudades guardadas.\nUsa /weather list para verlas."
	case 1:
		if err := config.RemoveWeatherCity(matches[0].Name, matches[0].Country); err != nil {
			m.status = err.Error()
			return m, nil
		}
		m.status = "✓ " + matches[0].Name + " eliminada"
	default:
		m.status = weatherSavedCityDisambiguation(query, matches)
	}
	return m, nil
}

// showDefaultWeather usa la primera ciudad guardada. Si no hay ninguna,
// avisa cómo agregar una en vez de fallar en silencio.
func (m Model) showDefaultWeather() (tea.Model, tea.Cmd) {
	cities, err := config.LoadWeatherCities()
	if err != nil {
		m.status = err.Error()
		return m, nil
	}
	if len(cities) == 0 {
		m.status = "No tienes ninguna ciudad guardada.\nUsa /weather add <ciudad> para agregar una, por ejemplo:\n/weather add San Salvador, El Salvador"
		return m, nil
	}
	m.status = "Consultando el clima en " + cities[0].Name + "…"
	return m, m.fetchForecast(cities[0])
}

// showWeatherForQuery busca entre las ciudades guardadas por coincidencia
// parcial. Si más de una coincide (p. ej. "san" entre "San Salvador" y "San
// Miguel"), pide que sea más específico en vez de elegir al azar.
func (m Model) showWeatherForQuery(query string) (tea.Model, tea.Cmd) {
	matches, err := config.FindWeatherCities(query)
	if err != nil {
		m.status = err.Error()
		return m, nil
	}
	switch len(matches) {
	case 0:
		m.status = "No encontré \"" + query + "\" entre tus ciudades guardadas.\nUsa /weather list para verlas, o /weather add " + query + " para agregarla."
		return m, nil
	case 1:
		m.status = "Consultando el clima en " + matches[0].Name + "…"
		return m, m.fetchForecast(matches[0])
	default:
		m.status = weatherSavedCityDisambiguation(query, matches)
		return m, nil
	}
}

func (m Model) fetchForecast(city config.WeatherCity) tea.Cmd {
	client := m.weather
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), weatherTimeout)
		defer cancel()
		forecast, err := client.Forecast(ctx, city.Latitude, city.Longitude, city.Timezone)
		return weatherForecastMsg{city: city, forecast: forecast, err: err}
	}
}

// formatForecast arma el bloque que se muestra en la línea de estado —
// mismo patrón visual de /memory y /search-key, nunca llega al modelo.
// formatForecast arma el pronóstico en markdown, no texto plano — se
// renderiza con el mismo glamour+burbuja que las respuestas de Alice (ver
// refresh()), así que un encabezado "##", negritas y una tabla salen con el
// mismo acabado visual, sin tener que alinear columnas a mano (el ancho de
// los emojis en terminal no es consistente byte a byte, una tabla markdown
// lo resuelve sola).
var (
	weatherTitleStyle = lipgloss.NewStyle().Foreground(lavender).Bold(true)
	weatherTempStyle  = lipgloss.NewStyle().Foreground(ink).Bold(true)
)

// padRight rellena s con espacios hasta width celdas de ancho visual,
// midiendo con lipgloss.Width — a diferencia de fmt's "%-Ns", que cuenta
// runas, esto sí sabe que un emoji ocupa 2 celdas en la terminal, así que
// la tabla del pronóstico no se desalinea entre filas.
func padRight(s string, width int) string {
	if gap := width - lipgloss.Width(s); gap > 0 {
		return s + strings.Repeat(" ", gap)
	}
	return s
}

// formatForecast arma el bloque de clima con estilos de lipgloss directos
// (el mismo mecanismo que usa /stats), en vez de markdown — da control
// preciso del color sin depender del parser de glamour. Solo lleva un
// emoji por línea, el del propio código de clima (información real, no
// decoración); el resto de datos van en texto con color, no íconos.
func formatForecast(city config.WeatherCity, forecast weather.Forecast) string {
	var text strings.Builder

	location := city.Name
	if city.Country != "" {
		location += ", " + city.Country
	}
	text.WriteString(weatherTitleStyle.Render(location) + "\n\n")

	emoji, description := weather.DescribeWeatherCode(forecast.Current.WeatherCode)
	fmt.Fprintf(&text, "%s %s %s\n", weatherTempStyle.Render(fmt.Sprintf("%.1f°C", forecast.Current.TemperatureC)), emoji, description)
	text.WriteString(mutedStyle.Render(fmt.Sprintf("Sensación %.0f°C · Humedad %d%% · Viento %.0f km/h",
		forecast.Current.ApparentC, forecast.Current.HumidityPercent, forecast.Current.WindSpeedKMH)) + "\n")

	if len(forecast.Daily) > 0 {
		text.WriteString("\n" + weatherTitleStyle.Render("Pronóstico") + "\n")
		for i, day := range forecast.Daily {
			dayEmoji, _ := weather.DescribeWeatherCode(day.WeatherCode)
			label := weekdayLabel(day.Date)
			switch i {
			case 0:
				label = "Hoy"
			case 1:
				label = "Mañana"
			}
			degrees := fmt.Sprintf("%.0f–%.0f°C", day.TempMinC, day.TempMaxC)
			rain := mutedStyle.Render(fmt.Sprintf("%.1fmm", day.PrecipitationMM))
			text.WriteString(padRight(label, 8) + " " + dayEmoji + " " + padRight(degrees, 11) + rain + "\n")
		}
		text.WriteString("\n" + mutedStyle.Render(fmt.Sprintf("Amanece %s · Anochece %s", forecast.Daily[0].Sunrise, forecast.Daily[0].Sunset)) + "\n")
	}
	text.WriteString(mutedStyle.Render("Open-Meteo"))
	return text.String()
}

var spanishWeekdayAbbrev = [7]string{"Dom", "Lun", "Mar", "Mié", "Jue", "Vie", "Sáb"}

// weekdayLabel convierte "2026-10-09" en "Vie 09". Si la fecha no parsea
// (no debería pasar con lo que manda Open-Meteo), devuelve la fecha tal cual
// en vez de fallar.
func weekdayLabel(isoDate string) string {
	parsed, err := time.Parse("2006-01-02", isoDate)
	if err != nil {
		return isoDate
	}
	return fmt.Sprintf("%s %02d", spanishWeekdayAbbrev[parsed.Weekday()], parsed.Day())
}

// weatherCityDisambiguation arma el aviso cuando el geocoding (al agregar)
// devuelve más de un lugar con ese nombre — el usuario tiene que repetir el
// comando siendo más específico (agregando país o departamento).
func weatherCityDisambiguation(query string, cities []weather.City) string {
	var text strings.Builder
	fmt.Fprintf(&text, "\"%s\" tiene %d coincidencias, sé más específico:\n", query, len(cities))
	for _, city := range cities {
		location := city.Country
		if city.Admin1 != "" {
			location = city.Admin1 + ", " + city.Country
		}
		fmt.Fprintf(&text, "- %s (%s)\n", city.Name, location)
	}
	text.WriteString("Agrega el departamento o país a la búsqueda, p. ej. \"" + cities[0].Name + " " + cities[0].Country + "\"")
	return text.String()
}

// weatherSavedCityDisambiguation es el equivalente a weatherCityDisambiguation
// pero para cuando la ambigüedad es entre tus propias ciudades ya guardadas
// (al consultar o al quitar), no entre resultados de geocoding.
func weatherSavedCityDisambiguation(query string, cities []config.WeatherCity) string {
	var text strings.Builder
	fmt.Fprintf(&text, "\"%s\" coincide con %d ciudades guardadas, sé más específico:\n", query, len(cities))
	for _, city := range cities {
		fmt.Fprintf(&text, "- %s, %s\n", city.Name, city.Country)
	}
	text.WriteString("Agrega el país a la búsqueda, p. ej. \"" + cities[0].Name + " " + cities[0].Country + "\"")
	return text.String()
}
