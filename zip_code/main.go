package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	semconv "go.opentelemetry.io/otel/semconv/v1.10.0"
	"log"
	"net/http"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
)

type CEPRequest struct {
	CEP string `json:"cep"`
}

const (
	ZIPKIN_HOST     = "http://localhost:9411/api/v2/spans"
	WEATHER_API_URL = "http://localhost:9090/getWeather"
)

func main() {
	shutdown, err := initTracer()
	if err != nil {
		log.Fatalf("failed to initialize tracer: %v", err)
	}
	defer shutdown(context.Background())

	http.HandleFunc("/weather", weatherHandler)
	fmt.Println("Running app at Port :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
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
			semconv.ServiceNameKey.String("zip_code_app"),
		)),
	)

	otel.SetTracerProvider(tp)
	return tp.Shutdown, nil
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
	_, _ = w.Write([]byte(fmt.Sprintf("Response from Weather App: %v", resp.StatusCode)))
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

	client := &http.Client{}
	return client.Do(req)
}
