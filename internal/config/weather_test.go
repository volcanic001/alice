package config

import "testing"

func TestSaveAndFindWeatherCity(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	city := WeatherCity{Name: "San Salvador", Country: "El Salvador", Latitude: 13.69, Longitude: -89.19, Timezone: "America/El_Salvador"}
	if err := SaveWeatherCity(city); err != nil {
		t.Fatal(err)
	}

	matches, err := FindWeatherCities("san salvador")
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 || matches[0] != city {
		t.Fatalf("ciudad encontrada no coincide: %#v", matches)
	}
}

func TestFindWeatherCitiesPartialMatch(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if err := SaveWeatherCity(WeatherCity{Name: "San Juan Tepezontes", Country: "El Salvador"}); err != nil {
		t.Fatal(err)
	}

	matches, err := FindWeatherCities("tepezontes")
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 || matches[0].Name != "San Juan Tepezontes" {
		t.Fatalf("no encontró por coincidencia parcial: %#v", matches)
	}
}

func TestFindWeatherCitiesAmbiguousAcrossSavedCities(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if err := SaveWeatherCity(WeatherCity{Name: "San Salvador", Country: "El Salvador"}); err != nil {
		t.Fatal(err)
	}
	if err := SaveWeatherCity(WeatherCity{Name: "San Miguel", Country: "El Salvador"}); err != nil {
		t.Fatal(err)
	}

	matches, err := FindWeatherCities("san")
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 2 {
		t.Fatalf("se esperaban 2 coincidencias ambiguas: %#v", matches)
	}
}

func TestFindWeatherCitiesNoMatch(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	matches, err := FindWeatherCities("no-existe")
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Fatalf("se esperaba lista vacía: %#v", matches)
	}
}

func TestFindWeatherCitiesEmptyQueryMatchesNothing(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if err := SaveWeatherCity(WeatherCity{Name: "San Salvador", Country: "El Salvador"}); err != nil {
		t.Fatal(err)
	}
	matches, err := FindWeatherCities("")
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Fatalf("una query vacía no debería matchear nada: %#v", matches)
	}
}

func TestLoadWeatherCitiesPreservesInsertionOrderForDefault(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	if err := SaveWeatherCity(WeatherCity{Name: "San Salvador", Country: "El Salvador"}); err != nil {
		t.Fatal(err)
	}
	if err := SaveWeatherCity(WeatherCity{Name: "San Juan Tepezontes", Country: "El Salvador"}); err != nil {
		t.Fatal(err)
	}

	cities, err := LoadWeatherCities()
	if err != nil {
		t.Fatal(err)
	}
	if len(cities) != 2 || cities[0].Name != "San Salvador" || cities[1].Name != "San Juan Tepezontes" {
		t.Fatalf("orden inesperado: %#v", cities)
	}
}

func TestSaveWeatherCityOverwritesSameNameCountryInPlace(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	if err := SaveWeatherCity(WeatherCity{Name: "San Salvador", Country: "El Salvador", Latitude: 1}); err != nil {
		t.Fatal(err)
	}
	if err := SaveWeatherCity(WeatherCity{Name: "Otra ciudad", Country: "México"}); err != nil {
		t.Fatal(err)
	}
	if err := SaveWeatherCity(WeatherCity{Name: "San Salvador", Country: "El Salvador", Latitude: 2}); err != nil {
		t.Fatal(err)
	}

	cities, err := LoadWeatherCities()
	if err != nil {
		t.Fatal(err)
	}
	if len(cities) != 2 || cities[0].Latitude != 2 || cities[0].Name != "San Salvador" {
		t.Fatalf("reemplazo no mantuvo la posición: %#v", cities)
	}
}

func TestRemoveWeatherCity(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	if err := SaveWeatherCity(WeatherCity{Name: "San Salvador", Country: "El Salvador"}); err != nil {
		t.Fatal(err)
	}
	if err := RemoveWeatherCity("San Salvador", "El Salvador"); err != nil {
		t.Fatal(err)
	}

	matches, err := FindWeatherCities("san salvador")
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Fatalf("la ciudad debió quedar eliminada: %#v", matches)
	}
}

func TestRemoveWeatherCityMissingIsNotError(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if err := RemoveWeatherCity("No existe", "Ninguno"); err != nil {
		t.Fatalf("quitar una ciudad inexistente no debería fallar: %v", err)
	}
}

func TestLoadWeatherCitiesEmptyWhenNeverSaved(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cities, err := LoadWeatherCities()
	if err != nil {
		t.Fatal(err)
	}
	if len(cities) != 0 {
		t.Fatalf("se esperaba lista vacía: %#v", cities)
	}
}
