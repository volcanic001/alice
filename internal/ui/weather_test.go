package ui

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/volcanic001/alice/internal/config"
	"github.com/volcanic001/alice/internal/weather"
)

func TestWeatherWithoutSavedCitiesShowsHint(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	model := newMemoryTestModel(t, &scriptedProvider{})
	model.SetWeather(weather.New())

	model.input.SetValue("/weather")
	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = next.(Model)

	if !strings.Contains(model.status, "No tienes ninguna ciudad guardada") {
		t.Fatalf("estado inesperado: %q", model.status)
	}
}

func TestWeatherAddGeocodesAndSavesSingleMatch(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	geocoding := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Query().Get("name") != "Chalatenango" {
			t.Errorf("query inesperada: %q", request.URL.Query().Get("name"))
		}
		fmt.Fprint(response, `{"results":[{"name":"Chalatenango","country":"El Salvador","admin1":"Departamento de Chalatenango","latitude":14.04138,"longitude":-88.93951,"timezone":"America/El_Salvador"}]}`)
	}))
	defer geocoding.Close()

	model := newMemoryTestModel(t, &scriptedProvider{})
	client := weather.New()
	client.GeocodingBaseURL = geocoding.URL
	model.SetWeather(client)

	model.input.SetValue("/weather add Chalatenango")
	next, command := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = drive(t, next.(Model), command, nil)

	if !strings.Contains(model.status, "Chalatenango") || !strings.Contains(model.status, "guardada") {
		t.Fatalf("no se confirmó el guardado: %q", model.status)
	}
	saved, err := config.FindWeatherCities("chalatenango")
	if err != nil {
		t.Fatal(err)
	}
	if len(saved) != 1 || saved[0].Name != "Chalatenango" || saved[0].Latitude != 14.04138 || saved[0].Timezone != "America/El_Salvador" {
		t.Fatalf("ciudad guardada inesperada: %#v", saved)
	}
}

func TestWeatherAddWithoutResultsSuggestsFullName(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	geocoding := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		fmt.Fprint(response, `{"generationtime_ms":0.6}`)
	}))
	defer geocoding.Close()

	model := newMemoryTestModel(t, &scriptedProvider{})
	client := weather.New()
	client.GeocodingBaseURL = geocoding.URL
	model.SetWeather(client)

	model.input.SetValue("/weather add Tepezontes")
	next, command := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = drive(t, next.(Model), command, nil)

	if !strings.Contains(model.status, "No encontré") || !strings.Contains(model.status, "nombre oficial completo") {
		t.Fatalf("no se avisó la falta de resultados: %q", model.status)
	}
	saved, err := config.FindWeatherCities("tepezontes")
	if err != nil {
		t.Fatal(err)
	}
	if len(saved) != 0 {
		t.Fatal("no debió guardarse nada sin resultados")
	}
}

func TestWeatherAddWithMultipleGeocodeResultsAsksToDisambiguate(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	geocoding := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		fmt.Fprint(response, `{"results":[
			{"name":"San Pedro","country":"El Salvador","admin1":"Usulután","latitude":1,"longitude":1},
			{"name":"San Pedro","country":"México","admin1":"Coahuila","latitude":2,"longitude":2}
		]}`)
	}))
	defer geocoding.Close()

	model := newMemoryTestModel(t, &scriptedProvider{})
	client := weather.New()
	client.GeocodingBaseURL = geocoding.URL
	model.SetWeather(client)

	model.input.SetValue("/weather add San Pedro")
	next, command := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = drive(t, next.(Model), command, nil)

	if !strings.Contains(model.status, "2 coincidencias") || !strings.Contains(model.status, "Usulután") || !strings.Contains(model.status, "Coahuila") {
		t.Fatalf("no se mostró la desambiguación: %q", model.status)
	}
	saved, err := config.FindWeatherCities("san pedro")
	if err != nil {
		t.Fatal(err)
	}
	if len(saved) != 0 {
		t.Fatal("no debió guardarse nada mientras es ambiguo")
	}
}

func TestWeatherAddWithoutArgumentsShowsUsage(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	model := newMemoryTestModel(t, &scriptedProvider{})
	model.SetWeather(weather.New())

	model.input.SetValue("/weather add")
	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = next.(Model)

	if !strings.Contains(model.status, "Uso: /weather add") {
		t.Fatalf("faltó el mensaje de uso: %q", model.status)
	}
}

