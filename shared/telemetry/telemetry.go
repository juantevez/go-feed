// Package telemetry configura OpenTelemetry para los servicios: traces por
// OTLP, métricas en /metrics (formato Prometheus) y propagación del contexto
// por HTTP y por NATS.
//
// Toda la configuración sale de las variables de entorno estándar del SDK:
//
//	OTEL_SERVICE_NAME                  pisa el nombre que se pasa a Init
//	OTEL_EXPORTER_OTLP_ENDPOINT        ej. http://tempo:4318 (sin esto no se exportan traces)
//	OTEL_TRACES_SAMPLER(_ARG)          ej. parentbased_traceidratio / 0.1
//
// Sin endpoint los traces no salen del proceso, pero igual se generan y se
// propagan: los trace_id siguen apareciendo en los logs y viajando en los
// headers, así un `go run` sin Tempo no llena la consola de errores de export.
package telemetry

import (
	"context"
	"errors"
	"fmt"
	"os"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	otelprom "go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// Init instala los providers globales de traces y métricas y el propagador
// W3C (traceparent + baggage). Llamarlo primero en main, antes de crear el
// pool de Postgres o el router: los instrumentadores toman los globales.
//
// La función que retorna vacía el buffer de spans; llamarla al apagar.
func Init(ctx context.Context, serviceName string) (shutdown func(context.Context) error, err error) {
	// El orden importa: los detectores posteriores pisan a los anteriores, así
	// OTEL_SERVICE_NAME y OTEL_RESOURCE_ATTRIBUTES ganan sobre el default.
	res, err := resource.New(ctx,
		resource.WithAttributes(attribute.String("service.name", serviceName)),
		resource.WithTelemetrySDK(),
		resource.WithHost(),
		resource.WithFromEnv(),
	)
	if err != nil {
		return nil, fmt.Errorf("telemetry: resource: %w", err)
	}

	traceOpts := []sdktrace.TracerProviderOption{sdktrace.WithResource(res)}
	if exportEnabled() {
		exp, err := otlptracehttp.New(ctx)
		if err != nil {
			return nil, fmt.Errorf("telemetry: otlp exporter: %w", err)
		}
		traceOpts = append(traceOpts, sdktrace.WithBatcher(exp))
	}
	// El sampler sale de OTEL_TRACES_SAMPLER; el default del SDK es
	// parentbased_always_on.
	tp := sdktrace.NewTracerProvider(traceOpts...)
	otel.SetTracerProvider(tp)

	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{}, propagation.Baggage{},
	))

	// Las métricas se registran en el registry default de Prometheus, el
	// mismo que ya trae go_* y process_*: /metrics los sirve todos juntos.
	promExp, err := otelprom.New()
	if err != nil {
		return nil, fmt.Errorf("telemetry: prometheus exporter: %w", err)
	}
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithResource(res), sdkmetric.WithReader(promExp))
	otel.SetMeterProvider(mp)

	return func(ctx context.Context) error {
		return errors.Join(tp.Shutdown(ctx), mp.Shutdown(ctx))
	}, nil
}

func exportEnabled() bool {
	return os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT") != "" ||
		os.Getenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT") != ""
}
