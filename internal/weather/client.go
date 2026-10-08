// Package weather habla con Open-Meteo (https://open-meteo.com) para traer
// el clima actual y el pronóstico de varios días. A diferencia de mem0 y
// Brave, Open-Meteo no requiere API key — por eso /weather no tiene un
// comando de activación, siempre está disponible.
package weather

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

const (
	defaultGeocodingBaseURL = "https://geocoding-api.open-meteo.com/v1"
	defaultForecastBaseURL  = "https://api.open-meteo.com/v1"
)

// Client es un cliente REST mínimo para el geocoding y el forecast de
// Open-Meteo. Son dos servicios con dominios distintos, por eso tiene dos
// URLs base en vez de una.
type Client struct {
	GeocodingBaseURL string // opcional; por defecto geocoding-api.open-meteo.com/v1
	ForecastBaseURL  string // opcional; por defecto api.open-meteo.com/v1
	HTTPClient       *http.Client
}

// New crea un cliente de Open-Meteo. No necesita credenciales.
func New() *Client {
	return &Client{}
}

func (c *Client) geocodingBaseURL() string {
	if c.GeocodingBaseURL == "" {
		return defaultGeocodingBaseURL
	}
	return strings.TrimRight(c.GeocodingBaseURL, "/")
}

func (c *Client) forecastBaseURL() string {
	if c.ForecastBaseURL == "" {
		return defaultForecastBaseURL
	}
	return strings.TrimRight(c.ForecastBaseURL, "/")
}

func (c *Client) httpClient() *http.Client {
	if c.HTTPClient == nil {
		return http.DefaultClient
	}
	return c.HTTPClient
}

func (c *Client) get(ctx context.Context, baseURL, path string, params url.Values) (io.ReadCloser, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+path+"?"+params.Encode(), nil)
	if err != nil {
		return nil, err
	}
	response, err := c.httpClient().Do(request)
	if err != nil {
		return nil, err
	}
	if response.StatusCode >= 300 {
		defer response.Body.Close()
		body, _ := io.ReadAll(io.LimitReader(response.Body, 2048))
		return nil, fmt.Errorf("open-meteo: %s devolvió %d: %s", path, response.StatusCode, strings.TrimSpace(string(body)))
	}
	return response.Body, nil
}

// City es un lugar resuelto por geocoding — la base de datos es GeoNames,
// así que necesita el nombre oficial del lugar ("San Juan Tepezontes"), no
// un apodo corto ("Tepezontes" solo no encuentra nada).
type City struct {
	Name      string
	Country   string
	Admin1    string // departamento/estado/provincia
	Latitude  float64
	Longitude float64
	Timezone  string
}

type geocodeResponse struct {
	Results []struct {
		Name      string  `json:"name"`
		Country   string  `json:"country"`
		Admin1    string  `json:"admin1"`
		Latitude  float64 `json:"latitude"`
		Longitude float64 `json:"longitude"`
		Timezone  string  `json:"timezone"`
	} `json:"results"`
}

// Geocode busca un lugar por nombre y devuelve hasta `count` coincidencias,
// de la más a la menos relevante. Una consulta sin resultados no es un
// error — devuelve una lista vacía.
func (c *Client) Geocode(ctx context.Context, query string, count int) ([]City, error) {
	params := url.Values{}
	params.Set("name", query)
	params.Set("language", "es")
	if count > 0 {
		params.Set("count", strconv.Itoa(count))
	}
	body, err := c.get(ctx, c.geocodingBaseURL(), "/search", params)
	if err != nil {
		return nil, err
	}
	defer body.Close()

	var parsed geocodeResponse
	if err := json.NewDecoder(body).Decode(&parsed); err != nil {
		return nil, err
	}
	cities := make([]City, 0, len(parsed.Results))
	for _, item := range parsed.Results {
		cities = append(cities, City{
			Name: item.Name, Country: item.Country, Admin1: item.Admin1,
			Latitude: item.Latitude, Longitude: item.Longitude, Timezone: item.Timezone,
		})
	}
	return cities, nil
}

// CurrentWeather es la condición del clima en el momento de la consulta.
type CurrentWeather struct {
	TemperatureC      float64
	ApparentC         float64
	HumidityPercent   int
	PrecipitationMM   float64
	WeatherCode       int
	WindSpeedKMH      float64
	CloudCoverPercent int
}

// DailyWeather es el resumen de un día del pronóstico.
type DailyWeather struct {
	Date            string // YYYY-MM-DD
	WeatherCode     int
	TempMaxC        float64
	TempMinC        float64
	PrecipitationMM float64
	Sunrise         string // HH:MM local
	Sunset          string // HH:MM local
}

// Forecast combina el clima actual con el pronóstico diario.
type Forecast struct {
	Current CurrentWeather
	Daily   []DailyWeather
}

const forecastDays = 7

type forecastResponse struct {
	Current struct {
		Temperature2m       float64 `json:"temperature_2m"`
		RelativeHumidity2m  int     `json:"relative_humidity_2m"`
		ApparentTemperature float64 `json:"apparent_temperature"`
		Precipitation       float64 `json:"precipitation"`
		WeatherCode         int     `json:"weather_code"`
		WindSpeed10m        float64 `json:"wind_speed_10m"`
		CloudCover          int     `json:"cloud_cover"`
	} `json:"current"`
	Daily struct {
		Time             []string  `json:"time"`
		WeatherCode      []int     `json:"weather_code"`
		Temperature2mMax []float64 `json:"temperature_2m_max"`
		Temperature2mMin []float64 `json:"temperature_2m_min"`
		PrecipitationSum []float64 `json:"precipitation_sum"`
		Sunrise          []string  `json:"sunrise"`
		Sunset           []string  `json:"sunset"`
	} `json:"daily"`
}

