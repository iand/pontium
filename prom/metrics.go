package prom

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	promexp "contrib.go.opencensus.io/exporter/prometheus"
	"github.com/prometheus/client_golang/prometheus"
	"go.opencensus.io/stats/view"
	"golang.org/x/exp/slog"
)

type (
	Counter = prometheus.Counter
	Gauge   = prometheus.Gauge
)

type PrometheusServer struct {
	addr        string
	metricsPath string
	pe          *promexp.Exporter
}

func NewPrometheusServer(addr string, metricsPath string, appName string) (*PrometheusServer, error) {
	pe, err := promexp.NewExporter(promexp.Options{
		Namespace:  appName,
		Registerer: prometheus.DefaultRegisterer,
		Gatherer:   prometheus.DefaultGatherer,
	})
	if err != nil {
		return nil, fmt.Errorf("new prometheus exporter: %w", err)
	}

	if !strings.HasPrefix(metricsPath, "/") {
		metricsPath = "/" + metricsPath
	}

	// register prometheus with opencensus
	view.RegisterExporter(pe)
	view.SetReportingPeriod(2 * time.Second)
	return &PrometheusServer{
		addr:        addr,
		metricsPath: metricsPath,
		pe:          pe,
	}, nil
}

func (p *PrometheusServer) Run(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.Handle(p.metricsPath, p.pe)
	server := &http.Server{Addr: p.addr, Handler: mux}
	go func() {
		<-ctx.Done()
		if err := server.Shutdown(context.Background()); err != nil {
			slog.Error("failed to shut down prometheus server", err)
		}
	}()

	slog.Info("starting prometheus server", "addr", p.addr, "path", p.metricsPath)
	return server.ListenAndServe()
}

func NewPrometheusCounter(name string, help string, labels map[string]string) (Counter, error) {
	m := prometheus.NewCounter(
		prometheus.CounterOpts{
			Name:        name,
			Help:        help,
			ConstLabels: labels,
		},
	)
	if err := prometheus.Register(m); err != nil {
		if are, ok := err.(prometheus.AlreadyRegisteredError); ok {
			m = are.ExistingCollector.(prometheus.Counter)
		} else {
			return nil, fmt.Errorf("register %s counter: %w", name, err)
		}
	}
	return m, nil
}

func NewPrometheusGauge(name string, help string, labels map[string]string) (Gauge, error) {
	m := prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name:        name,
			Help:        help,
			ConstLabels: labels,
		},
	)
	if err := prometheus.Register(m); err != nil {
		if are, ok := err.(prometheus.AlreadyRegisteredError); ok {
			m = are.ExistingCollector.(prometheus.Gauge)
		} else {
			return nil, fmt.Errorf("register %s gauge: %w", name, err)
		}
	}
	return m, nil
}
