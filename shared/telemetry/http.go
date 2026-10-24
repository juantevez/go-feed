package telemetry

import (
	"context"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// HTTPHandler envuelve el router chi de un servicio:
//   - GET /metrics sirve las métricas (fuera del router: sin CORS ni JWT).
//   - el resto se traza con otelhttp, extrayendo el traceparent entrante.
//
// El span se llama "<MÉTODO> <ruta de chi>" (GET /posts/{id}), no con el path
// real: el nombre es lo que agrupa Tempo en span-metrics, y con ids adentro
// cada request sería una serie distinta.
func HTTPHandler(router http.Handler) http.Handler {
	metrics := promhttp.Handler()
	traced := otelhttp.NewHandler(withRouteName(router), "http.server",
		otelhttp.WithFilter(traceable),
		otelhttp.WithSpanNameFormatter(func(_ string, r *http.Request) string { return r.Method }),
	)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/metrics" {
			metrics.ServeHTTP(w, r)
			return
		}
		traced.ServeHTTP(w, r)
	})
}

// traceable deja afuera lo que no es tráfico de negocio: los health checks
// (uno cada pocos segundos, para siempre) y los WebSockets, cuyo "request"
// dura lo que la conexión y arruinaría la latencia de span-metrics.
func traceable(r *http.Request) bool {
	switch r.URL.Path {
	case "/health", "/ready":
		return false
	}
	return !strings.EqualFold(r.Header.Get("Upgrade"), "websocket")
}

// withRouteName renombra el span con la ruta que resolvió chi.
//
// chi solo conoce la ruta después de rutear, y la guarda en su propia copia
// del request: desde afuera no se ve. Pero si el request ya trae un route
// context, chi reusa ese mismo puntero en vez de crear uno, así que se le pasa
// uno vacío y se lee al volver.
func withRouteName(router http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rctx := chi.NewRouteContext()
		if routes, ok := router.(chi.Routes); ok {
			rctx.Routes = routes
		}
		r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))

		router.ServeHTTP(w, r)

		route := rctx.RoutePattern()
		if route == "" {
			return
		}
		trace.SpanFromContext(r.Context()).SetName(r.Method + " " + route)
		trace.SpanFromContext(r.Context()).SetAttributes(attribute.String("http.route", route))
		// La métrica http.server.request.duration también agrupa por ruta.
		if l, ok := otelhttp.LabelerFromContext(r.Context()); ok {
			l.Add(attribute.String("http.route", route))
		}
	})
}
