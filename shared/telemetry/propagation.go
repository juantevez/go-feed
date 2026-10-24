package telemetry

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/exaring/otelpgx"
	"github.com/jackc/pgx/v5"
	"github.com/nats-io/nats.go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
)

// PgxTracer traza las queries de pgx (pgxpool.Config.ConnConfig.Tracer).
//
// Solo crea spans dentro de un trace existente: el polling del outbox relay o
// el cleanup periódico no generan traces sueltos.
func PgxTracer() pgx.QueryTracer {
	return otelpgx.NewTracer(otelpgx.WithTrimSQLInSpanName())
}

// InjectNATS escribe el contexto de ctx (traceparent) en los headers de un
// mensaje NATS. Se usa http.Header como carrier: los dos son
// map[string][]string, y así inject y extract canonicalizan igual las claves.
func InjectNATS(ctx context.Context, h nats.Header) {
	otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(http.Header(h)))
}

// ExtractNATS retorna ctx con el contexto remoto que traiga el mensaje.
func ExtractNATS(ctx context.Context, h nats.Header) context.Context {
	return otel.GetTextMapPropagator().Extract(ctx, propagation.HeaderCarrier(http.Header(h)))
}

// MarshalTraceContext serializa el contexto de ctx para guardarlo junto a un
// evento (la columna trace_context del outbox). nil si no hay trace.
func MarshalTraceContext(ctx context.Context) []byte {
	carrier := propagation.MapCarrier{}
	otel.GetTextMapPropagator().Inject(ctx, carrier)
	if len(carrier) == 0 {
		return nil
	}
	data, err := json.Marshal(carrier)
	if err != nil {
		return nil
	}
	return data
}

// UnmarshalTraceContext es la inversa: retorna ctx con el contexto guardado.
// Datos vacíos o inválidos devuelven ctx tal cual (el evento sale sin padre).
func UnmarshalTraceContext(ctx context.Context, data []byte) context.Context {
	if len(data) == 0 {
		return ctx
	}
	carrier := propagation.MapCarrier{}
	if err := json.Unmarshal(data, &carrier); err != nil {
		return ctx
	}
	return otel.GetTextMapPropagator().Extract(ctx, carrier)
}