// Forecast trae el clima actual y los próximos forecastDays días para unas
// coordenadas. timezone es el nombre IANA (p. ej. "America/El_Salvador");
// si viene vacío, Open-Meteo usa UTC, así que conviene siempre mandar el que
// devolvió Geocode para esa misma ciudad.
func (c *Client) Forecast(ctx context.Context, latitude, longitude float64, timezone string) (Forecast, error) {
	params := url.Values{}
	params.Set("latitude", strconv.FormatFloat(latitude, 'f', -1, 64))
	params.Set("longitude", strconv.FormatFloat(longitude, 'f', -1, 64))
	params.Set("current", "temperature_2m,relative_humidity_2m,apparent_temperature,precipitation,weather_code,wind_speed_10m,cloud_cover")
	params.Set("daily", "weather_code,temperature_2m_max,temperature_2m_min,precipitation_sum,sunrise,sunset")
	params.Set("forecast_days", strconv.Itoa(forecastDays))
	if timezone != "" {
		params.Set("timezone", timezone)
	}
	body, err := c.get(ctx, c.forecastBaseURL(), "/forecast", params)
	if err != nil {
		return Forecast{}, err
	}
	defer body.Close()

	var parsed forecastResponse
	if err := json.NewDecoder(body).Decode(&parsed); err != nil {
		return Forecast{}, err
	}

	forecast := Forecast{Current: CurrentWeather{
		TemperatureC: parsed.Current.Temperature2m, ApparentC: parsed.Current.ApparentTemperature,
		HumidityPercent: parsed.Current.RelativeHumidity2m, PrecipitationMM: parsed.Current.Precipitation,
		WeatherCode: parsed.Current.WeatherCode, WindSpeedKMH: parsed.Current.WindSpeed10m,
		CloudCoverPercent: parsed.Current.CloudCover,
	}}
	for i := range parsed.Daily.Time {
		forecast.Daily = append(forecast.Daily, DailyWeather{
			Date: parsed.Daily.Time[i], WeatherCode: parsed.Daily.WeatherCode[i],
			TempMaxC: parsed.Daily.Temperature2mMax[i], TempMinC: parsed.Daily.Temperature2mMin[i],
			PrecipitationMM: parsed.Daily.PrecipitationSum[i],
			Sunrise:         timeOfDay(parsed.Daily.Sunrise[i]), Sunset: timeOfDay(parsed.Daily.Sunset[i]),
		})
	}
	return forecast, nil
}

// timeOfDay recorta "2026-10-07T05:46" a "05:46". Open-Meteo siempre manda
// sunrise/sunset en ese formato ISO sin zona (ya está en hora local porque
// se pidió con el parámetro timezone).
func timeOfDay(isoDateTime string) string {
	if index := strings.IndexByte(isoDateTime, 'T'); index >= 0 && index+1 < len(isoDateTime) {
		return isoDateTime[index+1:]
	}
	return isoDateTime
}

// weatherCodeInfo es la descripción en español + emoji de un código WMO,
// tal como lo documenta Open-Meteo (open-meteo.com/en/docs).
type weatherCodeInfo struct {
	Emoji       string
	Description string
}

var weatherCodes = map[int]weatherCodeInfo{
	0:  {"☀️", "Despejado"},
	1:  {"🌤️", "Mayormente despejado"},
	2:  {"⛅", "Parcialmente nublado"},
	3:  {"☁️", "Nublado"},
	45: {"🌫️", "Niebla"},
	48: {"🌫️", "Niebla con escarcha"},
	51: {"🌦️", "Llovizna ligera"},
	53: {"🌦️", "Llovizna moderada"},
	55: {"🌧️", "Llovizna densa"},
	56: {"🌧️", "Llovizna helada ligera"},
	57: {"🌧️", "Llovizna helada densa"},
	61: {"🌧️", "Lluvia ligera"},
	63: {"🌧️", "Lluvia moderada"},
	65: {"🌧️", "Lluvia fuerte"},
	66: {"🌧️", "Lluvia helada ligera"},
	67: {"🌧️", "Lluvia helada fuerte"},
	71: {"🌨️", "Nevada ligera"},
	73: {"🌨️", "Nevada moderada"},
	75: {"❄️", "Nevada fuerte"},
	77: {"❄️", "Granizo de nieve"},
	80: {"🌦️", "Chubascos ligeros"},
	81: {"🌧️", "Chubascos moderados"},
	82: {"⛈️", "Chubascos violentos"},
	85: {"🌨️", "Chubascos de nieve ligeros"},
	86: {"❄️", "Chubascos de nieve fuertes"},
	95: {"⛈️", "Tormenta"},
	96: {"⛈️", "Tormenta con granizo ligero"},
	97: {"⛈️", "Tormenta fuerte"},
	99: {"⛈️", "Tormenta con granizo fuerte"},
}

// DescribeWeatherCode traduce un código WMO a emoji + texto. Un código no
// reconocido (Open-Meteo podría agregar nuevos) cae a un texto genérico en
// vez de fallar.
func DescribeWeatherCode(code int) (emoji, description string) {
	if info, ok := weatherCodes[code]; ok {
		return info.Emoji, info.Description
	}
	return "🌡️", fmt.Sprintf("código %d sin describir", code)
}
