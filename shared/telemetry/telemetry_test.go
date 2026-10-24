package telemetry

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/nats-io/nats.go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

func setupTracing(t *testing.T) *tracetest.SpanRecorder {
	t.Helper()
	rec := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec))
	prevTP, prevProp := otel.GetTracerProvider(), otel.GetTextMapPropagator()
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.TraceContext{})
	t.Cleanup(func() {
		otel.SetTracerProvider(prevTP)
		otel.SetTextMapPropagator(prevProp)
	})
	return rec
}

func TestHTTPHandler_SpanNamedAfterChiRoute(t *testing.T) {
	rec := setupTracing(t)

	r := chi.NewRouter()
	r.Get("/health", func(w http.ResponseWriter, _ *http.Request) {})
	r.Route("/posts", func(r chi.Router) {
		r.Get("/{id}", func(w http.ResponseWriter, r *http.Request) {
			// El router sigue resolviendo parámetros con el route context inyectado.
			_, _ = w.Write([]byte(chi.URLParam(r, "id")))
		})
	})
	h := HTTPHandler(r)

	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/posts/42", nil))
	if w.Body.String() != "42" {
		t.Fatalf("body = %q, want 42", w.Body.String())
	}

	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/health", nil))

	spans := rec.Ended()
	if len(spans) != 1 {
		t.Fatalf("got %d spans, want 1 (/health no se traza)", len(spans))
	}
	if got := spans[0].Name(); got != "GET /posts/{id}" {
		t.Errorf("span name = %q, want %q", got, "GET /posts/{id}")
	}
	if spans[0].SpanKind() != trace.SpanKindServer {
		t.Errorf("span kind = %v, want server", spans[0].SpanKind())
	}
}

func TestHTTPHandler_ContinuesIncomingTrace(t *testing.T) {
	rec := setupTracing(t)

	r := chi.NewRouter()
	r.Get("/feed", func(w http.ResponseWriter, _ *http.Request) {})

	const parent = "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"
	req := httptest.NewRequest(http.MethodGet, "/feed", nil)
	req.Header.Set("traceparent", parent)
	HTTPHandler(r).ServeHTTP(httptest.NewRecorder(), req)

	spans := rec.Ended()
	if len(spans) != 1 {
		t.Fatalf("got %d spans, want 1", len(spans))
	}
	if got := spans[0].SpanContext().TraceID().String(); got != "4bf92f3577b34da6a3ce929d0e0e4736" {
		t.Errorf("trace id = %s, want el del traceparent entrante", got)
	}
}

func TestHTTPHandler_MetricsOutsideRouter(t *testing.T) {
	setupTracing(t)

	// El router no tiene /metrics y exige auth para todo: igual tiene que responder.
	r := chi.NewRouter()
	r.Use(func(http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusUnauthorized) })
	})

	w := httptest.NewRecorder()
	HTTPHandler(r).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("/metrics status = %d, want 200", w.Code)
	}
}

func TestNATSPropagation_RoundTrip(t *testing.T) {
	setupTracing(t)

	ctx, span := otel.Tracer("test").Start(context.Background(), "producer")
	defer span.End()

	msg := nats.NewMsg("posts.created")
	InjectNATS(ctx, msg.Header)

	got := trace.SpanContextFromContext(ExtractNATS(context.Background(), msg.Header))
	if got.TraceID() != span.SpanContext().TraceID() || got.SpanID() != span.SpanContext().SpanID() {
		t.Fatalf("extracted %v, want %v", got, span.SpanContext())
	}
	if !got.IsRemote() {
		t.Error("el contexto extraído tiene que ser remoto")
	}
}

func TestTraceContext_RoundTrip(t *testing.T) {
	setupTracing(t)

	if data := MarshalTraceContext(context.Background()); data != nil {
		t.Fatalf("sin span: got %s, want nil (NULL en la columna)", data)
	}

	ctx, span := otel.Tracer("test").Start(context.Background(), "request")
	defer span.End()

	data := MarshalTraceContext(ctx)
	got := trace.SpanContextFromContext(UnmarshalTraceContext(context.Background(), data))
	if got.TraceID() != span.SpanContext().TraceID() {
		t.Fatalf("trace id = %s, want %s", got.TraceID(), span.SpanContext().TraceID())
	}

	// Datos corruptos no rompen: el evento sale sin padre.
	if sc := trace.SpanContextFromContext(UnmarshalTraceContext(context.Background(), []byte("{"))); sc.IsValid() {
		t.Error("datos inválidos no deberían producir un contexto")
	}
}
