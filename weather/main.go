package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"strings"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/sdk/resource"

	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/semconv/v1.4.0"
)

const (
	ZIPKIN_HOST          = "http://localhost:9411/api/v2/spans"
	VIA_CEP_API_URL      = "https://viacep.com.br/ws/%s/json/"
	WEATHER_EXTERNAL_API = "http://api.weatherapi.com/v1/current.json?key=%s&q=%s"
	WEATHER_API_KEY      = "0221a1d62222490882322259242305"
)

type CEPRequest struct {
	CEP string `json:"cep"`
}

type WeatherResponse struct {
	City  string  `json:"city"`
	TempC float64 `json:"temp_C"`
	TempF float64 `json:"temp_F"`
	TempK float64 `json:"temp_K"`
}

type WeatherAPIResponse struct {
	Current struct {
		TempC float64 `json:"temp_c"`
	} `json:"current"`
}

func main() {
	shutdown, err := initTracer()
	if err != nil {
		log.Fatalf("failed to initialize tracer: %v", err)
	}
	defer shutdown(context.Background())

	http.HandleFunc("/getWeather", handleGetWeatherRequest)
	fmt.Println("Running app at Port :9090")
	log.Fatal(http.ListenAndServe(":9090", nil))
}

func initTracer() (func(context.Context) error, error) {
	ctx := context.Background()

	exporter, err := otlptracehttp.New(
		ctx,
		otlptracehttp.WithEndpoint(ZIPKIN_HOST),
		otlptracehttp.WithInsecure(),
	)
	if err != nil {
		return nil, err
	}

	tp := trace.NewTracerProvider(
		trace.WithBatcher(exporter),
		trace.WithResource(resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceNameKey.String("weather-app"),
		)),
	)

	otel.SetTracerProvider(tp)
	return tp.Shutdown, nil
}

func handleGetWeatherRequest(w http.ResponseWriter, r *http.Request) {
	tr := otel.Tracer("weather-app")
	ctx, span := tr.Start(context.Background(), "handleGetWeatherRequest")
	defer span.End()

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var requestData CEPRequest
	err := json.NewDecoder(r.Body).Decode(&requestData)
	if err != nil {
		http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
		return
	}

	if !isValidCEP(requestData.CEP) {
		http.Error(w, "invalid zipcode", http.StatusUnprocessableEntity)
		return
	}

	city, err := getCityFromCEP(ctx, requestData.CEP)
	if err != nil {
		http.Error(w, "can not find zipcode", http.StatusNotFound)
		return
	}

	weather, err := getWeather(ctx, city)
	if err != nil {
		http.Error(w, "Failed to get weather data", http.StatusInternalServerError)
		return
	}

	responseData := WeatherResponse{
		City:  city,
		TempC: weather.Current.TempC,
		TempF: celsiusToFahrenheit(weather.Current.TempC),
		TempK: celsiusToKelvin(weather.Current.TempC),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(responseData)
}

func isValidCEP(cep string) bool {
	if len(cep) != 8 {
		return false
	}
	for _, char := range cep {
		if char < '0' || char > '9' {
			return false
		}
	}
	return true
}

func getCityFromCEP(ctx context.Context, cep string) (string, error) {
	tr := otel.Tracer("weather-app")
	_, span := tr.Start(ctx, "getCityFromCEP")
	defer span.End()

	resp, err := http.Get(fmt.Sprintf(VIA_CEP_API_URL, cep))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to fetch city")
	}

	var result map[string]interface{}
	body, _ := ioutil.ReadAll(resp.Body)
	json.Unmarshal(body, &result)

	if result["erro"] != nil {
		return "", fmt.Errorf("invalid CEP")
	}

	city, ok := result["localidade"].(string)
	if !ok {
		return "", fmt.Errorf("could not find city")
	}

	return city, nil
}

func getWeather(ctx context.Context, city string) (*WeatherAPIResponse, error) {
	tr := otel.Tracer("weather-app")
	_, span := tr.Start(ctx, "getWeather")
	defer span.End()

	resp, err := http.Get(fmt.Sprintf(WEATHER_EXTERNAL_API, WEATHER_API_KEY, strings.ReplaceAll(city, " ", "%20")))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to fetch weather data")
	}

	var weather WeatherAPIResponse
	body, _ := ioutil.ReadAll(resp.Body)
	json.Unmarshal(body, &weather)

	return &weather, nil
}

func celsiusToFahrenheit(celsius float64) float64 {
	return celsius*1.8 + 32
}

func celsiusToKelvin(celsius float64) float64 {
	return celsius + 273.15
}
