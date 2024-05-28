package main

import (
	"context"
	"encoding/json"
	"fmt"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/zipkin"
	"go.opentelemetry.io/otel/sdk/resource"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"

	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"go.opentelemetry.io/otel/semconv/v1.4.0"
)

const (
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
	initTracer()

	http.HandleFunc("/getWeather", handleGetWeatherRequest)
	fmt.Println("Running app at Port :9090")
	log.Fatal(http.ListenAndServe(":9090", nil))
}

func initTracer() {
	ctx := context.Background()
	client := otlptracehttp.NewClient(otlptracehttp.WithEndpoint("otel-collector:4317"), otlptracehttp.WithInsecure())
	exporter, err := otlptrace.New(ctx, client)

	if err != nil {
		log.Fatalf("failed to create exporter: %v", err)
	}

	endpoint := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	if endpoint == "" {
		log.Fatal("OTEL_EXPORTER_OTLP_ENDPOINT is not set")
	}
	fmt.Println("OTEL_EXPORTER_OTLP_ENDPOINT", endpoint)

	zipkinExporter, err := zipkin.New(endpoint)
	if err != nil {
		log.Fatalf("failed to create zipkin exporter: %v", err)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithBatcher(zipkinExporter),
		sdktrace.WithResource(resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceNameKey.String("weather-app"),
		)),
	)
	otel.SetTracerProvider(tp)
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
		fmt.Println("Failed to get weather data", err)
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

	client := http.Client{Transport: otelhttp.NewTransport(http.DefaultTransport)}
	resp, err := client.Get(fmt.Sprintf(VIA_CEP_API_URL, cep))
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

	encodedCity := url.QueryEscape(city)
	client := http.Client{Transport: otelhttp.NewTransport(http.DefaultTransport)}
	resp, err := client.Get(fmt.Sprintf(WEATHER_EXTERNAL_API, WEATHER_API_KEY, encodedCity))
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
