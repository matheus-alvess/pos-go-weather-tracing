package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/exporters/zipkin"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.10.0"
	"io"
	"log"
	"net/http"
	"os"
)

type CEPRequest struct {
	CEP string `json:"cep"`
}

const (
	WEATHER_API_URL = "http://weather-app:9090/getWeather"
)

func main() {
	initTracer()
	http.HandleFunc("/weather", weatherHandler)
	fmt.Println("Running app At Port :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
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
			semconv.ServiceNameKey.String("zip_code_app"),
		)),
	)
	otel.SetTracerProvider(tp)
}

func weatherHandler(w http.ResponseWriter, r *http.Request) {
	tr := otel.Tracer("zip_code_app")
	ctx, span := tr.Start(context.Background(), "weatherHandler")
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

	resp, err := sendToWeatherApp(ctx, requestData)
	if err != nil {
		http.Error(w, "Failed to contact Weather App", http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)

	if _, err := io.Copy(w, resp.Body); err != nil {
		http.Error(w, "Failed to copy response body", http.StatusInternalServerError)
		return
	}
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

func sendToWeatherApp(ctx context.Context, requestData CEPRequest) (*http.Response, error) {
	tr := otel.Tracer("zip_code_app")
	_, span := tr.Start(ctx, "sendToWeatherApp")
	defer span.End()

	requestBody, err := json.Marshal(requestData)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, WEATHER_API_URL, bytes.NewBuffer(requestBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	client := http.Client{Transport: otelhttp.NewTransport(http.DefaultTransport)}
	return client.Do(req)
}