func TestWeatherShowsForecastForSavedCityByPartialName(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	forecast := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Query().Get("latitude") != "13.69" {
			t.Errorf("latitude inesperada: %q", request.URL.Query().Get("latitude"))
		}
		fmt.Fprint(response, `{
			"current": {"temperature_2m":22.6,"relative_humidity_2m":100,"apparent_temperature":27.6,"precipitation":0.1,"weather_code":51,"wind_speed_10m":1.3,"cloud_cover":98},
			"daily": {
				"time":["2026-10-07"],
				"weather_code":[95],
				"temperature_2m_max":[27.6],
				"temperature_2m_min":[21.4],
				"precipitation_sum":[11.0],
				"sunrise":["2026-10-07T05:46"],
				"sunset":["2026-10-07T17:42"]
			}
		}`)
	}))
	defer forecast.Close()

	if err := config.SaveWeatherCity(config.WeatherCity{Name: "San Salvador", Country: "El Salvador", Latitude: 13.69, Longitude: -89.19, Timezone: "America/El_Salvador"}); err != nil {
		t.Fatal(err)
	}

	model := newMemoryTestModel(t, &scriptedProvider{})
	client := weather.New()
	client.ForecastBaseURL = forecast.URL
	model.SetWeather(client)

	model.input.SetValue("/weather salvador")
	next, command := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = drive(t, next.(Model), command, nil)

	if !strings.Contains(model.weatherDisplay, "San Salvador") || !strings.Contains(model.weatherDisplay, "22.6") || !strings.Contains(model.weatherDisplay, "Llovizna ligera") {
		t.Fatalf("pronóstico inesperado: %q", model.weatherDisplay)
	}
	if !strings.Contains(model.weatherDisplay, "Open-Meteo") {
		t.Fatalf("faltó la atribución de la fuente: %q", model.weatherDisplay)
	}
	if model.status != "Listo" {
		t.Fatalf("estado final inesperado: %q", model.status)
	}
}

func TestWeatherQueryAmbiguousAcrossSavedCitiesAsksToDisambiguate(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if err := config.SaveWeatherCity(config.WeatherCity{Name: "San Salvador", Country: "El Salvador"}); err != nil {
		t.Fatal(err)
	}
	if err := config.SaveWeatherCity(config.WeatherCity{Name: "San Miguel", Country: "El Salvador"}); err != nil {
		t.Fatal(err)
	}

	provider := &scriptedProvider{}
	model := newMemoryTestModel(t, provider)
	model.SetWeather(weather.New())

	model.input.SetValue("/weather san")
	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = next.(Model)

	if !strings.Contains(model.status, "2 ciudades guardadas") || !strings.Contains(model.status, "San Salvador") || !strings.Contains(model.status, "San Miguel") {
		t.Fatalf("no se avisó la ambigüedad entre guardadas: %q", model.status)
	}
}

func TestWeatherUnknownQuerySuggestsAddOrList(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	model := newMemoryTestModel(t, &scriptedProvider{})
	model.SetWeather(weather.New())

	model.input.SetValue("/weather no-existe")
	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = next.(Model)

	if !strings.Contains(model.status, "No encontré") || !strings.Contains(model.status, "/weather list") {
		t.Fatalf("aviso inesperado: %q", model.status)
	}
}

func TestWeatherListShowsSavedCitiesWithDefaultMarker(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if err := config.SaveWeatherCity(config.WeatherCity{Name: "San Salvador", Country: "El Salvador"}); err != nil {
		t.Fatal(err)
	}
	if err := config.SaveWeatherCity(config.WeatherCity{Name: "Chalatenango", Country: "El Salvador"}); err != nil {
		t.Fatal(err)
	}

	model := newMemoryTestModel(t, &scriptedProvider{})
	model.SetWeather(weather.New())

	model.input.SetValue("/weather list")
	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = next.(Model)

	if !strings.Contains(model.status, "San Salvador") || !strings.Contains(model.status, "Chalatenango") || !strings.Contains(model.status, "(default)") {
		t.Fatalf("listado inesperado: %q", model.status)
	}
}

