package postgres

import (
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
)

// PoolCollector exports pgxpool statistics as Prometheus metrics.
type PoolCollector struct {
	pool     *pgxpool.Pool
	acquired *prometheus.Desc
	idle     *prometheus.Desc
	total    *prometheus.Desc
	max      *prometheus.Desc
	waitSecs *prometheus.Desc
}

// NewPoolCollector creates a collector for pool.
func NewPoolCollector(pool *pgxpool.Pool) *PoolCollector {
	d := func(name, help string) *prometheus.Desc {
		return prometheus.NewDesc("db_pool_"+name, help, nil, nil)
	}
	return &PoolCollector{
		pool:     pool,
		acquired: d("acquired_conns", "Connections currently in use."),
		idle:     d("idle_conns", "Idle connections."),
		total:    d("total_conns", "Total connections."),
		max:      d("max_conns", "Configured maximum connections."),
		waitSecs: d("acquire_wait_seconds_total", "Cumulative time waiting for a connection."),
	}
}

// Describe implements prometheus.Collector.
func (c *PoolCollector) Describe(ch chan<- *prometheus.Desc) {
	for _, d := range []*prometheus.Desc{c.acquired, c.idle, c.total, c.max, c.waitSecs} {
		ch <- d
	}
}

// Collect implements prometheus.Collector.
func (c *PoolCollector) Collect(ch chan<- prometheus.Metric) {
	s := c.pool.Stat()
	ch <- prometheus.MustNewConstMetric(c.acquired, prometheus.GaugeValue, float64(s.AcquiredConns()))
	ch <- prometheus.MustNewConstMetric(c.idle, prometheus.GaugeValue, float64(s.IdleConns()))
	ch <- prometheus.MustNewConstMetric(c.total, prometheus.GaugeValue, float64(s.TotalConns()))
	ch <- prometheus.MustNewConstMetric(c.max, prometheus.GaugeValue, float64(s.MaxConns()))
	ch <- prometheus.MustNewConstMetric(c.waitSecs, prometheus.CounterValue, s.AcquireDuration().Seconds())
}
