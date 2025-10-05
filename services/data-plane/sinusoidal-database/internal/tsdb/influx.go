package tsdb

import (
	"context"
	"fmt"
	"time"

	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	"github.com/influxdata/influxdb-client-go/v2/api"
	"github.com/jlbutler/cloud-native-encabulator/services/data-plane/sinusoidal-database/internal/models"
)

type InfluxDB struct {
	client influxdb2.Client
	org    string
	bucket string
	ctx    context.Context
}

func NewInfluxDB(url, token, org, bucket string) (*InfluxDB, error) {
	client := influxdb2.NewClient(url, token)

	ctx := context.Background()

	// Test connection
	health, err := client.Health(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to InfluxDB: %w", err)
	}

	if health.Status != "pass" {
		return nil, fmt.Errorf("InfluxDB health check failed: %s", health.Status)
	}

	return &InfluxDB{
		client: client,
		org:    org,
		bucket: bucket,
		ctx:    ctx,
	}, nil
}

func (i *InfluxDB) WritePoints(points []models.MetricPoint) error {
	writeAPI := i.client.WriteAPIBlocking(i.org, i.bucket)

	for _, point := range points {
		p := influxdb2.NewPoint(
			point.Measurement,
			point.Tags,
			point.Fields,
			point.Timestamp,
		)

		if err := writeAPI.WritePoint(i.ctx, p); err != nil {
			return fmt.Errorf("failed to write point: %w", err)
		}
	}

	return nil
}

func (i *InfluxDB) QueryRange(measurement string, start, end time.Time, tags map[string]string) ([]models.MetricPoint, error) {
	queryAPI := i.client.QueryAPI(i.org)

	// Build query
	query := fmt.Sprintf(`from(bucket: "%s")
		|> range(start: %s, stop: %s)
		|> filter(fn: (r) => r._measurement == "%s")`,
		i.bucket,
		start.Format(time.RFC3339),
		end.Format(time.RFC3339),
		measurement,
	)

	// Add tag filters
	for key, value := range tags {
		query += fmt.Sprintf(`
		|> filter(fn: (r) => r.%s == "%s")`, key, value)
	}

	result, err := queryAPI.Query(i.ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query: %w", err)
	}
	defer result.Close()

	var points []models.MetricPoint
	for result.Next() {
		record := result.Record()

		point := models.MetricPoint{
			Measurement: record.Measurement(),
			Tags:        make(map[string]string),
			Fields:      make(map[string]interface{}),
			Timestamp:   record.Time(),
		}

		// Extract tags
		for key, value := range record.Values() {
			if key != "_time" && key != "_value" && key != "_field" && key != "_measurement" {
				if strVal, ok := value.(string); ok {
					point.Tags[key] = strVal
				}
			}
		}

		// Extract field
		fieldName := record.Field()
		point.Fields[fieldName] = record.Value()

		points = append(points, point)
	}

	if result.Err() != nil {
		return nil, fmt.Errorf("query error: %w", result.Err())
	}

	return points, nil
}

func (i *InfluxDB) Ping() error {
	health, err := i.client.Health(i.ctx)
	if err != nil {
		return err
	}

	if health.Status != "pass" {
		return fmt.Errorf("health check failed: %s", health.Status)
	}

	return nil
}

func (i *InfluxDB) Close() {
	i.client.Close()
}

// Helper to get write API for non-blocking writes
func (i *InfluxDB) GetWriteAPI() api.WriteAPI {
	return i.client.WriteAPI(i.org, i.bucket)
}