func TestWeatherRemoveByPartialNameDeletesSavedCity(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if err := config.SaveWeatherCity(config.WeatherCity{Name: "San Salvador", Country: "El Salvador"}); err != nil {
		t.Fatal(err)
	}

	model := newMemoryTestModel(t, &scriptedProvider{})
	model.SetWeather(weather.New())

	model.input.SetValue("/weather remove salvador")
	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = next.(Model)

	if !strings.Contains(model.status, "eliminada") {
		t.Fatalf("estado inesperado: %q", model.status)
	}
	matches, err := config.FindWeatherCities("san salvador")
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Fatal("la ciudad debió quedar eliminada")
	}
}

func TestWeatherRemoveAmbiguousAsksToDisambiguate(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if err := config.SaveWeatherCity(config.WeatherCity{Name: "San Salvador", Country: "El Salvador"}); err != nil {
		t.Fatal(err)
	}
	if err := config.SaveWeatherCity(config.WeatherCity{Name: "San Miguel", Country: "El Salvador"}); err != nil {
		t.Fatal(err)
	}

	model := newMemoryTestModel(t, &scriptedProvider{})
	model.SetWeather(weather.New())

	model.input.SetValue("/weather remove san")
	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = next.(Model)

	if !strings.Contains(model.status, "2 ciudades guardadas") {
		t.Fatalf("no se avisó la ambigüedad al quitar: %q", model.status)
	}
	cities, err := config.LoadWeatherCities()
	if err != nil {
		t.Fatal(err)
	}
	if len(cities) != 2 {
		t.Fatal("no debió borrarse nada mientras es ambiguo")
	}
}

// Ninguna variante de /weather debe tocar al provider ni al historial: es
// una consulta directa, no una pregunta de chat.
func TestWeatherNeverReachesProviderOrHistory(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	forecast := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		fmt.Fprint(response, `{"current":{"temperature_2m":22,"weather_code":0},"daily":{"time":[],"weather_code":[],"temperature_2m_max":[],"temperature_2m_min":[],"precipitation_sum":[],"sunrise":[],"sunset":[]}}`)
	}))
	defer forecast.Close()
	if err := config.SaveWeatherCity(config.WeatherCity{Name: "San Salvador", Country: "El Salvador", Latitude: 13.69, Longitude: -89.19}); err != nil {
		t.Fatal(err)
	}

	provider := &scriptedProvider{}
	model := newMemoryTestModel(t, provider)
	client := weather.New()
	client.ForecastBaseURL = forecast.URL
	model.SetWeather(client)

	model.input.SetValue("/weather salvador")
	next, command := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = drive(t, next.(Model), command, nil)

	if len(provider.requests) != 0 || len(model.messages) != 0 {
		t.Fatalf("/weather no debe tocar al provider ni al historial: requests=%d mensajes=%d", len(provider.requests), len(model.messages))
	}
}

// La burbuja de clima es transitoria: debe desaparecer en cuanto el usuario
// empieza un turno de chat nuevo, para no quedar flotando fuera de orden
// debajo de una respuesta posterior que no tiene nada que ver.
func TestWeatherDisplayClearsOnNextChatTurn(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	forecast := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		fmt.Fprint(response, `{"current":{"temperature_2m":22,"weather_code":0},"daily":{"time":[],"weather_code":[],"temperature_2m_max":[],"temperature_2m_min":[],"precipitation_sum":[],"sunrise":[],"sunset":[]}}`)
	}))
	defer forecast.Close()
	if err := config.SaveWeatherCity(config.WeatherCity{Name: "San Salvador", Country: "El Salvador"}); err != nil {
		t.Fatal(err)
	}

	model := newMemoryTestModel(t, &scriptedProvider{})
	client := weather.New()
	client.ForecastBaseURL = forecast.URL
	model.SetWeather(client)

	model.input.SetValue("/weather salvador")
	next, command := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = drive(t, next.(Model), command, nil)
	if model.weatherDisplay == "" {
		t.Fatal("se esperaba una burbuja de clima antes de la siguiente pregunta")
	}

	model.input.SetValue("hola")
	next, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = next.(Model)
	if model.weatherDisplay != "" {
		t.Fatalf("la burbuja de clima debió limpiarse al empezar un turno nuevo: %q", model.weatherDisplay)
	}
}
