package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// WeatherCity es una ubicación guardada por /weather add. Se identifica por
// su nombre real (el que devolvió el geocoding de Open-Meteo) — no hay alias
// inventado: para consultarla después basta con escribir una parte de ese
// nombre, ver FindWeatherCities.
type WeatherCity struct {
	Name      string  `json:"name"`
	Country   string  `json:"country"`
	Admin1    string  `json:"admin1,omitempty"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Timezone  string  `json:"timezone"`
}

const weatherCitiesFile = "weather_cities.json"

// LoadWeatherCities devuelve las ciudades guardadas, en el orden en que se
// agregaron. La primera es la ciudad por defecto de /weather sin argumentos.
// Si el archivo no existe todavía, devuelve una lista vacía, no error.
func LoadWeatherCities() ([]WeatherCity, error) {
	directory, err := Directory()
	if err != nil {
		return nil, err
	}
	content, err := os.ReadFile(filepath.Join(directory, weatherCitiesFile))
	switch {
	case err == nil:
		var cities []WeatherCity
		if err := json.Unmarshal(content, &cities); err != nil {
			return nil, err
		}
		return cities, nil
	case os.IsNotExist(err):
		return nil, nil
	default:
		return nil, err
	}
}

func saveWeatherCities(cities []WeatherCity) error {
	directory, err := Directory()
	if err != nil {
		return err
	}
	encoded, err := json.MarshalIndent(cities, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(directory, weatherCitiesFile), encoded, 0600)
}

// sameWeatherCity compara por nombre+país, sin distinguir mayúsculas — es lo
// que decide si /weather add actualiza una ciudad ya guardada en vez de
// duplicarla.
func sameWeatherCity(a, b WeatherCity) bool {
	return strings.EqualFold(a.Name, b.Name) && strings.EqualFold(a.Country, b.Country)
}

// SaveWeatherCity agrega una ciudad nueva, o la reemplaza si ya tenías
// guardada una con el mismo nombre y país (conservando su posición, para no
// perder el default sin querer si era la primera).
func SaveWeatherCity(city WeatherCity) error {
	cities, err := LoadWeatherCities()
	if err != nil {
		return err
	}
	for i, existing := range cities {
		if sameWeatherCity(existing, city) {
			cities[i] = city
			return saveWeatherCities(cities)
		}
	}
	cities = append(cities, city)
	return saveWeatherCities(cities)
}

// ErrWeatherCityNotFound se devuelve cuando ninguna ciudad guardada coincide
// con la búsqueda.
var ErrWeatherCityNotFound = errors.New("no hay ninguna ciudad guardada que coincida")

// FindWeatherCities busca, entre las ciudades guardadas, las que coincidan
// con query — contra nombre, departamento y país juntos, sin distinguir
// mayúsculas ni acentos exactos. Una query vacía no matchea nada.
func FindWeatherCities(query string) ([]WeatherCity, error) {
	cities, err := LoadWeatherCities()
	if err != nil {
		return nil, err
	}
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return nil, nil
	}
	var matches []WeatherCity
	for _, city := range cities {
		haystack := strings.ToLower(city.Name + ", " + city.Admin1 + ", " + city.Country)
		if strings.Contains(haystack, query) {
			matches = append(matches, city)
		}
	}
	return matches, nil
}

// RemoveWeatherCity quita la ciudad guardada con ese nombre+país exactos
// (no por coincidencia parcial — así no borra de más por accidente). El
// llamador debe resolver antes cuál ciudad exacta quitar, p. ej. con
// FindWeatherCities.
func RemoveWeatherCity(name, country string) error {
	cities, err := LoadWeatherCities()
	if err != nil {
		return err
	}
	filtered := make([]WeatherCity, 0, len(cities))
	for _, city := range cities {
		if !(strings.EqualFold(city.Name, name) && strings.EqualFold(city.Country, country)) {
			filtered = append(filtered, city)
		}
	}
	return saveWeatherCities(filtered)
}
