package weather

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGeocodeSendsQueryAndParsesResults(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/search" {
			t.Errorf("ruta inesperada: %s", request.URL.Path)
		}
		if request.URL.Query().Get("name") != "Chalatenango" {
			t.Errorf("query inesperada: %q", request.URL.Query().Get("name"))
		}
		fmt.Fprint(response, `{"results":[{"name":"Chalatenango","country":"El Salvador","admin1":"Departamento de Chalatenango","latitude":14.04138,"longitude":-88.93951,"timezone":"America/El_Salvador"}]}`)
	}))
	defer server.Close()

	client := Client{GeocodingBaseURL: server.URL}
	cities, err := client.Geocode(context.Background(), "Chalatenango", 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(cities) != 1 || cities[0].Name != "Chalatenango" || cities[0].Country != "El Salvador" || cities[0].Latitude != 14.04138 {
		t.Fatalf("resultado inesperado: %#v", cities)
	}
}

func TestGeocodeHandlesNoResults(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		fmt.Fprint(response, `{"generationtime_ms":0.63}`)
	}))
	defer server.Close()

	client := Client{GeocodingBaseURL: server.URL}
	cities, err := client.Geocode(context.Background(), "Tepezontes", 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(cities) != 0 {
		t.Fatalf("se esperaban cero resultados: %#v", cities)
	}
}

func TestForecastParsesCurrentAndDaily(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/forecast" {
			t.Errorf("ruta inesperada: %s", request.URL.Path)
		}
		if request.URL.Query().Get("latitude") != "13.6929" {
			t.Errorf("latitude inesperada: %q", request.URL.Query().Get("latitude"))
		}
		if request.URL.Query().Get("timezone") != "America/El_Salvador" {
			t.Errorf("timezone inesperado: %q", request.URL.Query().Get("timezone"))
		}
		if request.URL.Query().Get("forecast_days") != "7" {
			t.Errorf("forecast_days inesperado: %q", request.URL.Query().Get("forecast_days"))
		}
		fmt.Fprint(response, `{
			"current": {"temperature_2m":22.6,"relative_humidity_2m":100,"apparent_temperature":27.6,"precipitation":0.1,"weather_code":51,"wind_speed_10m":1.3,"cloud_cover":98},
			"daily": {
				"time":["2026-10-07","2026-10-08"],
				"weather_code":[95,55],
				"temperature_2m_max":[27.6,27.1],
				"temperature_2m_min":[21.4,20.1],
				"precipitation_sum":[11.0,14.3],
				"sunrise":["2026-10-07T05:46","2026-10-08T05:46"],
				"sunset":["2026-10-07T17:42","2026-10-08T17:41"]
			}
		}`)
	}))
	defer server.Close()

	client := Client{ForecastBaseURL: server.URL}
	forecast, err := client.Forecast(context.Background(), 13.6929, -89.2182, "America/El_Salvador")
	if err != nil {
		t.Fatal(err)
	}
	if forecast.Current.TemperatureC != 22.6 || forecast.Current.HumidityPercent != 100 || forecast.Current.WeatherCode != 51 {
		t.Fatalf("clima actual inesperado: %#v", forecast.Current)
	}
	if len(forecast.Daily) != 2 {
		t.Fatalf("se esperaban 2 días: %#v", forecast.Daily)
	}
	if forecast.Daily[0].Date != "2026-10-07" || forecast.Daily[0].TempMaxC != 27.6 || forecast.Daily[0].Sunrise != "05:46" || forecast.Daily[0].Sunset != "17:42" {
		t.Fatalf("día 0 inesperado: %#v", forecast.Daily[0])
	}
}

func TestForecastReturnsErrorOnHTTPFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(response, `{"reason":"Latitude must be in range of -90 to 90°"}`)
	}))
	defer server.Close()

	client := Client{ForecastBaseURL: server.URL}
	_, err := client.Forecast(context.Background(), 999, 999, "")
	if err == nil || !strings.Contains(err.Error(), "400") {
		t.Fatalf("error inesperado: %v", err)
	}
}

func TestDescribeWeatherCodeKnownAndUnknown(t *testing.T) {
	emoji, description := DescribeWeatherCode(95)
	if emoji != "⛈️" || description != "Tormenta" {
		t.Fatalf("código 95 inesperado: %s %s", emoji, description)
	}
	emoji, description = DescribeWeatherCode(12345)
	if !strings.Contains(description, "12345") {
		t.Fatalf("código desconocido debió incluir el número: %s %s", emoji, description)
	}
}
