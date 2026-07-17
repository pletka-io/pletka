package observability

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
)

// --- pgx pool collector -----------------------------------------------------

type poolCollector struct {
	pool *pgxpool.Pool

	total        *prometheus.Desc
	acquired     *prometheus.Desc
	idle         *prometheus.Desc
	max          *prometheus.Desc
	constructing *prometheus.Desc
	acquireCount *prometheus.Desc
	emptyAcquire *prometheus.Desc
	canceled     *prometheus.Desc
	acquireSecs  *prometheus.Desc
}

func newPoolCollector(pool *pgxpool.Pool) *poolCollector {
	d := func(name, help string) *prometheus.Desc { return prometheus.NewDesc(name, help, nil, nil) }
	return &poolCollector{
		pool:         pool,
		total:        d("pletka_db_pool_total_conns", "Total connections in the pgx pool."),
		acquired:     d("pletka_db_pool_acquired_conns", "Currently acquired connections."),
		idle:         d("pletka_db_pool_idle_conns", "Currently idle connections."),
		max:          d("pletka_db_pool_max_conns", "Maximum pool size."),
		constructing: d("pletka_db_pool_constructing_conns", "Connections being constructed."),
		acquireCount: d("pletka_db_pool_acquire_total", "Cumulative successful acquires."),
		emptyAcquire: d("pletka_db_pool_empty_acquire_total", "Acquires that waited for a new/idle conn."),
		canceled:     d("pletka_db_pool_canceled_acquire_total", "Acquires canceled by context."),
		acquireSecs:  d("pletka_db_pool_acquire_duration_seconds_total", "Cumulative time spent acquiring connections."),
	}
}

func (c *poolCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.total
	ch <- c.acquired
	ch <- c.idle
	ch <- c.max
	ch <- c.constructing
	ch <- c.acquireCount
	ch <- c.emptyAcquire
	ch <- c.canceled
	ch <- c.acquireSecs
}

func (c *poolCollector) Collect(ch chan<- prometheus.Metric) {
	s := c.pool.Stat()
	g := func(desc *prometheus.Desc, v float64) {
		ch <- prometheus.MustNewConstMetric(desc, prometheus.GaugeValue, v)
	}
	ct := func(desc *prometheus.Desc, v float64) {
		ch <- prometheus.MustNewConstMetric(desc, prometheus.CounterValue, v)
	}
	g(c.total, float64(s.TotalConns()))
	g(c.acquired, float64(s.AcquiredConns()))
	g(c.idle, float64(s.IdleConns()))
	g(c.max, float64(s.MaxConns()))
	g(c.constructing, float64(s.ConstructingConns()))
	ct(c.acquireCount, float64(s.AcquireCount()))
	ct(c.emptyAcquire, float64(s.EmptyAcquireCount()))
	ct(c.canceled, float64(s.CanceledAcquireCount()))
	ct(c.acquireSecs, s.AcquireDuration().Seconds())
}

// --- materialization gauges (queue depth + lag) -----------------------------

// materializationCollector exposes the live git-materialization backlog by
// querying weave_change_set at scrape time — the same signals the
// /admin/materialization viewer shows, in Prometheus form for alerting.
type materializationCollector struct {
	pool  *pgxpool.Pool
	queue *prometheus.Desc
	lag   *prometheus.Desc
}

func newMaterializationCollector(pool *pgxpool.Pool) *materializationCollector {
	return &materializationCollector{
		pool:  pool,
		queue: prometheus.NewDesc("pletka_materialization_queue_depth", "Closed change sets not yet materialized to git.", nil, nil),
		lag:   prometheus.NewDesc("pletka_materialization_lag_seconds", "Age of the oldest unprocessed closed change set.", nil, nil),
	}
}

func (c *materializationCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.queue
	ch <- c.lag
}

func (c *materializationCollector) Collect(ch chan<- prometheus.Metric) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	var queue int
	var lag float64
	err := c.pool.QueryRow(ctx, `
		SELECT
		  count(*) FILTER (WHERE closed_at IS NOT NULL AND processed_at IS NULL),
		  COALESCE(EXTRACT(EPOCH FROM (now() - min(closed_at)
		    FILTER (WHERE closed_at IS NOT NULL AND processed_at IS NULL)))::float8, 0)
		FROM weave_change_set
	`).Scan(&queue, &lag)
	if err != nil {
		// Skip these two metrics this scrape rather than failing the endpoint.
		return
	}
	ch <- prometheus.MustNewConstMetric(c.queue, prometheus.GaugeValue, float64(queue))
	ch <- prometheus.MustNewConstMetric(c.lag, prometheus.GaugeValue, lag)
}
